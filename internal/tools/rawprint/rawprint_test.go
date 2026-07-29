package rawprint

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalizeParams(t *testing.T) {
	tests := []struct {
		name        string
		in          Params
		wantPort    int
		wantTimeout int
		wantErr     string
	}{
		{"defaults", Params{Host: "192.168.1.50"}, 9100, 5000, ""},
		{"trimmed host", Params{Host: "  192.168.1.50  "}, 9100, 5000, ""},
		{"explicit port", Params{Host: "p", Port: 9101}, 9101, 5000, ""},
		{"lowest port", Params{Host: "p", Port: 1}, 1, 5000, ""},
		{"highest port", Params{Host: "p", Port: 65535}, 65535, 5000, ""},
		{"port above the range", Params{Host: "p", Port: 65536}, 0, 0, "is not a port number"},
		{"port below the range", Params{Host: "p", Port: -1}, 0, 0, "is not a port number"},
		{"empty host", Params{}, 0, 0, "Type the printer's address"},
		{"whitespace host", Params{Host: "   "}, 0, 0, "Type the printer's address"},
		{"timeout 0 defaults", Params{Host: "p", TimeoutMS: 0}, 9100, 5000, ""},
		{"timeout 499 rejected", Params{Host: "p", TimeoutMS: 499}, 0, 0, "between 0.5 and 30 seconds"},
		{"timeout 500 accepted", Params{Host: "p", TimeoutMS: 500}, 9100, 500, ""},
		{"timeout 30000 accepted", Params{Host: "p", TimeoutMS: 30000}, 9100, 30000, ""},
		{"timeout 30001 rejected", Params{Host: "p", TimeoutMS: 30001}, 0, 0, "between 0.5 and 30 seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.normalize()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize errored: %v", err)
			}
			if got.port != tt.wantPort {
				t.Errorf("port = %d, want %d", got.port, tt.wantPort)
			}
			if got.timeoutMS != tt.wantTimeout {
				t.Errorf("timeout = %d, want %d", got.timeoutMS, tt.wantTimeout)
			}
		})
	}
}

// TestQueryBytes pins the property the whole tool rests on: the safe button
// cannot make paper move.
func TestQueryBytes(t *testing.T) {
	got := string(QueryBytes())

	want := UEL + "@PJL INFO ID\r\n@PJL INFO STATUS\r\n" + UEL
	if got != want {
		t.Errorf("query bytes\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(got, "@PJL INFO ID") || !strings.Contains(got, "@PJL INFO STATUS") {
		t.Error("the enquiry does not ask what the printer is or how it is doing")
	}

	// These three absences are what make the enquiry read-only. Asserted
	// explicitly rather than implied by the equality above, so that a future
	// edit to the payload cannot quietly make the safe button print.
	if strings.Contains(got, "\x0c") {
		t.Error("the enquiry contains a form feed, which would eject a page")
	}
	if strings.Contains(got, "ENTER LANGUAGE") {
		t.Error("the enquiry contains ENTER LANGUAGE, which starts a print job")
	}
	if strings.Contains(got, "@PJL JOB") {
		t.Error("the enquiry contains a JOB command, which starts a print job")
	}
}

func TestTestPageBytes(t *testing.T) {
	when := time.Date(2026, 7, 28, 10, 30, 0, 0, time.UTC)
	got := string(TestPageBytes("192.168.1.50", 9100, when))

	if !strings.Contains(got, "192.168.1.50") {
		t.Error("the page does not name the printer")
	}
	if !strings.HasPrefix(got, UEL) {
		t.Error("the page does not start with the UEL")
	}
	if !strings.HasSuffix(got, UEL) {
		t.Error("the page does not end with the UEL")
	}
	if n := strings.Count(got, "\x0c"); n != 1 {
		t.Errorf("the page has %d form feeds, want exactly 1: none holds the sheet, two eject a blank one", n)
	}
	if !strings.Contains(got, "@PJL EOJ") {
		t.Error("the page does not end the job, so the printer may wait for a timeout")
	}
	if !strings.Contains(got, when.Format(time.RFC1123)) {
		t.Error("the page does not carry the time it was sent")
	}

	// Every line ends CRLF, which is what printers expect whatever OS CHIT runs
	// on. A bare \n would run the lines together on some models.
	for _, line := range strings.Split(got, "\n") {
		if line == "" || strings.HasSuffix(line, "\r") {
			continue
		}
		// The final segment after the last \n has no line ending at all.
		if strings.HasSuffix(got, line) {
			continue
		}
		t.Errorf("line %q does not end with CR LF", line)
	}

	// The byte count for a fixed host, port and time, produced by running the
	// function once. A change to the copy is meant to fail here so it gets read.
	// Was 431 until the product was renamed from CH-IT to CHIT on 2026-07-28,
	// which is two characters off two lines of the page. The page was printed and
	// read before this number was changed.
	if len(got) != 429 {
		t.Errorf("page is %d bytes, want 429", len(got))
	}
}

// TestTestPageNamesTheRealPort is the defect a live run found: the page said
// "on port 9100" whatever port it was actually sent to, so a sheet found on a
// printer later pointed at the wrong port.
func TestTestPageNamesTheRealPort(t *testing.T) {
	when := time.Date(2026, 7, 28, 10, 30, 0, 0, time.UTC)

	got := string(TestPageBytes("192.168.1.50", 9101, when))
	if !strings.Contains(got, "on port 9101") {
		t.Errorf("the page does not name port 9101:\n%s", got)
	}
	if strings.Contains(got, "on port 9100") {
		t.Error("the page names port 9100 when it was sent to 9101")
	}

	standard := string(TestPageBytes("192.168.1.50", 9100, when))
	if !strings.Contains(standard, "on port 9100") {
		t.Error("the page does not name port 9100 when that is the port used")
	}
}

func TestTestPageBytesUsesAHostileHostAsGiven(t *testing.T) {
	// There is no injection risk here beyond the printer printing odd text: the
	// payload is text sent to a printer, not a shell or a query. The length is
	// pinned so a change in handling is noticed rather than assumed.
	when := time.Date(2026, 7, 28, 10, 30, 0, 0, time.UTC)
	plain := TestPageBytes("printer", 9100, when)
	hostile := TestPageBytes("printer\r\n@PJL EOJ", 9100, when)
	if len(hostile) != len(plain)+len("\r\n@PJL EOJ") {
		t.Errorf("hostile host changed the page by %d bytes, want %d",
			len(hostile)-len(plain), len("\r\n@PJL EOJ"))
	}
}

func TestParsePJL(t *testing.T) {
	tests := []struct {
		name        string
		reply       string
		wantModel   string
		wantCode    string
		wantDisplay string
		wantOnline  string
	}{
		{
			name: "a full HP style reply",
			reply: UEL + "@PJL INFO ID\r\n\"HP LaserJet 400 M401dn\"\r\n\x0c" +
				"@PJL INFO STATUS\r\nCODE=10001\r\nDISPLAY=\"Ready\"\r\nONLINE=TRUE\r\n\x0c",
			wantModel: "HP LaserJet 400 M401dn", wantCode: "10001",
			wantDisplay: "Ready", wantOnline: "true",
		},
		{
			name:      "an unquoted model",
			reply:     "@PJL INFO ID\r\nBrother HL-L2350DW\r\n",
			wantModel: "Brother HL-L2350DW",
		},
		{
			name:        "status only",
			reply:       "@PJL INFO STATUS\r\nCODE=40021\r\nDISPLAY=\"Paper jam\"\r\nONLINE=FALSE\r\n",
			wantCode:    "40021",
			wantDisplay: "Paper jam", wantOnline: "false",
		},
		{
			name:      "LF line endings",
			reply:     "@PJL INFO ID\n\"Canon iR\"\n@PJL INFO STATUS\nCODE=10001\nONLINE=TRUE\n",
			wantModel: "Canon iR", wantCode: "10001", wantOnline: "true",
		},
		{
			name:       "lower case tags",
			reply:      "@PJL INFO STATUS\r\ncode=10001\r\nonline=true\r\n",
			wantCode:   "10001",
			wantOnline: "true",
		},
		{
			name:  "not PJL at all",
			reply: "<html><body>hello</body></html>",
		},
		{
			name:  "empty",
			reply: "",
		},
		{
			name:  "only the echoed commands",
			reply: UEL + "@PJL INFO ID\r\n@PJL INFO STATUS\r\n" + UEL,
		},
		{
			name:        "whitespace around the values",
			reply:       "@PJL INFO STATUS\r\n CODE = 10001 \r\n DISPLAY = \"Ready\" \r\n",
			wantCode:    "10001",
			wantDisplay: "Ready",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, code, display, online := ParsePJL(tt.reply)
			if model != tt.wantModel {
				t.Errorf("model = %q, want %q", model, tt.wantModel)
			}
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if display != tt.wantDisplay {
				t.Errorf("display = %q, want %q", display, tt.wantDisplay)
			}
			if online != tt.wantOnline {
				t.Errorf("online = %q, want %q", online, tt.wantOnline)
			}
		})
	}
}

// fakePrinter accepts one connection at a time, records everything it is sent,
// and optionally replies.
type fakePrinter struct {
	ln       net.Listener
	mu       sync.Mutex
	received []byte
	accepted int
	reply    string
	silent   bool
}

func startFakePrinter(t *testing.T, reply string, silent bool) *fakePrinter {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakePrinter{ln: ln, reply: reply, silent: silent}
	go f.serve()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakePrinter) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.accepted++
		silent, reply := f.silent, f.reply
		f.mu.Unlock()

		go func(c net.Conn) {
			defer c.Close()
			if !silent && reply != "" {
				_, _ = c.Write([]byte(reply))
			}
			// Read everything the client sends, then hang up so its read ends.
			_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
			data, _ := io.ReadAll(c)
			f.mu.Lock()
			f.received = append(f.received, data...)
			f.mu.Unlock()
		}(conn)
	}
}

func (f *fakePrinter) host() string {
	host, _, _ := net.SplitHostPort(f.ln.Addr().String())
	return host
}

func (f *fakePrinter) port() int {
	_, port, _ := net.SplitHostPort(f.ln.Addr().String())
	n, _ := net.LookupPort("tcp", port)
	return n
}

func (f *fakePrinter) params(timeoutMS int) Params {
	return Params{Host: f.host(), Port: f.port(), TimeoutMS: timeoutMS}
}

// waitForBytes gives the printer's goroutine time to record what it read. A
// transfer is recorded after the client has already finished, so reading the
// slice straight away is a race, not a pass.
func (f *fakePrinter) waitForBytes(t *testing.T) []byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := append([]byte(nil), f.received...)
		f.mu.Unlock()
		if len(got) > 0 {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the fake printer received nothing within 3 seconds")
	return nil
}

func (f *fakePrinter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.accepted
}

func TestQueryAgainstFakePrinter(t *testing.T) {
	reply := UEL + "@PJL INFO ID\r\n\"HP LaserJet 400 M401dn\"\r\n\x0c" +
		"@PJL INFO STATUS\r\nCODE=10001\r\nDISPLAY=\"Ready\"\r\nONLINE=TRUE\r\n\x0c"
	f := startFakePrinter(t, reply, false)

	got, err := Query(context.Background(), f.params(3000))
	if err != nil {
		t.Fatalf("Query errored: %v", err)
	}
	if !got.Connected {
		t.Fatalf("connected = false: %s", got.Headline)
	}
	if got.Printed {
		t.Error("printed = true for the read-only enquiry")
	}
	if got.Model != "HP LaserJet 400 M401dn" {
		t.Errorf("model = %q", got.Model)
	}
	if got.StatusCode != "10001" || got.Display != "Ready" || got.Online != "true" {
		t.Errorf("status = %q / %q / %q", got.StatusCode, got.Display, got.Online)
	}
	if got.Level != LevelOK {
		t.Errorf("level = %q, want %q", got.Level, LevelOK)
	}
	if !strings.Contains(got.Headline, "HP LaserJet 400 M401dn answered on") {
		t.Errorf("headline = %q", got.Headline)
	}
	if got.Address == "" {
		t.Error("address is blank")
	}

	// The property that matters: nothing that could eject a page was sent.
	sent := f.waitForBytes(t)
	if strings.Contains(string(sent), "\x0c") {
		t.Fatalf("the enquiry sent a form feed to the printer: %q", sent)
	}
	if strings.Contains(string(sent), "ENTER LANGUAGE") {
		t.Fatalf("the enquiry sent ENTER LANGUAGE: %q", sent)
	}
}

func TestQueryAgainstASilentPrinter(t *testing.T) {
	f := startFakePrinter(t, "", true)

	got, err := Query(context.Background(), f.params(600))
	if err != nil {
		t.Fatalf("Query errored: %v", err)
	}
	if !got.Connected {
		t.Fatalf("connected = false: %s", got.Headline)
	}
	if got.Level != LevelWarn {
		t.Errorf("level = %q, want %q", got.Level, LevelWarn)
	}
	if !strings.Contains(got.Headline, "sent nothing back") {
		t.Errorf("headline = %q", got.Headline)
	}
	if !strings.Contains(got.Advice, "many printers accept raw jobs and speak no PJL") {
		t.Errorf("advice = %q", got.Advice)
	}
}

func TestQueryReportsAnOfflinePrinter(t *testing.T) {
	f := startFakePrinter(t, "@PJL INFO STATUS\r\nCODE=40021\r\nDISPLAY=\"Paper jam\"\r\nONLINE=FALSE\r\n", false)

	got, err := Query(context.Background(), f.params(3000))
	if err != nil {
		t.Fatalf("Query errored: %v", err)
	}
	if got.Level != LevelWarn {
		t.Errorf("level = %q, want %q", got.Level, LevelWarn)
	}
	if !strings.Contains(got.Headline, "says it is not online") {
		t.Errorf("headline = %q", got.Headline)
	}
	// The code is reported verbatim and never translated: CHIT ships no
	// code-to-meaning table it cannot verify.
	if got.StatusCode != "40021" || got.Display != "Paper jam" {
		t.Errorf("status = %q / %q", got.StatusCode, got.Display)
	}
}

func TestQueryReportsAPrinterWithNoModel(t *testing.T) {
	f := startFakePrinter(t, "@PJL INFO STATUS\r\nCODE=10001\r\nONLINE=TRUE\r\n", false)

	got, err := Query(context.Background(), f.params(3000))
	if err != nil {
		t.Fatalf("Query errored: %v", err)
	}
	if got.Level != LevelOK {
		t.Errorf("level = %q, want %q", got.Level, LevelOK)
	}
	if !strings.Contains(got.Headline, "The printer at") {
		t.Errorf("headline = %q", got.Headline)
	}
}

func TestSendTestPageAgainstFakePrinter(t *testing.T) {
	f := startFakePrinter(t, "", true)

	got, err := SendTestPage(context.Background(), f.params(1500))
	if err != nil {
		t.Fatalf("SendTestPage errored: %v", err)
	}
	if !got.Printed {
		t.Fatalf("printed = false: %s", got.Headline)
	}
	if got.Level != LevelOK {
		t.Errorf("level = %q, want %q", got.Level, LevelOK)
	}
	if got.BytesSent != len(TestPageBytes(f.host(), f.port(), time.Now())) {
		t.Errorf("bytesSent = %d, want the whole page", got.BytesSent)
	}
	if !strings.Contains(got.Headline, "A page should come out of the printer") {
		t.Errorf("headline = %q", got.Headline)
	}

	sent := f.waitForBytes(t)
	if !strings.Contains(string(sent), "\x0c") {
		t.Fatal("the test page had no form feed, so the sheet would never be ejected")
	}
	if !strings.Contains(string(sent), "CHIT raw printer test") {
		t.Errorf("the page text did not arrive: %q", sent)
	}
}

func TestConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := net.LookupPort("tcp", portText)
	ln.Close()

	got, err := Query(context.Background(), Params{Host: host, Port: port, TimeoutMS: 1000})
	if err != nil {
		t.Fatalf("Query errored: %v", err)
	}
	if got.Connected {
		t.Fatal("connected = true on a closed port")
	}
	if got.Level != LevelError {
		t.Errorf("level = %q, want %q", got.Level, LevelError)
	}
	if !strings.Contains(got.Headline, "refused the connection on port") {
		t.Errorf("headline = %q", got.Headline)
	}
	if !strings.Contains(got.Advice, "Port 9100 printing") {
		t.Errorf("advice = %q", got.Advice)
	}
}

func TestNameThatDoesNotResolve(t *testing.T) {
	got, err := Query(context.Background(), Params{Host: "nosuchprinter.invalid", TimeoutMS: 3000})
	if err != nil {
		t.Fatalf("Query errored: %v", err)
	}
	if got.Connected {
		t.Fatal("connected = true for a name that does not resolve")
	}
	if !strings.Contains(got.Headline, "Could not find a printer called nosuchprinter.invalid") {
		t.Errorf("headline = %q", got.Headline)
	}
}

// TestNothingIsSentOnValidationFailure is the in-scope rule that nothing goes on
// the wire until the button is pressed with valid input.
func TestNothingIsSentOnValidationFailure(t *testing.T) {
	f := startFakePrinter(t, "", true)

	for _, p := range []Params{
		{Host: f.host(), Port: 70000},
		{Host: "", Port: f.port()},
		{Host: f.host(), Port: f.port(), TimeoutMS: 1},
	} {
		if _, err := Query(context.Background(), p); err == nil {
			t.Fatalf("%+v was accepted", p)
		}
		if _, err := SendTestPage(context.Background(), p); err == nil {
			t.Fatalf("%+v was accepted by SendTestPage", p)
		}
	}
	if f.count() != 0 {
		t.Errorf("the fake printer accepted %d connections: validation must happen before any of them", f.count())
	}
}

// TestSourceNeverShipsACodeTable guards the deliberate decision not to translate
// PJL status codes. Manufacturers define their own numbers, this machine has no
// vendor documentation to check any against, and a wrong meaning sends a tech to
// the wrong part of the printer.
func TestSourceNeverShipsACodeTable(t *testing.T) {
	for _, guess := range []string{"Paper jam", "Out of paper", "Toner low", "Cover open"} {
		if strings.Contains(sourceOf(t, "rawprint.go"), `"`+guess+`"`) {
			t.Errorf("the source contains a guessed meaning for a status code: %q", guess)
		}
	}
}
