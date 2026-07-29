// Package dupfind finds files whose contents are byte-for-byte identical, so a
// tech can see what is worth deleting. It reads only: nothing in here removes,
// moves or renames a file. It is the service layer the Duplicate File Finder
// page talks to.
package dupfind

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"chit/internal/core"
	"chit/internal/walk"
)

// JobKind is the job manager kind for a duplicate scan.
const JobKind = "dupfind"

// KindGroup is the result kind of every confirmed group of identical files.
const KindGroup = "group"

const (
	// quickBytes is how much of each file the second stage reads. 64 KiB is
	// enough to separate files that merely share a size (media containers put
	// their differing metadata in the first few KB) and is one disk read.
	quickBytes = 64 << 10

	// readBuffer is the chunk size for the full hash, matching hashfile.
	readBuffer = 1 << 20

	// maxGroups caps the result set. A tree with more than this many groups is
	// a backup drive, and the tech needs the biggest offenders, not all of them.
	maxGroups = 5000

	// defaultMinBytes ignores the small clutter nobody reclaims space from.
	defaultMinBytes = 1024

	// maxMinBytes is a sanity bound on the smallest-size option.
	maxMinBytes = 1 << 40 // 1 TiB

	// progressInterval throttles progress reports; core coalesces to ~10/sec.
	progressInterval = 200 * time.Millisecond
)

// Params is what the UI sends. MinBytes of zero means the default.
type Params struct {
	Path     string `json:"path"`
	MinBytes int64  `json:"minBytes"`
}

// File is one member of a duplicate group.
type File struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Modified string `json:"modified"`
}

// Group is a set of files with identical contents.
type Group struct {
	// Hash is the full SHA-256 of the contents, lowercase hex. It doubles as
	// the group's stable id in the UI.
	Hash  string `json:"hash"`
	Bytes int64  `json:"bytes"`
	Count int    `json:"count"`
	// Waste is Bytes * (Count - 1): what deleting the extras would give back.
	Waste int64  `json:"waste"`
	Files []File `json:"files"`
}

// Sink is where a scan reports to. The job wires it to jc.Emit and jc.Progress;
// a test wires it to a slice, which is the only way to read what a scan emitted
// without a Wails runtime.
type Sink struct {
	Emit     func(Group)
	Progress func(done, total int, message string)
}

// validate catches everything a user can get wrong, so bad input rejects the
// StartScan call instead of starting a job that immediately fails.
func (p Params) validate() (string, int64, error) {
	root, err := walk.Root(p.Path)
	if err != nil {
		return "", 0, err
	}
	if p.MinBytes < 0 || p.MinBytes > maxMinBytes {
		return "", 0, core.Errorf(core.CodeInvalidInput,
			"The smallest file size to compare must be between 1 byte and 1 TB.")
	}
	// An empty file is identical to every other empty file, which is true and
	// useless, so the effective minimum can never reach zero: a zero becomes
	// the default and anything negative was rejected above. A further floor
	// here would be unreachable, and TestValidateNeverReturnsAMinimumBelowOne
	// pins the property rather than the dead branch.
	minBytes := p.MinBytes
	if minBytes == 0 {
		minBytes = defaultMinBytes
	}
	return root, minBytes, nil
}

// Service owns the scanning entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
	// openFile is swapped in tests to count how much of each file was read, so
	// "stage two avoided a full read" can be proven rather than assumed.
	openFile func(string) (io.ReadCloser, error)
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs, openFile: openReal}
}

func openReal(path string) (io.ReadCloser, error) { return os.Open(path) }

// StartScan begins a scan and returns the job id at once. Groups arrive as
// "group" items on job:result, biggest files first, so stopping early still
// leaves the results worth acting on.
func (s *Service) StartScan(p Params) (string, error) {
	root, minBytes, err := p.validate()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		return s.runScan(jc, root, minBytes)
	}), nil
}

// runScan is the body of the job, named so it is not stranded at 0% coverage
// inside an anonymous closure.
func (s *Service) runScan(jc *core.JobContext, root string, minBytes int64) error {
	summary, err := s.Scan(jc.Ctx(), root, minBytes, Sink{
		Emit:     func(g Group) { jc.Emit(KindGroup, g) },
		Progress: func(done, total int, message string) { jc.Progress(done, total, message) },
	})
	if err != nil {
		return err
	}
	jc.SetSummary(summary)
	return nil
}

// Scan runs the three stages and returns the job:done summary.
//
//	stage 1  walk the tree, grouping by exact byte size. A size with one file
//	         cannot have a duplicate and is dropped without being opened.
//	stage 2  hash the first quickBytes of each candidate. Files whose prefix
//	         differs cannot be identical and are dropped without a full read.
//	stage 3  hash each survivor in full and group by digest.
//
// Sizes are processed largest first, so the biggest wins arrive first and a
// tech who stops after thirty seconds already has the answer worth acting on.
func (s *Service) Scan(ctx context.Context, root string, minBytes int64, out Sink) (map[string]any, error) {
	bySize := map[int64][]walk.File{}
	scanned := 0

	res, err := walk.Walk(ctx, walk.Options{Root: root, MinSize: minBytes}, func(f walk.File) error {
		bySize[f.Size] = append(bySize[f.Size], f)
		scanned++
		if scanned%500 == 0 {
			out.Progress(scanned, 0, "Listing files in "+filepath.Base(root))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sizes := make([]int64, 0, len(bySize))
	candidates := 0
	for size, files := range bySize {
		if len(files) < 2 {
			delete(bySize, size)
			continue
		}
		sizes = append(sizes, size)
		candidates += len(files)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] > sizes[j] })

	var groups, duplicates int
	var waste int64
	capped := false
	compared := 0
	lastProgress := time.Time{}

	for _, size := range sizes {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if capped {
			break
		}

		for _, bucket := range s.byPrefix(ctx, bySize[size], size, &compared, candidates, out, &lastProgress) {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			for _, group := range s.byFullHash(ctx, bucket, size) {
				out.Emit(group)
				groups++
				duplicates += group.Count - 1
				waste += group.Waste
				if groups >= maxGroups {
					capped = true
					break
				}
			}
			if capped {
				break
			}
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return summaryFor(root, sums{
		scanned: scanned, bytes: res.Bytes, groups: groups, duplicates: duplicates,
		waste: waste, unreadable: res.Unreadable, skipped: res.Skipped, capped: capped,
	}), nil
}

// byPrefix splits one size group by the hash of each file's first quickBytes.
// Files at or below quickBytes skip this stage: the prefix would be the whole
// file, and hashing it twice is waste.
func (s *Service) byPrefix(ctx context.Context, files []walk.File, size int64, compared *int, total int, out Sink, last *time.Time) [][]walk.File {
	if size <= quickBytes {
		return [][]walk.File{files}
	}

	buckets := map[string][]walk.File{}
	order := []string{}
	for _, f := range files {
		if ctx.Err() != nil {
			return nil
		}
		digest, err := s.hashPrefix(ctx, f.Path, quickBytes)
		*compared++
		if now := time.Now(); now.Sub(*last) >= progressInterval {
			*last = now
			out.Progress(*compared, total, "Comparing "+filepath.Base(f.Path))
		}
		if err != nil {
			continue
		}
		if _, seen := buckets[digest]; !seen {
			order = append(order, digest)
		}
		buckets[digest] = append(buckets[digest], f)
	}

	out2 := make([][]walk.File, 0, len(order))
	for _, digest := range order {
		if len(buckets[digest]) >= 2 {
			out2 = append(out2, buckets[digest])
		}
	}
	return out2
}

// byFullHash groups files by the SHA-256 of their whole contents.
func (s *Service) byFullHash(ctx context.Context, files []walk.File, size int64) []Group {
	buckets := map[string][]walk.File{}
	order := []string{}
	for _, f := range files {
		if ctx.Err() != nil {
			return nil
		}
		digest, err := s.hashPrefix(ctx, f.Path, -1)
		if err != nil {
			// A file locked or deleted since the walk simply leaves its group.
			continue
		}
		if _, seen := buckets[digest]; !seen {
			order = append(order, digest)
		}
		buckets[digest] = append(buckets[digest], f)
	}

	groups := make([]Group, 0, len(order))
	for _, digest := range order {
		bucket := buckets[digest]
		if len(bucket) < 2 {
			continue
		}
		files := make([]File, 0, len(bucket))
		for _, f := range bucket {
			files = append(files, File{
				Path:     f.Path,
				Name:     filepath.Base(f.Path),
				Modified: f.ModTime.UTC().Format(time.RFC3339),
			})
		}
		groups = append(groups, Group{
			Hash:  digest,
			Bytes: size,
			Count: len(bucket),
			Waste: size * int64(len(bucket)-1),
			Files: files,
		})
	}
	return groups
}

// hashPrefix returns the SHA-256 of the first limit bytes of a file, or of the
// whole file when limit is negative.
func (s *Service) hashPrefix(ctx context.Context, path string, limit int64) (string, error) {
	f, err := s.openFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var src io.Reader = f
	if limit >= 0 {
		src = io.LimitReader(f, limit)
	}

	sum := sha256.New()
	buf := make([]byte, readBuffer)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := src.Read(buf)
		if n > 0 {
			sum.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

type sums struct {
	scanned, groups, duplicates, unreadable, skipped int
	bytes, waste                                     int64
	capped                                           bool
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(root string, s sums) map[string]any {
	return map[string]any{
		"path":       root,
		"scanned":    s.scanned,
		"bytes":      s.bytes,
		"groups":     s.groups,
		"duplicates": s.duplicates,
		"waste":      s.waste,
		"unreadable": s.unreadable,
		"skipped":    s.skipped,
		"capped":     s.capped,
		"note":       noteFor(s),
	}
}

// noteFor explains an empty or truncated result, so an empty table never reads
// as a broken tool.
func noteFor(s sums) string {
	switch {
	case s.capped:
		return "Stopped after the first " + strconv.Itoa(maxGroups) +
			" groups. Those are the biggest ones; narrow the folder or raise the smallest size to see the rest."
	case s.groups == 0 && s.scanned == 0:
		return "No files that size or larger were found in that folder. Lower the smallest size and look again."
	case s.groups == 0:
		return "No two files in that folder have identical contents. Files with the same name but different contents are not duplicates, and this tool compares contents."
	case s.unreadable > 0 && s.skipped > 0:
		return "Could not open " + count(s.unreadable, "folder", "folders") +
			", and stepped over " + count(s.skipped, "item", "items") +
			" (shortcuts, links and system folders), so some duplicates may be missing."
	case s.unreadable > 0:
		return "Could not open " + count(s.unreadable, "folder", "folders") +
			", so some duplicates may be missing. Those usually belong to another user or to Windows itself."
	case s.skipped > 0:
		return "Stepped over " + count(s.skipped, "item", "items") +
			": shortcuts and links are never followed, so a file is never reported as a copy of itself."
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
