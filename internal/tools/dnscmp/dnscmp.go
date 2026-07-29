// Package dnscmp asks several DNS resolvers the same question at the same
// moment and reports which of them disagree. It is the service layer the DNS
// Resolver Comparer page talks to.
//
// It is deliberately narrower than internal/tools/dnslook: one name and one
// record type, laid out one row per resolver with a majority answer and a
// speed ranking. That comparison is the whole reason this tool exists next to
// the shipped DNS Lookup.
package dnscmp

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"chit/internal/core"
	"chit/internal/dnsx"
	"chit/internal/netinfo"
)

// SystemResolverLabel is what an answer carries when it came through whatever
// this computer is configured to use.
const SystemResolverLabel = "System resolver"

// SupportedTypes is the complete list, one at a time. PTR and SRV are in the
// shipped DNS Lookup tool: a reverse lookup is not a propagation question.
var SupportedTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "NS"}

// MaxServers keeps one comparison readable and one run bounded.
const MaxServers = 8

// Answer statuses.
const (
	StatusOK    = "ok"    // values present
	StatusEmpty = "empty" // the resolver answered, there is no such record
	StatusError = "error" // the resolver could not be asked
)

// Level values, shared with the other Phase 8 shape B tools.
const (
	LevelOK    = "ok"
	LevelWarn  = "warn"
	LevelError = "error"
)

// headlineValues is how many answers a headline names before it says
// "and N more".
const headlineValues = 3

const (
	defaultTimeoutMS = 3000
	minTimeoutMS     = 200
	maxTimeoutMS     = 15000
	maxNameLength    = 253
	maxLabelLength   = 63
)

type Params struct {
	Name string `json:"name"`
	// Type is one of SupportedTypes, upper case.
	Type string `json:"type"`
	// Servers holds "" for the system resolver and an IP (optionally with a
	// port) for anything else.
	Servers   []string `json:"servers"`
	TimeoutMS int      `json:"timeoutMs"`
}

// ServerOption is one tick box in the UI.
type ServerOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// Answer is what one resolver said.
type Answer struct {
	Server string `json:"server"`
	Label  string `json:"label"`
	// Values is the normalised answer set: lower case, no trailing dot, sorted.
	Values []string `json:"values"`
	Status string   `json:"status"`
	// Message is one sentence for empty and error, "" for ok.
	Message string  `json:"message"`
	QueryMS float64 `json:"queryMs"`
	// InStep is false when this resolver's answer differs from the majority. It
	// is always true for an error, because a resolver that could not be asked
	// has not disagreed about anything.
	InStep bool `json:"inStep"`
}

type Comparison struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Answers is in the order the servers were requested.
	Answers []Answer `json:"answers"`
	// Majority is the answer set most resolvers gave, [] when nothing answered.
	Majority []string `json:"majority"`
	// MajorityCount and Answered count resolvers, not values.
	MajorityCount int  `json:"majorityCount"`
	Answered      int  `json:"answered"`
	Agree         bool `json:"agree"`
	// FastestLabel and FastestMS describe the quickest resolver that answered.
	FastestLabel string  `json:"fastestLabel"`
	FastestMS    float64 `json:"fastestMs"`
	Level        string  `json:"level"`
	Headline     string  `json:"headline"`
	Advice       string  `json:"advice"`
	CheckedAt    string  `json:"checkedAt"`
}

// target is one validated resolver: the tick box id, the label an answer
// carries, and the address to dial.
type target struct {
	id    string
	label string
	addr  string
}

// settings is a validated Params, ready to run.
type settings struct {
	name      string
	typ       string
	servers   []target
	timeout   time.Duration
	timeoutMS int
}

// normalize catches everything a user can get wrong, so a bad request rejects
// the call instead of firing off queries that immediately fail.
func (p Params) normalize() (settings, error) {
	var s settings

	name, err := normalizeName(p.Name)
	if err != nil {
		return s, err
	}
	s.name = name

	s.typ = strings.ToUpper(strings.TrimSpace(p.Type))
	if s.typ == "" {
		s.typ = "A"
	}
	if !supported(s.typ) {
		return settings{}, core.Errorf(core.CodeInvalidInput,
			"CHIT cannot compare %s records. Choose from A, AAAA, CNAME, MX, TXT and NS.", s.typ)
	}

	s.servers, err = normalizeServers(p.Servers)
	if err != nil {
		return settings{}, err
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

func supported(typ string) bool {
	for _, t := range SupportedTypes {
		if t == typ {
			return true
		}
	}
	return false
}

// normalizeName rejects an address as well as a malformed name. Unlike the
// shipped DNS Lookup, an IP here is not a reverse lookup: it is a question this
// tool cannot answer.
func normalizeName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimSuffix(name, ".")

	if name == "" {
		return "", core.Errorf(core.CodeInvalidInput,
			"Type a name to look up, for example example.com.")
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return "", core.Errorf(core.CodeInvalidInput,
			"%s is an address, not a name. This tool compares what different DNS servers say about a name. Use DNS Lookup for a reverse lookup.",
			name)
	}

	bad := core.Errorf(core.CodeInvalidInput,
		"%q is not a host name. Try example.com.", raw)
	if len(name) > maxNameLength {
		return "", bad
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > maxLabelLength {
			return "", bad
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", bad
		}
		for i, c := range label {
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			// _ldap._tcp.example.com is how service records are named, so a
			// label may begin with an underscore even though one inside a name
			// cannot.
			case c == '_' && i == 0:
			default:
				return "", bad
			}
		}
	}
	return name, nil
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
	// A caller that ticks nothing gets the system resolver rather than an error.
	if len(ids) == 0 {
		ids = []string{""}
	}
	if len(ids) > MaxServers {
		return nil, core.Errorf(core.CodeInvalidInput,
			"Compare at most %d resolvers at a time. You picked %d.", MaxServers, len(ids))
	}

	out := make([]target, 0, len(ids))
	for _, id := range ids {
		addr, err := dnsx.ServerAddress(id)
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

// Compare asks every resolver the same question and reports who disagrees.
func Compare(ctx context.Context, p Params) (Comparison, error) {
	st, err := p.normalize()
	if err != nil {
		return Comparison{}, err
	}

	answers := make([]Answer, len(st.servers))
	type indexed struct {
		i int
		a Answer
	}
	for r := range core.Pool(ctx, indexRange(len(st.servers)), MaxServers,
		func(c context.Context, i int) (indexed, bool) {
			return indexed{i: i, a: ask(c, st.servers[i], st)}, true
		}) {
		answers[r.i] = r.a
	}
	if ctx.Err() != nil {
		return Comparison{}, core.Errorf(core.CodeInternal,
			"CHIT was closing down, so the comparison did not finish. Try it again.")
	}

	out := Comparison{
		Name:      st.name,
		Type:      st.typ,
		Answers:   answers,
		CheckedAt: time.Now().Format(time.RFC3339),
	}
	Summarize(&out)
	return out, nil
}

func indexRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// Summarize fills in the majority, the in-step flags, the fastest resolver and
// the headline. It takes no network, so every case is testable.
func Summarize(c *Comparison) {
	if c.Answers == nil {
		c.Answers = []Answer{}
	}
	// A nil slice marshals to JSON null and the table renders nothing at all, so
	// every slice that crosses the boundary is made empty here rather than
	// trusting whoever built the Answers.
	for i := range c.Answers {
		if c.Answers[i].Values == nil {
			c.Answers[i].Values = []string{}
		}
	}

	majority, count, answered := majorityOf(c.Answers)
	c.Majority = majority
	c.MajorityCount = count
	c.Answered = answered

	key := strings.Join(majority, "\n")
	c.Agree = true
	for i := range c.Answers {
		if c.Answers[i].Status == StatusError {
			// A resolver that could not be asked has not disagreed about
			// anything, so it is never flagged out of step.
			c.Answers[i].InStep = true
			continue
		}
		c.Answers[i].InStep = strings.Join(c.Answers[i].Values, "\n") == key
		if !c.Answers[i].InStep {
			c.Agree = false
		}
	}

	c.FastestLabel, c.FastestMS = fastest(c.Answers)
	c.Level, c.Headline, c.Advice = classify(c)
}

// majorityOf finds the answer set the most resolvers gave. A tie is broken by
// the earlier position in the request order, so the result does not depend on
// which resolver happened to answer first.
func majorityOf(answers []Answer) ([]string, int, int) {
	counts := make(map[string]int)
	firstAt := make(map[string]int)
	values := make(map[string][]string)
	answered := 0

	for i, a := range answers {
		if a.Status == StatusError {
			continue
		}
		answered++
		key := strings.Join(a.Values, "\n")
		counts[key]++
		if _, seen := firstAt[key]; !seen {
			firstAt[key] = i
			values[key] = a.Values
		}
	}
	if answered == 0 {
		return []string{}, 0, 0
	}

	bestKey, bestCount, bestAt := "", -1, 0
	for key, count := range counts {
		if count > bestCount || (count == bestCount && firstAt[key] < bestAt) {
			bestKey, bestCount, bestAt = key, count, firstAt[key]
		}
	}
	best := values[bestKey]
	if best == nil {
		best = []string{}
	}
	return best, bestCount, answered
}

// fastest names the quickest resolver that answered, ignoring the system
// resolver: that one may be served from a local cache or a stub, so its time is
// not comparable with a real network query.
func fastest(answers []Answer) (string, float64) {
	label, best := "", 0.0
	for _, a := range answers {
		if a.Status == StatusError || a.Server == "" {
			continue
		}
		if label == "" || a.QueryMS < best {
			label, best = a.Label, a.QueryMS
		}
	}
	return label, best
}

func classify(c *Comparison) (level, headline, advice string) {
	switch {
	case c.Answered == 0:
		return LevelError,
			"None of the resolvers answered, so nothing could be compared.",
			"Check that this computer can reach port 53 on those servers."
	case len(c.Answers) == 1:
		return LevelOK,
			"Only one resolver was asked, so there is nothing to compare. Tick a second one to see whether they agree.",
			""
	case c.Agree && len(c.Majority) == 0:
		return LevelOK,
			fmt.Sprintf("All %d resolvers agree: there is no %s record for %s.",
				c.Answered, c.Type, c.Name),
			""
	case c.Agree && isAddressType(c.Type):
		return LevelOK,
			fmt.Sprintf("All %d resolvers agree: %s is %s.",
				c.Answered, c.Name, listValues(c.Majority)),
			""
	case c.Agree:
		// "example.com is ns1.example.com" is nonsense. Only an address answers
		// the question "what is this name"; every other type has to be named.
		return LevelOK,
			fmt.Sprintf("All %d resolvers agree on the %s records for %s: %s.",
				c.Answered, c.Type, c.Name, listValues(c.Majority)),
			""
	default:
		said := "say something else"
		return LevelWarn,
			fmt.Sprintf("%d of %d resolvers say %s, the other %d %s.",
				c.MajorityCount, c.Answered, listValues(c.Majority), c.Answered-c.MajorityCount, said),
			"That usually means a change has not reached every server yet, or one of them is holding a stale cache. The out of step rows below show what each returned."
	}
}

// isAddressType reports whether "<name> is <value>" reads correctly for this
// record type. Only A and AAAA answer "what is this name".
func isAddressType(typ string) bool { return typ == "A" || typ == "AAAA" }

// listValues names the first few answers and counts the rest, so a domain with
// twenty addresses does not produce a headline nobody can read.
func listValues(values []string) string {
	if len(values) == 0 {
		return "nothing"
	}
	if len(values) <= headlineValues {
		return strings.Join(values, ", ")
	}
	return fmt.Sprintf("%s and %d more",
		strings.Join(values[:headlineValues], ", "), len(values)-headlineValues)
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

// normalizeValues makes two resolvers' answers comparable: lower case, no
// trailing dot, no duplicates, sorted. Without the sort, two servers returning
// the same addresses in a different order would read as a disagreement, which
// is exactly what round-robin DNS does on every query.
func normalizeValues(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
