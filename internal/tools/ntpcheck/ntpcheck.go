// Package ntpcheck asks a time server what time it is and reports how far this
// computer's clock is from that answer. It is the service layer the NTP Time
// Check page talks to.
package ntpcheck

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

// DefaultServer is what the page offers before the user types anything.
const DefaultServer = "pool.ntp.org"

// NTPPort is where a time server listens when the user did not say.
const NTPPort = "123"

// MaxServers keeps one run to a sensible number of questions.
const MaxServers = 4

// Server statuses.
const (
	StatusOK          = "ok"
	StatusWarn        = "warn"
	StatusError       = "error"
	StatusUnreachable = "unreachable"
)

// The three thresholds every verdict is built from, in milliseconds. Each one
// appears in a sentence the user reads, so the tests write the literals in and
// check these constants match them: the wording and the behaviour must not
// drift apart.
const (
	// FineOffsetMS is the gap below which nothing needs doing.
	FineOffsetMS = 1000
	// DriftOffsetMS is the gap below which a clock is drifting but harmless.
	DriftOffsetMS = 60000
	// KerberosLimitMS is the gap at which domain logins start failing, because
	// Kerberos allows five minutes of skew.
	KerberosLimitMS = 300000
)

const (
	defaultTimeoutMS = 3000
	minTimeoutMS     = 200
	maxTimeoutMS     = 15000
	maxWorkers       = 4
)

type Params struct {
	// Servers is host names or IP addresses, optionally with :port. Empty means
	// DefaultServer.
	Servers   []string `json:"servers"`
	TimeoutMS int      `json:"timeoutMs"`
}

// Server is one server's answer, or one server that did not answer.
type Server struct {
	// Server is the address as typed.
	Server string `json:"server"`
	// Address is the ip:port actually reached, "" when nothing was reached.
	Address string `json:"address"`
	// OffsetMS is positive when this computer is ahead of the server.
	OffsetMS float64 `json:"offsetMs"`
	// DelayMS is the round trip time of the exchange.
	DelayMS float64 `json:"delayMs"`
	Stratum int     `json:"stratum"`
	// ServerTime and LocalTime are RFC3339 with milliseconds, or "" when the
	// server did not answer.
	ServerTime string `json:"serverTime"`
	LocalTime  string `json:"localTime"`
	Status     string `json:"status"`
	// Message is one sentence, always set, including when Status is "ok".
	Message string `json:"message"`
}

type Report struct {
	Servers []Server `json:"servers"`
	// Level is "ok", "warn" or "error" and drives the colour of the headline.
	Level    string `json:"level"`
	Headline string `json:"headline"`
	// Advice is the fix, "" when Level is "ok".
	Advice    string `json:"advice"`
	CheckedAt string `json:"checkedAt"`
}

// settings is a validated Params, ready to run.
type settings struct {
	servers   []string
	timeout   time.Duration
	timeoutMS int
}

// normalize catches everything a user can get wrong, so a bad request fails
// before a single packet leaves the machine.
func (p Params) normalize() (settings, error) {
	var s settings

	s.servers = splitServers(p.Servers)
	if len(s.servers) == 0 {
		s.servers = []string{DefaultServer}
	}
	if len(s.servers) > MaxServers {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"Check at most %d time servers at once. You listed %d.", MaxServers, len(s.servers))
	}
	for _, server := range s.servers {
		if _, err := serverAddress(server); err != nil {
			return settings{}, err
		}
	}

	s.timeoutMS = p.TimeoutMS
	if s.timeoutMS == 0 {
		s.timeoutMS = defaultTimeoutMS
	}
	if s.timeoutMS < minTimeoutMS || s.timeoutMS > maxTimeoutMS {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"The wait for an answer must be between 0.2 and 15 seconds. %d ms is outside that.",
			s.timeoutMS)
	}
	s.timeout = time.Duration(s.timeoutMS) * time.Millisecond
	return s, nil
}

// splitServers accepts the whole field as typed: commas, semicolons, spaces and
// newlines all separate one server from the next.
func splitServers(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	return out
}

// serverAddress turns what the user typed into a dial target. A bare name or
// address gets the standard port; an explicit port is checked here rather than
// failing much later at dial time with nothing useful to say.
func serverAddress(server string) (string, error) {
	bad := core.Errorf(core.CodeInvalidInput,
		"%q is not a server name or an IP address. Try pool.ntp.org or 192.168.1.10.", server)
	if server == "" {
		return "", bad
	}
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		// No port, or an IPv6 literal without brackets. Either way the whole
		// string is the host.
		if strings.ContainsAny(server, " \t") {
			return "", bad
		}
		return net.JoinHostPort(server, NTPPort), nil
	}
	if host == "" {
		return "", bad
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", core.Errorf(core.CodeInvalidInput,
			"%q does not name a port between 1 and 65535.", server)
	}
	return net.JoinHostPort(host, port), nil
}

// Check asks every server in p and reports what each one said.
func Check(ctx context.Context, p Params) (Report, error) {
	st, err := p.normalize()
	if err != nil {
		return Report{}, err
	}

	results := make([]Server, len(st.servers))
	type indexed struct {
		i int
		s Server
	}
	for r := range core.Pool(ctx, indexRange(len(st.servers)), maxWorkers,
		func(c context.Context, i int) (indexed, bool) {
			return indexed{i: i, s: query(c, st.servers[i], st.timeout, st.timeoutMS)}, true
		}) {
		results[r.i] = r.s
	}
	// A cancelled run leaves holes in the slice, and a half-filled report is
	// worse than none: the caller is shutting down anyway.
	if ctx.Err() != nil {
		return Report{}, core.Errorf(core.CodeInternal,
			"CHIT was closing down, so the time check did not finish. Try it again.")
	}

	report := Report{Servers: results, CheckedAt: time.Now().Format(time.RFC3339)}
	report.Level, report.Headline, report.Advice = Classify(results)
	return report, nil
}

// indexRange exists because core.Pool works over a slice and the results have to
// come back in the order the user typed the servers, not the order they answered.
func indexRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// query is one whole SNTP exchange with one server.
func query(ctx context.Context, server string, timeout time.Duration, timeoutMS int) Server {
	out := Server{Server: server, Status: StatusUnreachable}

	addr, err := serverAddress(server)
	if err != nil {
		out.Message = core.MessageOf(err)
		return out
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		out.Message = dialMessage(server, err, timeoutMS)
		return out
	}
	defer conn.Close()
	out.Address = conn.RemoteAddr().String()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	sent := time.Now()
	if _, err := conn.Write(buildRequest(sent)); err != nil {
		out.Message = fmt.Sprintf(
			"%s did not answer within %d ms. UDP port 123 may be blocked between here and there.",
			server, timeoutMS)
		return out
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	received := time.Now()
	if err != nil {
		out.Message = fmt.Sprintf(
			"%s did not answer within %d ms. UDP port 123 may be blocked between here and there.",
			server, timeoutMS)
		return out
	}

	r, err := parseReply(buf[:n], sent)
	if err != nil {
		out.Stratum = r.stratum
		out.Message = replyMessage(server, r, err)
		return out
	}

	offset, delay := offsetAndDelay(sent, r, received)
	out.Stratum = r.stratum
	out.OffsetMS = milliseconds(offset)
	out.DelayMS = milliseconds(delay)
	out.ServerTime = r.transmit.Local().Format("2006-01-02T15:04:05.000Z07:00")
	out.LocalTime = received.Format("2006-01-02T15:04:05.000Z07:00")
	out.Status, out.Message = classifyOffset(server, offset)
	return out
}

func dialMessage(server string, err error, timeoutMS int) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Sprintf(
			"Could not find a server called %s. Check the spelling, or use its IP address.", server)
	}
	return fmt.Sprintf(
		"%s did not answer within %d ms. UDP port 123 may be blocked between here and there.",
		server, timeoutMS)
}

func replyMessage(server string, r reply, err error) string {
	switch err {
	case errKiss:
		code := r.kiss
		if code == "" {
			code = "an empty code"
		}
		return fmt.Sprintf(
			"%s refused the request and sent back the code %s. That usually means it is asking you to query it less often.",
			server, code)
	case errNoTime:
		return fmt.Sprintf(
			"%s answered without a time in it, so there is nothing to compare against.", server)
	default:
		return fmt.Sprintf(
			"%s answered, but not with a time. Check that it really is an NTP server.", server)
	}
}

// classifyOffset turns a measured gap into the row status and the sentence that
// explains it. The three thresholds are the constants at the top of this file,
// and the numbers in these sentences come from the same place.
func classifyOffset(server string, offset time.Duration) (string, string) {
	ms := math.Abs(float64(offset) / float64(time.Millisecond))
	direction := "ahead of"
	if offset < 0 {
		direction = "behind"
	}
	gap := describeGap(offset)

	switch {
	case ms < FineOffsetMS:
		return StatusOK, fmt.Sprintf("This computer is %s %s %s. That is fine.", gap, direction, server)
	case ms < DriftOffsetMS:
		return StatusWarn, fmt.Sprintf(
			"This computer is %s %s %s. Nothing will break yet, but the clock is drifting.",
			gap, direction, server)
	case ms < KerberosLimitMS:
		return StatusWarn, fmt.Sprintf(
			"This computer is %s %s %s. That is inside the 5 minute limit for domain logins, but only just.",
			gap, direction, server)
	default:
		return StatusError, fmt.Sprintf(
			"This computer is %s %s %s. Domain logins will fail until the clock is fixed: Kerberos allows 5 minutes.",
			gap, direction, server)
	}
}

// describeGap words a duration the way a tech would say it out loud.
func describeGap(offset time.Duration) string {
	d := offset
	if d < 0 {
		d = -d
	}
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	secs := int(d.Round(time.Second) / time.Second)
	if secs < 60 {
		return fmt.Sprintf("%d s", secs)
	}
	mins := secs / 60
	secs %= 60
	if mins < 60 {
		return fmt.Sprintf("%d m %d s", mins, secs)
	}
	hours := mins / 60
	mins %= 60
	return fmt.Sprintf("%d h %d m %d s", hours, mins, secs)
}

// Classify rolls the rows up into the one line at the top of the page. The
// worst row that answered wins; a server that could not be reached does not by
// itself condemn a good answer from another one.
func Classify(servers []Server) (level, headline, advice string) {
	answered, worst := 0, StatusOK
	unreachable := 0
	var best Server
	for _, s := range servers {
		switch s.Status {
		case StatusUnreachable:
			unreachable++
			continue
		case StatusError:
			worst = StatusError
		case StatusWarn:
			if worst == StatusOK {
				worst = StatusWarn
			}
		}
		if answered == 0 {
			best = s
		}
		answered++
	}

	if answered == 0 {
		return StatusError,
			"None of the time servers answered, so the clock could not be checked. That is not the same as the clock being right.",
			"UDP port 123 is often blocked on guest and hotel networks. Try again from a wired connection, or check your domain controller by name."
	}

	headline = best.Message
	switch worst {
	case StatusError:
		level = StatusError
		advice = "Fix the clock, then try the login again. On Windows run w32tm /resync as an administrator."
	case StatusWarn:
		level = StatusWarn
		advice = "Nothing is broken yet. If this machine drifts again, check that its time service is running and pointed at the right server."
	default:
		level = StatusOK
	}
	// A dead server alongside a good answer is worth a line, but it must not
	// turn a healthy clock into a red screen.
	if unreachable > 0 {
		note := fmt.Sprintf("%d of the %d servers could not be reached.", unreachable, len(servers))
		advice = strings.TrimSpace(note + " " + advice)
	}
	return level, headline, advice
}

func milliseconds(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}
