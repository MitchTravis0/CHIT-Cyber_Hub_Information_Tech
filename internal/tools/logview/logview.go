// Package logview reads a log file a window at a time, so a multi-gigabyte file
// opens as fast as a small one, and searches the whole of it as a cancellable
// job. It opens files read-only and never writes.
package logview

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a log search.
const JobKind = "logsearch"

// KindMatch is the result kind of every matching line a search emits.
const KindMatch = "match"

const (
	// defaultLines is one window. 500 lines is about ten screens, which is
	// enough to scroll around in without the page becoming slow.
	defaultLines = 500
	maxLines     = 5000

	// maxWindowBytes stops a file with no newlines at all (a minified blob, a
	// mis-detected binary) from pulling gigabytes into one window.
	maxWindowBytes = 8 << 20

	// maxLineBytes truncates a single absurd line, because one 300 MB line
	// would freeze the webview.
	maxLineBytes = 4000

	// sniffBytes is how much OpenLog reads to decide text or binary.
	sniffBytes = 8 << 10

	// blockBytes is the chunk a backwards read walks in when it counts
	// newlines from the end of the file.
	blockBytes = 64 << 10

	// maxQuery keeps a pasted stack trace out of the search box.
	maxQuery = 1000
)

// Where values, the only four ReadLog accepts.
const (
	WhereHead   = "head"
	WhereTail   = "tail"
	WhereAt     = "at"
	WhereBefore = "before"
)

// Info describes an opened file.
type Info struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	Modified string `json:"modified"`
	// CRLF is true when the first newline found was preceded by a carriage
	// return, so the page can say "Windows line endings" rather than showing a
	// stray character.
	CRLF bool `json:"crlf"`
}

// Line is one line of the file.
type Line struct {
	// Number is 1-based and exact only when the window was read from the start
	// of the file. A tail window cannot know it without counting the whole
	// file, so Number is 0 there and the page shows the byte offset instead.
	Number int    `json:"number"`
	Offset int64  `json:"offset"`
	Text   string `json:"text"`
	Level  string `json:"level"`
	// Truncated is true when the line was longer than maxLineBytes.
	Truncated bool `json:"truncated"`
}

// Chunk is one window of lines out of the file.
type Chunk struct {
	Lines []Line `json:"lines"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Bytes int64  `json:"bytes"`
	// AtStart and AtEnd tell the page whether the Older and Newer buttons do
	// anything, so it can disable them rather than paging into nothing.
	AtStart bool `json:"atStart"`
	AtEnd   bool `json:"atEnd"`
	// Shrank is true when the file is now smaller than the offset asked for,
	// which is what log rotation looks like from here.
	Shrank bool `json:"shrank"`
}

// ReadParams is what the page sends for one window.
type ReadParams struct {
	Path   string `json:"path"`
	Where  string `json:"where"`
	Offset int64  `json:"offset"`
	Lines  int    `json:"lines"`
}

// SearchParams is what the page sends for a whole-file search.
type SearchParams struct {
	Path      string `json:"path"`
	Query     string `json:"query"`
	MatchCase bool   `json:"matchCase"`
}

// Match is one line that contained the query.
type Match struct {
	Number int    `json:"number"`
	Offset int64  `json:"offset"`
	Text   string `json:"text"`
	Level  string `json:"level"`
	// Col is the byte index of the first hit inside Text, so the page can mark
	// it without searching again and without disagreeing about case folding.
	Col       int  `json:"col"`
	Truncated bool `json:"truncated"`
}

// refusedExtensions are files that are not text and never will be. Refusing
// them by name is kinder than showing a screen of mojibake.
var refusedExtensions = map[string]string{
	".evtx": "%s is a Windows event log, which is a database rather than a text file. Open it with Event Viewer, or export it to .txt from there and open that here.",
	".gz":   "%s is a compressed archive. Extract it first, then open the log file inside it.",
	".zip":  "%s is a compressed archive. Extract it first, then open the log file inside it.",
	".7z":   "%s is a compressed archive. Extract it first, then open the log file inside it.",
	".bz2":  "%s is a compressed archive. Extract it first, then open the log file inside it.",
	".xz":   "%s is a compressed archive. Extract it first, then open the log file inside it.",
}

// Open checks that a file can be read as text and describes it. It reads at
// most sniffBytes, so a 4 GB file opens as fast as a 4 KB one.
func Open(path string) (Info, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Info{}, core.Errorf(core.CodeInvalidInput,
			"Choose a log file, or type the full path to it.")
	}

	info, err := os.Stat(path)
	if err != nil {
		return Info{}, openError(path, err)
	}
	if info.IsDir() {
		return Info{}, core.Errorf(core.CodeInvalidInput,
			"%s is a folder, not a file. Pick the log file inside it.", path)
	}

	if message, refused := refusedExtensions[strings.ToLower(filepath.Ext(path))]; refused {
		return Info{}, core.Errorf(core.CodeInvalidInput, message, path)
	}

	f, err := os.Open(path)
	if err != nil {
		return Info{}, openError(path, err)
	}
	defer f.Close()

	head := make([]byte, sniffBytes)
	n, err := f.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		return Info{}, openError(path, err)
	}
	head = head[:n]

	for _, b := range head {
		if b == 0 {
			return Info{}, core.Errorf(core.CodeInvalidInput,
				"%s does not look like a text file, so there is nothing readable to show. Log files are plain text; this one is a program, a database or a compressed archive.", path)
		}
	}

	return Info{
		Path:     path,
		Name:     filepath.Base(path),
		Bytes:    info.Size(),
		Modified: info.ModTime().UTC().Format(time.RFC3339),
		CRLF:     firstNewlineIsCRLF(head),
	}, nil
}

func firstNewlineIsCRLF(head []byte) bool {
	for i, b := range head {
		if b == '\n' {
			return i > 0 && head[i-1] == '\r'
		}
	}
	return false
}

// openError turns a failed Stat or Open into the sentence a tech can act on.
func openError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return core.Errorf(core.CodeNotFound,
			"There is no file at %s. Check the path, or press Choose file and pick it.", path)
	case errors.Is(err, fs.ErrPermission):
		return core.Errorf(core.CodePermission,
			"This computer will not let CHIT read %s. The file may belong to another user, or be locked by the program writing to it.", path)
	default:
		return core.Errorf(core.CodeInternal,
			"%s could not be opened. It may be on a drive or network share that is no longer connected.", path)
	}
}

// validateRead catches everything a user can get wrong before any file is
// touched.
func (p ReadParams) validate() (ReadParams, error) {
	if strings.TrimSpace(p.Path) == "" {
		return p, core.Errorf(core.CodeInvalidInput,
			"Choose a log file, or type the full path to it.")
	}
	if p.Where == "" {
		p.Where = WhereTail
	}
	switch p.Where {
	case WhereHead, WhereTail, WhereAt, WhereBefore:
	default:
		return p, core.Errorf(core.CodeInvalidInput,
			"That is not a place in the file. Use the Start, End, Older or Newer buttons.")
	}
	if p.Lines < 0 || p.Lines > maxLines {
		return p, core.Errorf(core.CodeInvalidInput,
			"Show between 1 and %d lines at a time.", maxLines)
	}
	if p.Lines == 0 {
		p.Lines = defaultLines
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p, nil
}

// Read returns one window of lines out of the file.
func Read(p ReadParams) (Chunk, error) {
	p, err := p.validate()
	if err != nil {
		return Chunk{}, err
	}

	f, err := os.Open(p.Path)
	if err != nil {
		return Chunk{}, openError(p.Path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return Chunk{}, openError(p.Path, err)
	}
	size := stat.Size()

	shrank := false
	if p.Offset > size {
		p.Offset = size
		shrank = true
	}

	var from int64
	numbered := false
	// until bounds a "before" read so it stops where the current window began,
	// instead of reading straight back over it.
	until := size

	switch p.Where {
	case WhereHead:
		from, numbered = 0, true
	case WhereTail:
		from, err = lineStartBefore(f, size, size, p.Lines)
		if err != nil {
			return Chunk{}, openError(p.Path, err)
		}
		numbered = from == 0
	case WhereAt:
		from, err = lineStartAt(f, p.Offset)
		if err != nil {
			return Chunk{}, openError(p.Path, err)
		}
		numbered = from == 0
	case WhereBefore:
		from, err = lineStartBefore(f, size, p.Offset, p.Lines)
		if err != nil {
			return Chunk{}, openError(p.Path, err)
		}
		numbered = from == 0
		until = p.Offset
	}

	lines, end, err := readForward(f, from, p.Lines, numbered, until)
	if err != nil {
		return Chunk{}, openError(p.Path, err)
	}

	return Chunk{
		Lines:   lines,
		Start:   from,
		End:     end,
		Bytes:   size,
		AtStart: from == 0,
		AtEnd:   end >= size,
		Shrank:  shrank,
	}, nil
}

// lineStartAt backs up from offset to the start of the line it falls inside, so
// a window never begins half way through a line.
func lineStartAt(f *os.File, offset int64) (int64, error) {
	if offset <= 0 {
		return 0, nil
	}
	buf := make([]byte, 1)
	for at := offset - 1; at >= 0; at-- {
		if _, err := f.ReadAt(buf, at); err != nil {
			return 0, err
		}
		if buf[0] == '\n' {
			return at + 1, nil
		}
		// Backing up more than one line's worth means the file has no newlines
		// nearby; start where we were asked rather than rewinding for ever.
		if offset-at > maxLineBytes {
			return offset, nil
		}
	}
	return 0, nil
}

// lineStartBefore finds the byte offset of the first of the last `lines` lines
// that end at or before `until`, by walking backwards in blocks and counting
// newlines. This is what makes opening the end of a 4 GB file instant.
func lineStartBefore(f *os.File, size, until int64, lines int) (int64, error) {
	if until <= 0 {
		return 0, nil
	}
	if until > size {
		until = size
	}

	// A file ending in a newline has one final empty line that nobody wants
	// counted, so the search starts just before it.
	end := until
	if end > 0 {
		one := make([]byte, 1)
		if _, err := f.ReadAt(one, end-1); err != nil {
			return 0, err
		}
		if one[0] == '\n' {
			end--
		}
	}

	needed := lines
	at := end
	buf := make([]byte, blockBytes)
	for at > 0 {
		from := at - int64(len(buf))
		if from < 0 {
			from = 0
		}
		block := buf[:at-from]
		if _, err := f.ReadAt(block, from); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		for i := len(block) - 1; i >= 0; i-- {
			if block[i] != '\n' {
				continue
			}
			needed--
			if needed == 0 {
				return from + int64(i) + 1, nil
			}
		}
		at = from
	}
	return 0, nil
}

// readForward reads up to `lines` lines starting at `from` and stopping before
// byte `until`. It also stops at maxWindowBytes, so a file with no newlines
// cannot pull gigabytes into memory.
func readForward(f *os.File, from int64, lines int, numbered bool, until int64) ([]Line, int64, error) {
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return nil, from, err
	}

	out := make([]Line, 0, lines)
	buf := make([]byte, blockBytes)
	var pending []byte
	offset := from
	consumed := int64(0)
	number := 0
	if numbered {
		number = 1
	}

	room := func() bool { return len(out) < lines && offset < until }

	flush := func(raw []byte, lineBytes int64) {
		text, truncated := clean(raw)
		line := Line{Offset: offset, Text: text, Level: Level(text), Truncated: truncated}
		if numbered {
			line.Number = number
			number++
		}
		out = append(out, line)
		offset += lineBytes
	}

	for room() && consumed < maxWindowBytes {
		n, err := f.Read(buf)
		if n > 0 {
			consumed += int64(n)
			chunk := buf[:n]
			for len(chunk) > 0 && room() {
				at := indexByte(chunk, '\n')
				if at < 0 {
					pending = append(pending, chunk...)
					break
				}
				pending = append(pending, chunk[:at]...)
				flush(pending, int64(len(pending))+1)
				pending = pending[:0]
				chunk = chunk[at+1:]
			}
		}
		if err != nil {
			break
		}
	}

	// A last line with no trailing newline is still a line.
	if len(pending) > 0 && room() {
		flush(pending, int64(len(pending)))
	}

	return out, offset, nil
}

// trimNewline drops the line terminator ReadBytes leaves on, so the search and
// the window reader agree about where a line ends.
func trimNewline(raw []byte) []byte {
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		return raw[:len(raw)-1]
	}
	return raw
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// clean strips a trailing carriage return and a leading byte order mark, and
// cuts a line that is longer than maxLineBytes.
func clean(raw []byte) (string, bool) {
	text := string(raw)
	text = strings.TrimSuffix(text, "\r")
	// Written as an escape on purpose: a byte order mark typed literally into a
	// Go source file is a compile error.
	text = strings.TrimPrefix(text, "\uFEFF")
	if len(text) > maxLineBytes {
		return text[:maxLineBytes], true
	}
	return text, false
}
