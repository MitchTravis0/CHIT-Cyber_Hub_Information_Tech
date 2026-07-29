// Package rawprint talks to a network printer's raw port (JetDirect, port
// 9100) with no driver and no print queue in the way. It is the service layer
// the Raw Printer Test page talks to.
package rawprint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
	"chit/internal/netscan"
)

const (
	// DefaultPort is the standard raw printing port. Some printers offer 9101
	// and 9102 as a second and third port.
	DefaultPort = 9100
	// DefaultTimeoutMS is the wait for a connection and for a PJL reply.
	DefaultTimeoutMS = 5000
	minTimeoutMS     = 500
	maxTimeoutMS     = 30000
	// MaxReplyBytes caps what is read back, so a printer that streams forever
	// cannot become the whole check.
	MaxReplyBytes = 8192
)

// Level values, shared with the other Phase 8 shape B tools.
const (
	LevelOK    = "ok"
	LevelWarn  = "warn"
	LevelError = "error"
)

type Params struct {
	Host string `json:"host"`
	// Port defaults to DefaultPort when 0.
	Port      int `json:"port"`
	TimeoutMS int `json:"timeoutMs"`
}

type Result struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// Address is the ip:port actually connected to, "" when nothing connected.
	Address   string  `json:"address"`
	Connected bool    `json:"connected"`
	ConnectMS float64 `json:"connectMs"`
	// Printed is true only when SendTestPage put a page on the wire.
	Printed   bool `json:"printed"`
	BytesSent int  `json:"bytesSent"`
	// Reply is the raw PJL text the printer sent back, "" when it sent nothing.
	Reply string `json:"reply"`
	// Model is the @PJL INFO ID value, "" when the printer did not give one.
	Model string `json:"model"`
	// StatusCode and Display are the CODE= and DISPLAY= values, verbatim. CHIT
	// ships no code-to-meaning table: manufacturers define their own numbers
	// beyond a small core, there is no vendor documentation to check one
	// against, and a wrong meaning sends a tech to the wrong part of the
	// printer. Only ONLINE=, which is unambiguous, drives a sentence.
	StatusCode string `json:"statusCode"`
	Display    string `json:"display"`
	// Online is "true", "false" or "" when the printer did not say.
	Online    string `json:"online"`
	Level     string `json:"level"`
	Headline  string `json:"headline"`
	Advice    string `json:"advice"`
	CheckedAt string `json:"checkedAt"`
}

type settings struct {
	host      string
	port      int
	timeout   time.Duration
	timeoutMS int
}

func (p Params) normalize() (settings, error) {
	var s settings

	s.host = strings.TrimSpace(p.Host)
	if s.host == "" {
		return s, core.Errorf(core.CodeInvalidInput,
			"Type the printer's address, for example 192.168.1.50.")
	}

	s.port = p.Port
	if s.port == 0 {
		s.port = DefaultPort
	}
	if s.port < 1 || s.port > 65535 {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"%d is not a port number. Ports run from 1 to 65535, and a raw printer port is usually 9100.",
			p.Port)
	}

	s.timeoutMS = p.TimeoutMS
	if s.timeoutMS == 0 {
		s.timeoutMS = DefaultTimeoutMS
	}
	if s.timeoutMS < minTimeoutMS || s.timeoutMS > maxTimeoutMS {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"The wait must be between 0.5 and 30 seconds. %d ms is outside that.", s.timeoutMS)
	}
	s.timeout = time.Duration(s.timeoutMS) * time.Millisecond
	return s, nil
}

// Query opens one connection and asks PJL what the printer is and whether it is
// online. It sends nothing that can cause a page to print.
func Query(ctx context.Context, p Params) (Result, error) {
	st, err := p.normalize()
	if err != nil {
		return Result{}, err
	}
	return exchange(ctx, st, QueryBytes(), false), nil
}

// SendTestPage sends a plain text page and a form feed, which makes the printer
// produce a sheet of paper.
func SendTestPage(ctx context.Context, p Params) (Result, error) {
	st, err := p.normalize()
	if err != nil {
		return Result{}, err
	}
	return exchange(ctx, st, TestPageBytes(st.host, st.port, time.Now()), true), nil
}

// exchange is one whole conversation: connect, send, read whatever comes back.
// A printer that will not answer is a successful check with a bad answer, so it
// comes back as a Result with Level "error" rather than as a Go error.
func exchange(ctx context.Context, st settings, payload []byte, printing bool) Result {
	out := Result{
		Host:      st.host,
		Port:      st.port,
		Level:     LevelError,
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	ctx, cancel := context.WithTimeout(ctx, st.timeout)
	defer cancel()

	address := net.JoinHostPort(st.host, strconv.Itoa(st.port))
	started := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		out.Headline, out.Advice = connectFailure(st, err)
		return out
	}
	defer conn.Close()

	out.Connected = true
	out.ConnectMS = milliseconds(time.Since(started))
	out.Address = conn.RemoteAddr().String()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	n, err := conn.Write(payload)
	out.BytesSent = n
	if err != nil || n < len(payload) {
		out.Headline = fmt.Sprintf(
			"The connection to %s closed before the whole page was sent.", st.host)
		out.Advice = "Try again. If it keeps happening, the printer may be busy with another job."
		return out
	}

	// A printer with nothing to say simply never writes, so the read ends at the
	// deadline. That is a normal outcome, not a failure.
	reply, _ := io.ReadAll(io.LimitReader(conn, MaxReplyBytes))
	out.Reply = string(reply)
	out.Model, out.StatusCode, out.Display, out.Online = ParsePJL(out.Reply)

	if printing {
		out.Printed = true
		out.Level = LevelOK
		out.Headline = fmt.Sprintf(
			"Sent %d bytes to %s port %d. A page should come out of the printer within a few seconds.",
			out.BytesSent, st.host, st.port)
		out.Advice = "If nothing comes out, the printer is either not really at this address or it does not accept plain text on this port."
		return out
	}

	out.Level, out.Headline, out.Advice = classifyQuery(st, out)
	return out
}

func connectFailure(st settings, err error) (headline, advice string) {
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return fmt.Sprintf(
			"Could not find a printer called %s. Check the spelling, or use its IP address.", st.host), ""
	case netscan.IsRefused(err):
		return fmt.Sprintf("%s refused the connection on port %d.", st.host, st.port),
			"The printer is switched on and reachable, but raw printing is turned off or on another port. Look for \"Port 9100 printing\" or \"Raw print\" in the printer's web page."
	default:
		return fmt.Sprintf("%s did not answer on port %d within %d ms.", st.host, st.port, st.timeoutMS),
			"The printer may be switched off, on a different address, or a firewall may be dropping the connection."
	}
}

// classifyQuery turns what came back from the read-only enquiry into the line
// at the top of the page.
func classifyQuery(st settings, r Result) (level, headline, advice string) {
	switch {
	case r.Model == "" && r.StatusCode == "" && r.Online == "":
		return LevelWarn,
			fmt.Sprintf("%s accepted the connection on port %d but sent nothing back.", st.host, st.port),
			"That is normal: many printers accept raw jobs and speak no PJL. The test page will probably still print."
	case r.Online == "false":
		return LevelWarn,
			"The printer answered but says it is not online.",
			"Check the printer's own display: it is usually out of paper, jammed, or paused."
	case r.Model != "":
		return LevelOK,
			fmt.Sprintf("%s answered on %s port %d.", r.Model, st.host, st.port), ""
	default:
		return LevelOK,
			fmt.Sprintf("The printer at %s answered on port %d.", st.host, st.port), ""
	}
}

// ParsePJL pulls the model, the status code, the display text and the online
// flag out of a PJL reply. A reply that is not PJL at all yields four empty
// strings and no error: plenty of printers speak none of it.
func ParsePJL(reply string) (model, code, display, online string) {
	inID := false
	for _, raw := range strings.Split(reply, "\n") {
		// A printer sends the UEL glued to the command that follows it, and ends
		// each block with a form feed on the same line as the next one. Neither
		// is a separate line, so both have to come out before anything can be
		// recognised.
		line := strings.ReplaceAll(raw, UEL, "")
		line = strings.ReplaceAll(line, formFeed, "")
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))

		if strings.HasPrefix(trimmed, "@PJL") {
			// The value of INFO ID is on the line after the echoed command,
			// which is how every printer answers it.
			inID = strings.Contains(trimmed, "INFO ID")
			continue
		}
		if trimmed == "" {
			continue
		}
		if name, value, ok := strings.Cut(trimmed, "="); ok {
			switch strings.ToUpper(strings.TrimSpace(name)) {
			case "CODE":
				code = unquote(value)
			case "DISPLAY":
				display = unquote(value)
			case "ONLINE":
				online = strings.ToLower(unquote(value))
			}
			inID = false
			continue
		}
		if inID && model == "" {
			model = unquote(trimmed)
			inID = false
		}
	}
	return model, code, display, online
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func milliseconds(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
