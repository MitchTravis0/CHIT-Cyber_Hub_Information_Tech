package filedrop

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"chit/internal/core"
)

func write(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// recorder collects what a session emitted. It is mutex-guarded and has a
// wait, because a transfer is recorded on the handler goroutine after the last
// byte has gone out: the client can finish reading the body before the handler
// gets to record it. Reading the slice straight after an http.Get was racy and
// failed about one run in ten.
type recorder struct {
	mu   sync.Mutex
	list []Transfer
}

func (r *recorder) add(tr Transfer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = append(r.list, tr)
}

func (r *recorder) all() []Transfer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Transfer(nil), r.list...)
}

// wait blocks until n transfers have been recorded, or gives up and returns
// what there is so the assertion reports the real count.
func (r *recorder) wait(n int) []Transfer {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := r.all(); len(got) >= n {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	return r.all()
}

// live starts a real server on 127.0.0.1 with an operating-system chosen port,
// so nothing collides with a real port and nothing is reachable from off the
// machine. Loopback is not a network, so no -short skip is needed.
func live(t *testing.T, opts options) (string, string, *recorder, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	token, err := newToken()
	if err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	sess := &session{
		opts:  opts,
		token: token,
		out:   Sink{Emit: rec.add},
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = Serve(ctx, listener, sess)
	}()

	base := "http://" + listener.Addr().String()
	return base, token, rec, func() {
		cancel()
		<-stopped
	}
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

func TestSanitizeName(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"unix traversal", "../../etc/passwd", "passwd"},
		{"windows traversal", `..\..\win.ini`, "win.ini"},
		{"a bare dot", ".", "upload"},
		{"two dots", "..", "upload"},
		{"empty", "", "upload"},
		{"only spaces", "   ", "upload"},
		{"only dots and spaces", " . . ", "upload"},
		{"illegal characters", "a<b>c.txt", "a_b_c.txt"},
		{"a colon", "C:file.txt", "C_file.txt"},
		{"a pipe and a question mark", "a|b?c.txt", "a_b_c.txt"},
		{"a newline", "a\nb.txt", "a_b.txt"},
		{"a trailing dot, which Windows strips silently", "trailing.", "trailing"},
		{"a trailing space", "trailing ", "trailing"},
		{"a reserved device name", "CON", "_CON"},
		{"a reserved name with an extension", "con.txt", "_con.txt"},
		{"a serial port name", "COM1.log", "_COM1.log"},
		{"a printer port name", "LPT9", "_LPT9"},
		{"an ordinary name is left alone", "report.docx", "report.docx"},
		{"a name with spaces is left alone", "My Report (final).docx", "My Report (final).docx"},
		{"a leading dot file is kept", ".env", ".env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeName(tt.in); got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeNameTruncatesKeepingTheExtension(t *testing.T) {
	got := sanitizeName(strings.Repeat("a", 300) + ".txt")
	if len(got) != 200 {
		t.Errorf("length = %d, want 200", len(got))
	}
	if !strings.HasSuffix(got, ".txt") {
		t.Errorf("got %q, want the extension kept", got[len(got)-10:])
	}
}

func TestSanitizeNameNeverEscapesTheFolder(t *testing.T) {
	dir := t.TempDir()
	nasty := []string{
		"../../etc/passwd", `..\..\..\Windows\System32\drivers\etc\hosts`,
		"/etc/shadow", `C:\Windows\win.ini`, "....//....//x", "..",
	}
	for _, raw := range nasty {
		joined := filepath.Join(dir, sanitizeName(raw))
		if !strings.HasPrefix(joined, dir+string(filepath.Separator)) {
			t.Errorf("sanitizeName(%q) produced %q, which is outside %q", raw, joined, dir)
		}
	}
}

func TestUniqueName(t *testing.T) {
	dir := t.TempDir()

	if got := uniqueName(dir, "a.txt"); got != "a.txt" {
		t.Errorf("into an empty folder = %q, want a.txt", got)
	}

	write(t, dir, "a.txt", []byte("x"))
	if got := uniqueName(dir, "a.txt"); got != "a (1).txt" {
		t.Errorf("with a.txt present = %q, want a (1).txt", got)
	}

	write(t, dir, "a (1).txt", []byte("x"))
	if got := uniqueName(dir, "a.txt"); got != "a (2).txt" {
		t.Errorf("with two present = %q, want a (2).txt", got)
	}

	write(t, dir, "noext", []byte("x"))
	if got := uniqueName(dir, "noext"); got != "noext (1)" {
		t.Errorf("no extension = %q, want noext (1)", got)
	}

	write(t, dir, ".env", []byte("x"))
	// A dotfile's whole name is its extension to filepath.Ext, so the counter
	// goes in front of it rather than inside it.
	if got := uniqueName(dir, ".env"); got == ".env" {
		t.Error("an existing dotfile would have been overwritten")
	}
}

func TestNoPathFromRequest(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "shared.txt", []byte("the shared contents"))

	base, token, _, stop := live(t, options{
		shares: []Share{{Name: "shared.txt", Path: path, Bytes: 19}}, uploadLimit: maxUploadBytes,
	})
	defer stop()
	address := strings.TrimPrefix(base, "http://")

	// The one route that works.
	resp, body := get(t, base+"/d/"+token+"/f/0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index 0 returned %d, want 200", resp.StatusCode)
	}
	if string(body) != "the shared contents" {
		t.Errorf("body = %q", body)
	}

	// Everything that is not a plain in-range index is a 404, including every
	// shape of traversal. There is no path to traverse: files are served by
	// their position in a slice resolved before the listener opened.
	//
	// These go down a raw socket rather than through http.Client, because the
	// client rewrites "/a/../b" into "/b" before it ever leaves the machine and
	// would test its own path cleaning instead of the server's routing.
	for _, tail := range []string{
		"f/../../etc/passwd", "f/0/../..", "f/-1", "f/99", "f/abc", "f/", "f/0x0",
		"f/%2e%2e%2f%2e%2e%2fetc%2fpasswd", "f/1e0", "f/+0", "f/00000",
	} {
		status, body := rawGet(t, address, "/d/"+token+"/"+tail)
		if status != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404 (body %q)", tail, status, body)
		}
		if strings.Contains(body, "the shared contents") {
			t.Fatalf("%s served the file", tail)
		}
	}
}

// rawGet sends the request line byte for byte, so nothing normalises the path
// on the way.
func rawGet(t *testing.T, address, path string) (int, string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	request := "GET " + path + " HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}

	response := string(raw)
	fields := strings.Fields(response)
	if len(fields) < 2 {
		t.Fatalf("unreadable response: %q", response)
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("unreadable status in %q", response)
	}
	return status, response
}

func TestWrongTokenIsIndistinguishableFromAWrongPath(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "a.txt", []byte("x"))

	base, token, _, stop := live(t, options{
		shares: []Share{{Name: "a.txt", Path: path, Bytes: 1}}, uploadLimit: maxUploadBytes,
	})
	defer stop()

	// One character off.
	wrong := token[:len(token)-1] + "0"
	if wrong == token {
		wrong = token[:len(token)-1] + "1"
	}

	resp1, body1 := get(t, base+"/d/"+wrong)
	resp2, body2 := get(t, base+"/totally/unrelated")

	if resp1.StatusCode != resp2.StatusCode {
		t.Errorf("a wrong token gave %d and a wrong path gave %d: the server must not confirm a session exists",
			resp1.StatusCode, resp2.StatusCode)
	}
	if string(body1) != string(body2) {
		t.Errorf("bodies differ: %q vs %q", body1, body2)
	}
	if resp1.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp1.StatusCode)
	}
}

func TestTokenIsRandom(t *testing.T) {
	// 8 random bytes is 16 hex characters. The literal is written in so the
	// token cannot quietly shrink.
	const wantLength = 16
	if tokenBytes*2 != wantLength {
		t.Fatalf("tokenBytes*2 = %d, want %d", tokenBytes*2, wantLength)
	}

	seen := map[string]bool{}
	hex := regexp.MustCompile(`^[0-9a-f]+$`)
	for i := 0; i < 100; i++ {
		token, err := newToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(token) != wantLength {
			t.Fatalf("token %q is %d characters, want %d", token, len(token), wantLength)
		}
		if !hex.MatchString(token) {
			t.Fatalf("token %q is not lowercase hex", token)
		}
		if seen[token] {
			t.Fatalf("token %q came up twice in 100 draws", token)
		}
		seen[token] = true
	}
}

func TestIndexHTMLIsSelfContained(t *testing.T) {
	out := IndexHTML("abc", []Share{{Name: "a.txt", Bytes: 10}}, true)

	// The machine on the other end may have no internet at all, so nothing may
	// be fetched from anywhere.
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`src="https?:`),
		regexp.MustCompile(`href="https?:`),
		regexp.MustCompile(`src="//`),
		regexp.MustCompile(`href="//`),
		regexp.MustCompile(`@import`),
		regexp.MustCompile(`<script`),
		regexp.MustCompile(`url\(`),
	}
	for _, pattern := range forbidden {
		if pattern.MatchString(out) {
			t.Errorf("the served page matches %s, so it would fetch something", pattern)
		}
	}
}

func TestIndexHTMLEscapes(t *testing.T) {
	out := IndexHTML("abc", []Share{{Name: `<script>alert(1)</script>.txt`, Bytes: 1}}, false)

	if strings.Count(out, "<script") != 0 {
		t.Error("a file name injected a script tag into the served page")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("the file name was not escaped at all")
	}
}

func TestIndexHTMLUploadFormOnlyWhenAllowed(t *testing.T) {
	off := IndexHTML("abc", []Share{{Name: "a.txt"}}, false)
	if strings.Contains(off, "<form") {
		t.Error("the upload form is on the page although receiving is off")
	}

	on := IndexHTML("abc", []Share{{Name: "a.txt"}}, true)
	if !strings.Contains(on, `<form method="post" action="up" enctype="multipart/form-data">`) {
		t.Error("the upload form is missing or has the wrong shape")
	}
	if !strings.Contains(on, `name="file"`) {
		t.Error("the file input is not named file")
	}
}

// The served page is parsed by an XML parser rather than by the code that
// produced it, which is the independent check SPECS/CONVENTIONS.md asks for and
// the same trick Phase 4 used on the label SVG.
func TestIndexHTMLIsWellFormed(t *testing.T) {
	for _, allowUpload := range []bool{false, true} {
		out := IndexHTML("abc", []Share{
			{Name: "plain.txt", Bytes: 10},
			{Name: `awkward & "quoted" <name>.bin`, Bytes: 1 << 20},
		}, allowUpload)

		body := strings.TrimPrefix(out, "<!doctype html>\n")
		var any struct{}
		if err := xml.Unmarshal([]byte(body), &any); err != nil {
			t.Errorf("allowUpload=%v: the served page is not well formed: %v", allowUpload, err)
		}
	}
}

func TestIndexHTMLLinksEveryShareByIndex(t *testing.T) {
	shares := []Share{{Name: "a.txt"}, {Name: "b.txt"}, {Name: "c.txt"}}
	out := IndexHTML("abc", shares, false)

	for i := range shares {
		if !strings.Contains(out, `href="f/`+strconv.Itoa(i)+`"`) {
			t.Errorf("no link to index %d", i)
		}
	}
	if strings.Contains(out, `href="f/3"`) {
		t.Error("the page links to an index that does not exist")
	}
}

func TestIndexHTMLEmptyShareList(t *testing.T) {
	out := IndexHTML("abc", nil, false)
	if !strings.Contains(out, "There is nothing to download.") {
		t.Error("an empty share list does not say so")
	}
}

func TestDownloadEmitsTransfer(t *testing.T) {
	dir := t.TempDir()
	content := bytes.Repeat([]byte{7}, 4096)
	path := write(t, dir, "big.bin", content)

	base, token, transfers, stop := live(t, options{
		shares:      []Share{{Name: "big.bin", Path: path, Bytes: int64(len(content))}},
		uploadLimit: maxUploadBytes,
	})
	defer stop()

	resp, body := get(t, base+"/d/"+token+"/f/0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !bytes.Equal(body, content) {
		t.Error("the bytes served do not match the file")
	}
	if got := resp.Header.Get("Content-Disposition"); got != `attachment; filename="big.bin"` {
		t.Errorf("Content-Disposition = %q", got)
	}

	got := transfers.wait(1)
	if len(got) != 1 {
		t.Fatalf("emitted %d transfers, want 1: %+v", len(got), got)
	}
	tr := got[0]
	if tr.Direction != "download" || tr.Name != "big.bin" || tr.Status != "ok" {
		t.Errorf("transfer = %+v", tr)
	}
	if tr.Bytes != int64(len(content)) {
		t.Errorf("Bytes = %d, want %d", tr.Bytes, len(content))
	}
	if tr.Peer != "127.0.0.1" {
		t.Errorf("Peer = %q, want 127.0.0.1", tr.Peer)
	}
	if tr.Time == "" {
		t.Error("Time is empty")
	}
}

func postFile(t *testing.T, url, field, filename string, content []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	writer.Close()

	resp, err := http.Post(url, writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestUploadWritesFileAndEmits(t *testing.T) {
	shareDir := t.TempDir()
	receiveDir := t.TempDir()
	path := write(t, shareDir, "a.txt", []byte("x"))

	base, token, transfers, stop := live(t, options{
		shares:      []Share{{Name: "a.txt", Path: path, Bytes: 1}},
		allowUpload: true, receiveDir: receiveDir, uploadLimit: maxUploadBytes,
	})
	defer stop()

	content := []byte("sent from the other machine")
	resp := postFile(t, base+"/d/"+token+"/up", "file", "../../evil name.txt", content)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	entries, err := os.ReadDir(receiveDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("wrote %d files, want 1: %+v", len(entries), entries)
	}
	if entries[0].Name() != "evil name.txt" {
		t.Errorf("saved as %q, want the sanitized name", entries[0].Name())
	}

	onDisk, err := os.ReadFile(filepath.Join(receiveDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, content) {
		t.Error("the bytes on disk do not match what was sent")
	}

	got := transfers.wait(1)
	if len(got) != 1 {
		t.Fatalf("emitted %d transfers, want 1", len(got))
	}
	tr := got[0]
	if tr.Direction != "upload" || tr.Status != "ok" || tr.Name != "evil name.txt" {
		t.Errorf("transfer = %+v", tr)
	}
	if tr.Bytes != int64(len(content)) {
		t.Errorf("Bytes = %d, want %d", tr.Bytes, len(content))
	}
}

func TestUploadNeverOverwrites(t *testing.T) {
	shareDir := t.TempDir()
	receiveDir := t.TempDir()
	path := write(t, shareDir, "a.txt", []byte("x"))
	write(t, receiveDir, "notes.txt", []byte("something already here"))

	base, token, _, stop := live(t, options{
		shares:      []Share{{Name: "a.txt", Path: path, Bytes: 1}},
		allowUpload: true, receiveDir: receiveDir, uploadLimit: maxUploadBytes,
	})
	defer stop()

	postFile(t, base+"/d/"+token+"/up", "file", "notes.txt", []byte("new")).Body.Close()

	existing, err := os.ReadFile(filepath.Join(receiveDir, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(existing) != "something already here" {
		t.Error("an upload overwrote a file that was already there")
	}
	if _, err := os.Stat(filepath.Join(receiveDir, "notes (1).txt")); err != nil {
		t.Errorf("the upload was not saved under a free name: %v", err)
	}
}

func TestUploadRefusedWhenNotAllowed(t *testing.T) {
	shareDir := t.TempDir()
	receiveDir := t.TempDir()
	path := write(t, shareDir, "a.txt", []byte("x"))

	base, token, _, stop := live(t, options{
		shares:      []Share{{Name: "a.txt", Path: path, Bytes: 1}},
		allowUpload: false, receiveDir: receiveDir, uploadLimit: maxUploadBytes,
	})
	defer stop()

	resp := postFile(t, base+"/d/"+token+"/up", "file", "x.txt", []byte("nope"))
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	entries, _ := os.ReadDir(receiveDir)
	if len(entries) != 0 {
		t.Errorf("%d files were written although receiving is off", len(entries))
	}
}

func TestUploadOverLimitLeavesNothingBehind(t *testing.T) {
	// The real limit is 4 GiB, which no test is going to send, so the session
	// carries it as a field and this drives a small one. The constant itself is
	// pinned separately below.
	shareDir := t.TempDir()
	receiveDir := t.TempDir()
	path := write(t, shareDir, "a.txt", []byte("x"))

	base, token, transfers, stop := live(t, options{
		shares:      []Share{{Name: "a.txt", Path: path, Bytes: 1}},
		allowUpload: true, receiveDir: receiveDir, uploadLimit: 1024,
	})
	defer stop()

	resp := postFile(t, base+"/d/"+token+"/up", "file", "big.bin", bytes.Repeat([]byte{1}, 4096))
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	entries, _ := os.ReadDir(receiveDir)
	if len(entries) != 0 {
		t.Errorf("%d files were left behind after an over-size upload", len(entries))
	}
	if got := transfers.wait(1); len(got) != 1 || got[0].Status != "failed" {
		t.Errorf("transfers = %+v, want one failed row", got)
	}
}

func TestMaxUploadBytesIsFourGigabytes(t *testing.T) {
	const want = 4 << 30
	if maxUploadBytes != want {
		t.Errorf("maxUploadBytes = %d, want %d: the refusal message quotes this figure", maxUploadBytes, want)
	}
}

func TestUploadWithNoFilePart(t *testing.T) {
	shareDir := t.TempDir()
	receiveDir := t.TempDir()
	path := write(t, shareDir, "a.txt", []byte("x"))

	base, token, transfers, stop := live(t, options{
		shares:      []Share{{Name: "a.txt", Path: path, Bytes: 1}},
		allowUpload: true, receiveDir: receiveDir, uploadLimit: maxUploadBytes,
	})
	defer stop()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("notafile", "value")
	writer.Close()

	resp, err := http.Post(base+"/d/"+token+"/up", writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if got := transfers.wait(1); len(got) != 1 || got[0].Message != "The upload did not contain a file." {
		t.Errorf("transfers = %+v", got)
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "a.txt", []byte("x"))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()

	sess := &session{
		opts:  options{shares: []Share{{Name: "a.txt", Path: path, Bytes: 1}}, uploadLimit: maxUploadBytes},
		token: "abc",
		out:   Sink{Emit: func(Transfer) {}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan error, 1)
	go func() { returned <- Serve(ctx, listener, sess) }()

	// Prove it is up before stopping it.
	if _, err := http.Get("http://" + address + "/d/abc"); err != nil {
		t.Fatalf("the server did not come up: %v", err)
	}

	cancel()
	select {
	case err := <-returned:
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Serve returned %v, want the context error so job:done says cancelled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return within 10 seconds of a cancel")
	}

	// Nothing is left listening.
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err == nil {
		conn.Close()
		t.Error("the port is still open after Stop")
	}
}

func TestValidateParams(t *testing.T) {
	dir := t.TempDir()
	one := write(t, dir, "a.txt", []byte("x"))
	receive := t.TempDir()

	many := make([]string, 51)
	for i := range many {
		many[i] = write(t, dir, "f"+strconv.Itoa(i)+".txt", []byte("x"))
	}

	tests := []struct {
		name     string
		params   Params
		wantErr  bool
		wantPort int
		wantMsg  string
	}{
		{name: "one file, default port", params: Params{Files: []string{one}}, wantPort: 8722},
		{name: "the lowest allowed port", params: Params{Files: []string{one}, Port: 1024}, wantPort: 1024},
		{name: "the highest allowed port", params: Params{Files: []string{one}, Port: 65535}, wantPort: 65535},
		{
			name: "one below the floor", params: Params{Files: []string{one}, Port: 1023}, wantErr: true,
			wantMsg: "The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{
			name: "above the ceiling", params: Params{Files: []string{one}, Port: 65536}, wantErr: true,
			wantMsg: "The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.",
		},
		{name: "no files", params: Params{}, wantErr: true, wantMsg: "Choose at least one file to share."},
		{
			name: "fifty is allowed", params: Params{Files: many[:50]}, wantPort: 8722,
		},
		{
			name: "fifty one is not", params: Params{Files: many}, wantErr: true,
			wantMsg: "That is 51 files. Share at most 50 at a time, or put them in a zip first.",
		},
		{
			name: "a missing file", params: Params{Files: []string{filepath.Join(dir, "gone.txt")}},
			wantErr: true,
			wantMsg: "There is no file at " + filepath.Join(dir, "gone.txt") + " any more. Remove it from the list and try again.",
		},
		{
			name: "a folder in the list", params: Params{Files: []string{dir}}, wantErr: true,
			wantMsg: dir + " is a folder. Pick the files inside it: whole folders are not shared.",
		},
		{
			name:    "receiving on with no folder",
			params:  Params{Files: []string{one}, AllowUpload: true},
			wantErr: true, wantMsg: "Choose the folder that files sent to you should land in.",
		},
		{
			name:   "receiving on with a folder",
			params: Params{Files: []string{one}, AllowUpload: true, ReceiveDir: receive}, wantPort: 8722,
		},
		{
			name:    "a receive folder that is a file",
			params:  Params{Files: []string{one}, AllowUpload: true, ReceiveDir: one},
			wantErr: true,
			wantMsg: one + " is not a folder, so files sent to you have nowhere to land.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := tt.params.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("validate accepted bad params")
				}
				if got := core.MessageOf(err); got != tt.wantMsg {
					t.Errorf("message =\n%q\nwant\n%q", got, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if opts.port != tt.wantPort {
				t.Errorf("port = %d, want %d", opts.port, tt.wantPort)
			}
		})
	}
}

func TestPortInUseRejectsBeforeTheJobStarts(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "a.txt", []byte("x"))

	// Take a port, then ask for it.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	_, portText, _ := net.SplitHostPort(blocker.Addr().String())
	port, _ := strconv.Atoi(portText)

	jobs := core.NewJobManager()
	service := &Service{jobs: jobs, live: map[string]Session{}}

	id, err := service.StartDrop(Params{Files: []string{path}, Port: port}, "")
	if err == nil {
		t.Fatal("StartDrop accepted a port that is already taken")
	}
	if id != "" {
		t.Errorf("job id = %q, want none: the port is bound before the job starts", id)
	}
	want := "Something else on this computer is already using port " + portText +
		". Change the port and start again."
	if got := core.MessageOf(err); got != want {
		t.Errorf("message =\n%q\nwant\n%q", got, want)
	}
	if running := jobs.Running(); running != 0 {
		t.Errorf("%d jobs are running, want none", running)
	}
}

func TestAddressesExcludesLoopbackAndLinkLocal(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.44", true},
		{"10.0.0.5", true},
		{"172.16.3.9", true},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip).To4()
		if ip == nil {
			t.Fatalf("%s did not parse as IPv4", tt.ip)
		}
		if got := usableForSharing(ip); got != tt.want {
			t.Errorf("usableForSharing(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestURLFor(t *testing.T) {
	if got := URLFor("192.168.1.44", 8722, "k3f9x2qp"); got != "http://192.168.1.44:8722/d/k3f9x2qp" {
		t.Errorf("URLFor = %q", got)
	}
	// An IPv6 literal has to be bracketed or the port reads as part of it.
	if got := URLFor("fd00::1", 8722, "x"); got != "http://[fd00::1]:8722/d/x" {
		t.Errorf("URLFor for IPv6 = %q", got)
	}
}

func TestSessionNote(t *testing.T) {
	want := "Nothing was transferred. If the other machine could not reach the address, check that both machines are on the same network and that this computer's firewall allowed CHIT through."
	if got := sessionNote(0, 0); got != want {
		t.Errorf("sessionNote(0,0) =\n%q\nwant\n%q", got, want)
	}
	if got := sessionNote(1, 0); got != "" {
		t.Errorf("sessionNote(1,0) = %q, want empty", got)
	}
	if got := sessionNote(0, 1); got != "" {
		t.Errorf("sessionNote(0,1) = %q, want empty", got)
	}
}

func TestSummaryKeys(t *testing.T) {
	got := summaryFor("http://x/d/y", 8722, 1, 0, 0, 0, 0)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	want := []string{"bytesIn", "bytesOut", "downloads", "files", "note", "port", "uploads", "url"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("summary keys = %v, want %v", keys, want)
	}
}

func TestSessionIsRememberedThenForgotten(t *testing.T) {
	dir := t.TempDir()
	path := write(t, dir, "a.txt", []byte("x"))

	jobs := core.NewJobManager()
	service := &Service{jobs: jobs, live: map[string]Session{}}

	id, err := service.StartDrop(Params{Files: []string{path}}, "")
	if err != nil {
		t.Skipf("this machine has no shareable network address: %v", err)
	}

	session := service.SessionFor(id)
	if session.Token == "" || session.URL == "" {
		t.Fatalf("the session was not remembered: %+v", session)
	}
	if session.Files != 1 {
		t.Errorf("Files = %d, want 1", session.Files)
	}
	if !strings.HasSuffix(session.URL, "/d/"+session.Token) {
		t.Errorf("URL %q does not end with the token", session.URL)
	}

	if err := jobs.Cancel(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for jobs.Running() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the drop did not stop within 10 seconds")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Give the deferred forget a moment after the job body returns.
	time.Sleep(50 * time.Millisecond)

	if after := service.SessionFor(id); after.Token != "" {
		t.Errorf("the session is still remembered after the job ended: %+v", after)
	}
}

func TestSessionForUnknownJob(t *testing.T) {
	service := &Service{jobs: core.NewJobManager(), live: map[string]Session{}}
	if got := service.SessionFor("job-nope"); got.Token != "" {
		t.Errorf("SessionFor an unknown job = %+v, want an empty Session", got)
	}
}

func TestNoFileServerAnywhereInThisPackage(t *testing.T) {
	// The security design is that no filesystem path is ever built from a
	// request. A grep is a blunt check and it is the right one.
	for _, name := range []string{"filedrop.go", "serve.go", "page.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"http.FileServer", "http.Dir", "http.ServeFile"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("%s contains %q: a file must be served by index, never by path", name, forbidden)
			}
		}
	}
}

func TestHeaderSafe(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain.txt", "plain.txt"},
		{`a"b.txt`, "a_b.txt"},
		{`a\b.txt`, "a_b.txt"},
		{"a\nb.txt", "a_b.txt"},
		{"a\rb.txt", "a_b.txt"},
	}
	for _, tt := range tests {
		if got := headerSafe(tt.in); got != tt.want {
			t.Errorf("headerSafe(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.in); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
