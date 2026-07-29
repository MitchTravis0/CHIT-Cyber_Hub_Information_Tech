// Package diskscan measures what is inside a folder, one immediate child at a
// time, so a tech can see which subfolder is filling a drive and drill into it.
// It is the service layer the Disk Space Visualizer page talks to.
package diskscan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"chit/internal/core"
	"chit/internal/walk"
)

// JobKind is the job manager kind for a disk scan.
const JobKind = "diskscan"

// KindEntry is the result kind of every immediate child a scan emits.
const KindEntry = "entry"

const (
	// maxLargest is how many individual files the summary carries. 25 fits on
	// one screen and is enough to spot the one huge file that is the answer.
	maxLargest = 25

	// progressInterval throttles progress reports. core already coalesces
	// events to about ten a second, so anything shorter is thrown away.
	progressInterval = 200 * time.Millisecond
)

// Params is what the UI sends.
type Params struct {
	Path string `json:"path"`
}

// Entry is one immediate child of the scanned folder, with its whole subtree
// already measured.
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
}

// Largest is one big file, reported in the summary rather than streamed,
// because it is only known to be in the top few once the scan has finished.
type Largest struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Sink is where a scan reports to. The job wires it to jc.Emit and jc.Progress;
// a test wires it to a slice, which is the only way to read what a scan emitted
// without a Wails runtime.
type Sink struct {
	Emit     func(Entry)
	Progress func(done int, message string)
}

// Service owns the scanning entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartScan begins a scan and returns the job id at once. Each immediate child
// of the folder arrives as an "entry" item on job:result the moment its whole
// subtree has been measured, so the big folders show up while the rest is still
// being counted.
func (s *Service) StartScan(p Params) (string, error) {
	root, err := walk.Root(p.Path)
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		return runScan(jc, root)
	}), nil
}

// runScan is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func runScan(jc *core.JobContext, root string) error {
	summary, err := Scan(jc.Ctx(), root, Sink{
		Emit:     func(e Entry) { jc.Emit(KindEntry, e) },
		Progress: func(done int, message string) { jc.Progress(done, 0, message) },
	})
	if err != nil {
		return err
	}
	jc.SetSummary(summary)
	return nil
}

// Scan measures every immediate child of root, reporting each one through out
// as soon as its whole subtree is counted, and returns the job:done summary.
//
// A cancelled context comes back as ctx.Err() so the job ends as job:done with
// cancelled=true rather than looking like a failure the user did not cause.
func Scan(ctx context.Context, root string, out Sink) (map[string]any, error) {
	children, err := os.ReadDir(root)
	if err != nil {
		return nil, core.Errorf(core.CodePermission,
			"This computer will not let CHIT look inside %s. Pick a folder you have access to, such as your own Documents folder.", root)
	}

	var total walk.Result
	largest := &largestSet{}
	lastProgress := time.Time{}
	// seen counts files across every child, so the number keeps climbing while
	// one big subtree is being measured.
	seen := 0

	for _, child := range children {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		path := filepath.Join(root, child.Name())

		// A symbolic link at the top level is stepped over for the same reason
		// walk steps over one deeper down: following it counts something twice.
		if child.Type()&os.ModeSymlink != 0 {
			total.Skipped++
			continue
		}

		if !child.IsDir() {
			if !child.Type().IsRegular() {
				total.Skipped++
				continue
			}
			info, err := child.Info()
			if err != nil {
				total.Skipped++
				continue
			}
			total.Files++
			total.Bytes += info.Size()
			seen++
			largest.add(walk.File{Path: path, Size: info.Size(), ModTime: info.ModTime()})
			out.Emit(Entry{Name: child.Name(), Path: path, Bytes: info.Size(), Files: 1})
			continue
		}

		res, err := walk.Walk(ctx, walk.Options{Root: path}, func(f walk.File) error {
			largest.add(f)
			seen++
			if now := time.Now(); now.Sub(lastProgress) >= progressInterval {
				lastProgress = now
				out.Progress(seen, "Reading "+child.Name())
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		total.Add(res)
		out.Emit(Entry{Name: child.Name(), Path: path, Dir: true, Bytes: res.Bytes, Files: res.Files})
		out.Progress(seen, "Reading "+child.Name())
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return summaryFor(root, total, largest), nil
}

// largestSet keeps the biggest files seen so far. It is a plain sorted slice
// because maxLargest is 25: a heap would be more code and no faster.
type largestSet struct {
	items []Largest
}

func (l *largestSet) add(f walk.File) {
	if len(l.items) == maxLargest && f.Size <= l.items[len(l.items)-1].Bytes {
		return
	}
	// A strictly-less comparison places a new file after any file of equal
	// size, so scanning the same tree twice gives the same list.
	at := sort.Search(len(l.items), func(i int) bool { return l.items[i].Bytes < f.Size })
	l.items = append(l.items, Largest{})
	copy(l.items[at+1:], l.items[at:])
	l.items[at] = Largest{Path: f.Path, Name: filepath.Base(f.Path), Bytes: f.Size}
	if len(l.items) > maxLargest {
		l.items = l.items[:maxLargest]
	}
}

func (l *largestSet) maps() []map[string]any {
	out := make([]map[string]any, 0, len(l.items))
	for _, item := range l.items {
		out = append(out, map[string]any{"path": item.Path, "name": item.Name, "bytes": item.Bytes})
	}
	return out
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(root string, res walk.Result, largest *largestSet) map[string]any {
	return map[string]any{
		"path":       root,
		"bytes":      res.Bytes,
		"files":      res.Files,
		"dirs":       res.Dirs,
		"unreadable": res.Unreadable,
		"skipped":    res.Skipped,
		"largest":    largest.maps(),
		"note":       noteFor(res),
	}
}

// noteFor explains anything that makes the total lower than the real figure. A
// tech who does not know a folder was missed will trust a number that is wrong,
// so this is not decoration.
func noteFor(res walk.Result) string {
	folders := count(res.Unreadable, "folder", "folders")
	items := count(res.Skipped, "item", "items")

	switch {
	case res.Unreadable > 0 && res.Skipped > 0:
		return "Could not open " + folders + ", and stepped over " + items +
			" (shortcuts, links and system folders), so the total is lower than the real figure."
	case res.Unreadable > 0:
		return "Could not open " + folders +
			", so the total is lower than the real figure. Those usually belong to another user or to Windows itself."
	case res.Skipped > 0:
		return "Stepped over " + items +
			": shortcuts, links and system folders are never followed, so nothing is counted twice."
	case res.Files == 0:
		return "That folder is empty."
	}
	return ""
}

// count writes "1 folder" or "3 folders", so a note never reads "1 folders".
func count(n int, singular, plural string) string {
	word := plural
	if n == 1 {
		word = singular
	}
	return strconv.Itoa(n) + " " + word
}
