package logview

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

// maxMatches caps a search. Past this the answer is "your search is too broad",
// not "here are two million lines".
const maxMatches = 5000

// Sink is where a search reports to. The job wires it to jc.Emit and
// jc.Progress; a test wires it to a slice, which is the only way to read what a
// search emitted without a Wails runtime.
type Sink struct {
	Emit     func(Match)
	Progress func(done, total int, message string)
}

// validate catches everything a user can get wrong, so bad input rejects the
// StartSearch call instead of starting a job that immediately fails.
func (p SearchParams) validate() (SearchParams, error) {
	if strings.TrimSpace(p.Path) == "" {
		return p, core.Errorf(core.CodeInvalidInput,
			"Choose a log file, or type the full path to it.")
	}
	if p.Query == "" {
		return p, core.Errorf(core.CodeInvalidInput, "Type something to look for.")
	}
	if len(p.Query) > maxQuery {
		return p, core.Errorf(core.CodeInvalidInput,
			"That search text is too long. Use a shorter piece of the line.")
	}
	return p, nil
}

// Service owns the search entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartSearch scans the whole file for a plain substring and returns the job id
// at once. Matching lines arrive as "match" items on job:result.
func (s *Service) StartSearch(p SearchParams) (string, error) {
	p, err := p.validate()
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		return "", openError(p.Path, err)
	}
	if info.IsDir() {
		return "", core.Errorf(core.CodeInvalidInput,
			"%s is a folder, not a file. Pick the log file inside it.", p.Path)
	}

	return s.jobs.Start(JobKind, int(info.Size()), func(jc *core.JobContext) error {
		return runSearch(jc, p)
	}), nil
}

// runSearch is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func runSearch(jc *core.JobContext, p SearchParams) error {
	summary, err := Search(jc.Ctx(), p, Sink{
		Emit:     func(m Match) { jc.Emit(KindMatch, m) },
		Progress: func(done, total int, message string) { jc.Progress(done, total, message) },
	})
	if err != nil {
		return err
	}
	jc.SetSummary(summary)
	return nil
}

// Search reads the file once from the start, emitting one Match per line that
// contains the query, and returns the job:done summary.
func Search(ctx context.Context, p SearchParams, out Sink) (map[string]any, error) {
	f, err := os.Open(p.Path)
	if err != nil {
		return nil, openError(p.Path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, openError(p.Path, err)
	}
	size := stat.Size()

	needle := p.Query
	if !p.MatchCase {
		needle = strings.ToLower(needle)
	}

	reader := bufio.NewReaderSize(f, blockBytes)
	name := filepath.Base(p.Path)

	var offset int64
	number := 0
	matches := 0
	capped := false
	lastProgress := time.Time{}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		raw, readErr := reader.ReadBytes('\n')
		consumed := int64(len(raw))
		if consumed == 0 && readErr != nil {
			break
		}

		lineStart := offset
		offset += consumed
		number++

		text, truncated := clean(trimNewline(raw))
		haystack := text
		if !p.MatchCase {
			haystack = strings.ToLower(text)
		}

		if col := strings.Index(haystack, needle); col >= 0 {
			out.Emit(Match{
				Number:    number,
				Offset:    lineStart,
				Text:      text,
				Level:     Level(text),
				Col:       col,
				Truncated: truncated,
			})
			matches++
			if matches >= maxMatches {
				capped = true
			}
		}

		if now := time.Now(); now.Sub(lastProgress) >= progressInterval {
			lastProgress = now
			out.Progress(int(offset), int(size), "Searching "+name)
		}

		if capped {
			break
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, core.Errorf(core.CodeInternal,
				"Reading %s stopped part way through, so the search is incomplete. The drive or the network share it is on may have disconnected.", name)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return searchSummary(p, matches, number, size, capped), nil
}

// progressInterval throttles progress reports; core coalesces to ~10/sec anyway.
const progressInterval = 200 * time.Millisecond

// searchSummary is the job:done summary. Its keys are a contract with the page
// and are checked nowhere by the compiler, in either language, so they are
// built in one named place that a test can pin.
func searchSummary(p SearchParams, matches, lines int, size int64, capped bool) map[string]any {
	return map[string]any{
		"path":    p.Path,
		"query":   p.Query,
		"matches": matches,
		"lines":   lines,
		"bytes":   size,
		"capped":  capped,
		"note":    searchNote(p.Query, matches, capped),
	}
}

// searchNote explains a truncated or empty result, so an empty table never
// reads as a broken tool.
func searchNote(query string, matches int, capped bool) string {
	switch {
	case capped:
		return "Stopped after the first " + strconv.Itoa(maxMatches) +
			" matching lines. Search for something more specific to see the rest."
	case matches == 0:
		return "Nothing in that file contains \"" + query +
			"\". Check the spelling, and remember that this search is exact text, not a pattern. Turn Match case off if you are not sure of the capitals."
	}
	return ""
}
