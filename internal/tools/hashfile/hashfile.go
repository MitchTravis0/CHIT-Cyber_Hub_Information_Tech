// Package hashfile reads one file once and reports its MD5, SHA-1 and SHA-256,
// so a tech can check a download against the value the vendor published. It is
// the service layer the File Hash Checker page talks to.
package hashfile

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a file hash.
const JobKind = "hashfile"

// KindDigest is the result kind of the single item a hash job emits.
const KindDigest = "digest"

const (
	// readBuffer is the chunk io.CopyBuffer reads at a time. 1 MiB is large
	// enough that syscall overhead disappears on a spinning disk or an SMB
	// share, and small enough that a cancel is noticed within a few
	// milliseconds.
	readBuffer = 1 << 20

	// progressInterval throttles jc.Progress. core already coalesces events to
	// about ten a second, so anything under 100 ms would be thrown away.
	progressInterval = 200 * time.Millisecond
)

// Params is what the UI sends. Path is the only field; there is nothing here a
// user could set wrongly except the path itself.
type Params struct {
	Path string `json:"path"`
}

// Digests is the one result a hash job emits, after the whole file has been
// read. Every hash is lowercase hex, which is the form sha256sum, certutil and
// Get-FileHash all print (certutil and Get-FileHash print uppercase; the
// comparison lowercases both sides, see NormalizeExpected).
type Digests struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	MD5    string `json:"md5"`
	SHA1   string `json:"sha1"`
	SHA256 string `json:"sha256"`
}

// Service owns the hashing entry point. App forwards its bound method here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// validate catches everything a user can get wrong, so a bad path rejects the
// StartHash call instead of starting a job that immediately fails. The size it
// returns is the progress denominator.
func validate(p Params) (string, int64, error) {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return "", 0, core.Errorf(core.CodeInvalidInput,
			"Choose a file to check, or type the full path to it.")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, openError(path, err)
	}
	if info.IsDir() {
		return "", 0, core.Errorf(core.CodeInvalidInput,
			"%s is a folder, not a file. Pick the file inside it that you want to check.", path)
	}
	return path, info.Size(), nil
}

// openError turns a failed Stat or Open into the sentence a tech can act on.
func openError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return core.Errorf(core.CodeNotFound,
			"There is no file at %s. Check the path, or press Choose file and pick it.", path)
	case errors.Is(err, fs.ErrPermission):
		return core.Errorf(core.CodePermission,
			"This computer will not let CHIT look at %s. The file may belong to another user, or be locked by the program that is using it.", path)
	default:
		return core.Errorf(core.CodeInternal,
			"%s could not be opened. It may be on a drive or network share that is no longer connected.", path)
	}
}

// noteFor explains an unexpected read: an empty file, or one that grew or shrank
// while it was being hashed.
func noteFor(read, size int64) string {
	if read == 0 {
		return "That file is empty (0 bytes). These are the hashes of an empty file, which are the same for every empty file. If you were expecting content, the download or the copy did not finish."
	}
	if read != size {
		return "The file changed size while it was being read, so these hashes may not match the finished file. Wait until the download or the copy has finished, then check it again."
	}
	return ""
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(d Digests, size int64) map[string]any {
	return map[string]any{
		"path": d.Path, "name": d.Name, "bytes": d.Bytes,
		"md5": d.MD5, "sha1": d.SHA1, "sha256": d.SHA256,
		"note": noteFor(d.Bytes, size),
	}
}

// runHash is the body of the hash job, named so a test can drive it with a real
// JobContext and see what it hands back.
func runHash(jc *core.JobContext, path string, size int64) error {
	d, err := hashPath(jc.Ctx(), path, size, func(read, total int64) {
		jc.Progress(int(read), int(total), "Reading "+filepath.Base(path))
	})
	if err != nil {
		// A cancel must produce job:done with cancelled=true, not job:error.
		if errors.Is(err, context.Canceled) {
			return jc.Ctx().Err()
		}
		return err
	}
	jc.Emit(KindDigest, d)
	jc.SetSummary(summaryFor(d, size))
	return nil
}

// StartHash begins a hash and returns the job id at once. One "digest" item
// arrives on job:result when the whole file has been read.
func (s *Service) StartHash(p Params) (string, error) {
	path, size, err := validate(p)
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, int(size), func(jc *core.JobContext) error {
		return runHash(jc, path, size)
	}), nil
}

// progressReader is what makes the job cancellable and the bar move. It sits
// between the file and io.CopyBuffer so no byte is ever buffered twice.
type progressReader struct {
	r      io.Reader
	ctx    context.Context
	total  int64
	read   int64
	last   time.Time
	report func(read, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := p.r.Read(b)
	p.read += int64(n)
	if now := time.Now(); now.Sub(p.last) >= progressInterval {
		p.last = now
		p.report(p.read, p.total)
	}
	return n, err
}

// hashPath reads path once, feeding every byte into all three hashes at the
// same time. It never holds more than readBuffer bytes in memory, so a 100 GB
// file uses the same memory as a 1 KB one. report is called at most every
// progressInterval, plus once at the start and once at the end.
func hashPath(ctx context.Context, path string, size int64, report func(read, total int64)) (Digests, error) {
	f, err := os.Open(path)
	if err != nil {
		return Digests{}, openError(path, err)
	}
	defer f.Close()

	md5sum, sha1sum, sha256sum := md5.New(), sha1.New(), sha256.New()
	mw := io.MultiWriter(md5sum, sha1sum, sha256sum)

	pr := &progressReader{r: f, ctx: ctx, total: size, report: report}
	report(0, size)

	// io.CopyBuffer uses the buffer given rather than its own 32 KB one, which
	// is the whole reason for passing it.
	if _, err := io.CopyBuffer(mw, pr, make([]byte, readBuffer)); err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			// The one error left unwrapped, so the caller can tell a cancel
			// from a failure.
			return Digests{}, err
		case errors.Is(err, fs.ErrPermission):
			return Digests{}, core.Errorf(core.CodePermission,
				"This computer will not let CHIT look at %s. The file may belong to another user, or be locked by the program that is using it.", path)
		default:
			return Digests{}, core.Errorf(core.CodeInternal,
				"Reading %s stopped part way through, so there is no hash. The drive or the network share it is on may have disconnected.", filepath.Base(path))
		}
	}
	report(pr.read, size)

	return Digests{
		Path:   path,
		Name:   filepath.Base(path),
		Bytes:  pr.read,
		MD5:    hex.EncodeToString(md5sum.Sum(nil)),
		SHA1:   hex.EncodeToString(sha1sum.Sum(nil)),
		SHA256: hex.EncodeToString(sha256sum.Sum(nil)),
	}, nil
}
