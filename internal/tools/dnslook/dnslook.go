// Package dnslook asks one or more DNS servers the same question and streams
// every answer back as a cancellable job. It is the service layer the DNS
// Lookup page talks to.
package dnslook

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"chit/internal/core"
	"chit/internal/netinfo"
)

// JobKind is the job manager kind for a lookup.
const JobKind = "dnslookup"

// KindRecord is the result kind every Record is emitted under. A job emits this
// kind and no other: useJob ignores the batch kind, so a second kind would be
// mixed into the same results array.
const KindRecord = "record"

// Record statuses.
const (
	StatusOK    = "ok"    // an answer, in Value
	StatusEmpty = "empty" // the server answered, there is no such record
	StatusError = "error" // the server could not be asked, or would not say
)

// SystemResolverLabel is what Record.Server carries for a query sent through
// whatever this computer is configured to use.
const SystemResolverLabel = "System resolver"

// SupportedTypes is the complete list. net.Resolver cannot ask for anything
// else, so this is not a starting point to extend.
var SupportedTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "PTR"}

// MaxServers keeps one run to a sensible number of questions.
const MaxServers = 6

const (
	defaultTimeoutMS = 3000
	minTimeoutMS     = 200
	maxTimeoutMS     = 15000
	maxNameLength    = 253
	maxLabelLength   = 63
	maxWorkers       = 8
)

type Params struct {
	Name string `json:"name"`
	// Types is a subset of SupportedTypes, upper case.
	Types []string `json:"types"`
	// Servers holds "" for the system resolver and an IP (optionally with a
	// port) for anything else.
	Servers   []string `json:"servers"`
	TimeoutMS int      `json:"timeoutMs"`
}

// Record is one answer, or one question that had no answer.
type Record struct {
	// Server is the display label: SystemResolverLabel or the server as typed.
	Server string `json:"server"`
	Type   string `json:"type"`
	// Name is the question asked, normalised: lower case, no trailing dot.
	Name  string `json:"name"`
	Value string `json:"value"`
	// Priority is the MX preference or the SRV priority, 0 for everything else.
	Priority int    `json:"priority"`
	Status   string `json:"status"`
	// Message explains an empty or failed query, "" when Status is "ok".
	Message string `json:"message"`
	// QueryMS is how long that server took to answer this question.
	QueryMS float64 `json:"queryMs"`
}

// ServerOption is one tick box in the UI.
type ServerOption struct {
	// ID is "" for the system resolver, otherwise the server address.
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// target is one validated server: the tick box id, the label a record carries,
// and the address to dial.
type target struct {
	id    string
	label string
	addr  string
}

// settings is a validated Params, ready to run.
type settings struct {
	name      string
	isIP      bool
	types     []string
	servers   []target
	timeout   time.Duration
	timeoutMS int
}

// normalize catches everything a user can get wrong, so a bad request rejects
// the StartLookup call instead of starting a job that immediately fails.
func (p Params) normalize() (settings, error) {
	var s settings

	if strings.TrimSpace(p.Name) == "" {
		return s, core.Errorf(core.CodeInvalidInput,
			"Enter a name or an IP address to look up, for example example.com or 192.168.1.1.")
	}
	name, isIP, err := normalizeName(p.Name)
	if err != nil {
		return s, err
	}
	s.name, s.isIP = name, isIP

	s.types, err = normalizeTypes(p.Types)
	if err != nil {
		return s, err
	}
	// An address has no A record to look up, and asking for one is never what a
	// tech means when they paste an IP into the box.
	if isIP {
		s.types = []string{"PTR"}
	}

	s.servers, err = normalizeServers(p.Servers)
	if err != nil {
		return s, err
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

func normalizeTypes(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		typ := strings.ToUpper(strings.TrimSpace(raw))
		if typ == "" {
			continue
		}
		if !supported(typ) {
			return nil, core.Errorf(core.CodeInvalidInput,
				"CHIT does not look up %s records. Choose from A, AAAA, CNAME, MX, TXT, NS, SRV and PTR.",
				typ)
		}
		if seen[typ] {
			continue
		}
		seen[typ] = true
		out = append(out, typ)
	}
	if len(out) == 0 {
		return nil, core.Errorf(core.CodeInvalidInput, "Pick at least one record type to look up.")
	}
	return out, nil
}

func supported(typ string) bool {
	for _, t := range SupportedTypes {
		if t == typ {
			return true
		}
	}
	return false
}

func normalizeServers(in []string) ([]target, error) {
	ids := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		id := strings.TrimSpace(raw)
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	// A caller that sends nothing gets the system resolver rather than an error.
	if len(ids) == 0 {
		ids = []string{""}
	}
	if len(ids) > MaxServers {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Ask at most %d servers at a time. You picked %d.", MaxServers, len(ids))
	}

	out := make([]target, 0, len(ids))
	for _, id := range ids {
		addr, err := serverAddress(id)
		if err != nil {
			return nil, err
		}
		label := id
		if id == "" {
			label = SystemResolverLabel
		}
		out = append(out, target{id: id, label: label, addr: addr})
	}
	return out, nil
}

// normalizeName turns what the user typed into the question to ask, and reports
// whether it is an address, which is what switches the run to a reverse lookup.
func normalizeName(raw string) (string, bool, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimSuffix(name, ".")

	if addr, err := netip.ParseAddr(name); err == nil {
		return addr.String(), true, nil
	}

	bad := core.Errorf(core.CodeInvalidInput,
		"%q is not a host name or an IP address. Try example.com or 192.168.1.1.", raw)
	if name == "" || len(name) > maxNameLength {
		return "", false, bad
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > maxLabelLength {
			return "", false, bad
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", false, bad
		}
		for i, c := range label {
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			// _sip._tcp.example.com is how SRV records are named, so a label may
			// begin with an underscore even though one inside a name cannot.
			case c == '_' && i == 0:
			default:
				return "", false, bad
			}
		}
	}
	return name, false, nil
}

// Service owns the lookup entry points. App forwards its bound methods here.
type Service struct {
	jobs *core.JobManager
}

func New(jobs *core.JobManager) *Service {
	return &Service{jobs: jobs}
}

// StartLookup begins a lookup and returns the job id at once. Answers arrive as
// "record" items on job:result, one per answer plus one for every question that
// had none.
func (s *Service) StartLookup(p Params) (string, error) {
	st, err := p.normalize()
	if err != nil {
		return "", err
	}
	return s.jobs.Start(JobKind, len(st.servers)*len(st.types), func(jc *core.JobContext) error {
		return lookup(jc, st)
	}), nil
}

// question is one server asked for one record type.
type question struct {
	server target
	typ    string
}

func lookup(jc *core.JobContext, st settings) error {
	ctx := jc.Ctx()

	questions := make([]question, 0, len(st.servers)*len(st.types))
	for _, server := range st.servers {
		for _, typ := range st.types {
			questions = append(questions, question{server: server, typ: typ})
		}
	}
	total := len(questions)
	message := fmt.Sprintf("Asking %d servers about %s", len(st.servers), st.name)
	jc.SetTotal(total)
	jc.Progress(0, total, message)

	var all []Record
	done, answers, failed := 0, 0, 0
	for records := range core.Pool(ctx, questions, maxWorkers, func(c context.Context, q question) ([]Record, bool) {
		return ask(c, q, st), true
	}) {
		done++
		allFailed := len(records) > 0
		for _, r := range records {
			if r.Status == StatusOK {
				answers++
			}
			if r.Status != StatusError {
				allFailed = false
			}
			all = append(all, r)
			jc.Emit(KindRecord, r)
		}
		if allFailed {
			failed++
		}
		jc.Progress(done, total, message)
	}
	// The pool closes its channel on cancellation too, so the loop ends either
	// way and the context is what tells the two apart.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	agree, note := summaryNote(st.name, st.isIP, failed == total, all)
	jc.SetSummary(map[string]any{
		"name":    st.name,
		"types":   len(st.types),
		"servers": len(st.servers),
		"answers": answers,
		"agree":   agree,
		"note":    note,
	})
	return nil
}

// summaryNote builds the line the page shows under the headline. It is the only
// place a tech is told that an IP switched the run to PTR, that two servers
// disagree, or that nothing answered at all.
func summaryNote(name string, isIP, allFailed bool, records []Record) (bool, string) {
	var parts []string
	if isIP {
		parts = append(parts, fmt.Sprintf(
			"%s is an IP address, so CHIT did a reverse (PTR) lookup and ignored the other record types.",
			name))
	}
	agree, disagreement := compareAnswers(name, records)
	if !agree {
		parts = append(parts, disagreement)
	}
	if allFailed {
		parts = append(parts,
			"None of the servers answered. Check that this computer can reach them on port 53.")
	}
	return agree, strings.Join(parts, " ")
}

// compareAnswers reports whether every server that answered gave the same set
// of addresses. Only A and AAAA are compared: a difference in MX or TXT between
// an internal and an external server is normal, a difference in the address is
// what breaks a connection.
func compareAnswers(name string, records []Record) (bool, string) {
	sets := make(map[string]map[string]bool)
	for _, r := range records {
		if r.Status != StatusOK || (r.Type != "A" && r.Type != "AAAA") {
			continue
		}
		if sets[r.Server] == nil {
			sets[r.Server] = make(map[string]bool)
		}
		sets[r.Server][r.Type+" "+r.Value] = true
	}
	if len(sets) < 2 {
		return true, ""
	}

	var first []string
	for _, set := range sets {
		flat := make([]string, 0, len(set))
		for key := range set {
			flat = append(flat, key)
		}
		sort.Strings(flat)
		if first == nil {
			first = flat
			continue
		}
		if strings.Join(first, "\n") != strings.Join(flat, "\n") {
			return false, fmt.Sprintf(
				"The servers do not agree on the address for %s. That usually means one of them is holding a stale cache, or the internal and external copies of the zone are different.",
				name)
		}
	}
	return true, ""
}

// Servers lists the tick boxes the UI offers: the system resolver, then the
// servers this machine is actually configured with, then three public ones.
func Servers() []ServerOption {
	out := []ServerOption{
		{ID: "", Label: SystemResolverLabel, Detail: "Whatever this computer is set to use"},
	}

	// A convenience list, not a requirement: a machine that will not report its
	// adapters still gets the system resolver and the public servers.
	if report, err := netinfo.List(); err == nil {
		for _, adapter := range report.Adapters {
			if !adapter.Primary {
				continue
			}
			for _, ip := range adapter.DNS {
				out = append(out, ServerOption{ID: ip, Label: ip, Detail: "This network (" + adapter.Name + ")"})
			}
		}
		for _, ip := range report.DNS {
			out = append(out, ServerOption{ID: ip, Label: ip, Detail: "This computer's settings"})
		}
	}

	out = append(out,
		ServerOption{ID: "8.8.8.8", Label: "8.8.8.8", Detail: "Google Public DNS"},
		ServerOption{ID: "1.1.1.1", Label: "1.1.1.1", Detail: "Cloudflare"},
		ServerOption{ID: "9.9.9.9", Label: "9.9.9.9", Detail: "Quad9"},
	)

	unique := make([]ServerOption, 0, len(out))
	seen := make(map[string]bool, len(out))
	for _, option := range out {
		if seen[option.ID] {
			continue
		}
		seen[option.ID] = true
		unique = append(unique, option)
	}
	return unique
}
