package tracert

import (
	"reflect"
	"testing"
)

// Every line below is real traceroute output, pasted as written rather than
// assembled, so the tests break if the parser stops handling what a machine in
// the field actually prints.
func TestParseUnixHop(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		number   int
		ip       string
		hostname string
		times    []float64
		lost     int
		alsoSeen []string
		note     string
	}{
		{
			name:     "gateway with a name",
			line:     " 1  _gateway (192.168.1.1)  1.234 ms  1.111 ms  1.050 ms",
			number:   1,
			ip:       "192.168.1.1",
			hostname: "_gateway",
			times:    []float64{1.234, 1.111, 1.05},
			alsoSeen: []string{},
		},
		{
			name:     "no reply at all",
			line:     " 2  * * *",
			number:   2,
			times:    []float64{},
			lost:     3,
			alsoSeen: []string{},
		},
		{
			name:     "address repeated in the name column",
			line:     " 3  10.0.0.1 (10.0.0.1)  9.876 ms  9.700 ms  10.200 ms",
			number:   3,
			ip:       "10.0.0.1",
			times:    []float64{9.876, 9.7, 10.2},
			alsoSeen: []string{},
		},
		{
			name:     "named router",
			line:     " 4  ae-1.border.example.net (203.0.113.5)  12.0 ms  11.5 ms  12.3 ms",
			number:   4,
			ip:       "203.0.113.5",
			hostname: "ae-1.border.example.net",
			times:    []float64{12, 11.5, 12.3},
			alsoSeen: []string{},
		},
		{
			name:     "load balanced hop answering from two routers",
			line:     " 5  a.example.net (198.51.100.1)  10.0 ms  b.example.net (198.51.100.2)  11.0 ms  10.5 ms",
			number:   5,
			ip:       "198.51.100.1",
			hostname: "a.example.net",
			times:    []float64{10, 11, 10.5},
			alsoSeen: []string{"198.51.100.2"},
		},
		{
			name:     "host unreachable annotation",
			line:     " 6  192.0.2.1 (192.0.2.1)  20.0 ms !H",
			number:   6,
			ip:       "192.0.2.1",
			times:    []float64{20},
			alsoSeen: []string{},
			note:     "the router said it cannot reach that host",
		},
		{
			name:     "one probe answered of three",
			line:     " 7  * 203.0.113.9 (203.0.113.9)  30.0 ms *",
			number:   7,
			ip:       "203.0.113.9",
			times:    []float64{30},
			lost:     2,
			alsoSeen: []string{},
		},
		{
			name:     "numeric form from -n",
			line:     " 8  192.168.1.1  1.234 ms  1.111 ms  1.050 ms",
			number:   8,
			ip:       "192.168.1.1",
			times:    []float64{1.234, 1.111, 1.05},
			alsoSeen: []string{},
		},
		{
			name:     "single probe",
			line:     " 9  198.51.100.7 (198.51.100.7)  7.5 ms",
			number:   9,
			ip:       "198.51.100.7",
			times:    []float64{7.5},
			alsoSeen: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hop, ok := parseUnixHop(c.line)
			if !ok {
				t.Fatalf("parseUnixHop(%q) rejected a hop line", c.line)
			}
			if hop.Number != c.number {
				t.Errorf("number = %d, want %d", hop.Number, c.number)
			}
			if hop.IP != c.ip {
				t.Errorf("ip = %q, want %q", hop.IP, c.ip)
			}
			if hop.Hostname != c.hostname {
				t.Errorf("hostname = %q, want %q", hop.Hostname, c.hostname)
			}
			if !reflect.DeepEqual(hop.TimesMS, c.times) {
				t.Errorf("timesMs = %v, want %v", hop.TimesMS, c.times)
			}
			if hop.Lost != c.lost {
				t.Errorf("lost = %d, want %d", hop.Lost, c.lost)
			}
			if !reflect.DeepEqual(hop.AlsoSeen, c.alsoSeen) {
				t.Errorf("alsoSeen = %v, want %v", hop.AlsoSeen, c.alsoSeen)
			}
			if hop.Note != c.note {
				t.Errorf("note = %q, want %q", hop.Note, c.note)
			}
		})
	}
}

func TestParseUnixHopRejects(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"header", "traceroute to google.com (142.250.72.206), 30 hops max, 60 byte packets"},
		{"empty", ""},
		{"blank", "   "},
		{"trailer", "Trace complete."},
		{"starts with a word", "sendto: Network is unreachable"},
		{"leading zero is not a hop number", " 01  192.168.1.1  1.0 ms"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if hop, ok := parseUnixHop(c.line); ok {
				t.Errorf("parseUnixHop(%q) returned %+v, want rejected", c.line, hop)
			}
		})
	}
}

func TestParseTracertHop(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		number   int
		ip       string
		hostname string
		times    []float64
		lost     int
	}{
		{
			name:   "three answers",
			line:   "  1     1 ms     1 ms     1 ms  192.168.1.1",
			number: 1,
			ip:     "192.168.1.1",
			times:  []float64{1, 1, 1},
		},
		{
			name:   "under a millisecond",
			line:   "  2    <1 ms    <1 ms    <1 ms  10.0.0.1",
			number: 2,
			ip:     "10.0.0.1",
			times:  []float64{1, 1, 1},
		},
		{
			name:   "timed out, message in any language",
			line:   "  3     *        *        *     Request timed out.",
			number: 3,
			times:  []float64{},
			lost:   3,
		},
		{
			name:     "name and address",
			line:     "  4    12 ms    11 ms    12 ms  ae-1.border.example.net [203.0.113.5]",
			number:   4,
			ip:       "203.0.113.5",
			hostname: "ae-1.border.example.net",
			times:    []float64{12, 11, 12},
		},
		{
			name:   "one probe lost",
			line:   "  5     9 ms     *       10 ms  203.0.113.9",
			number: 5,
			ip:     "203.0.113.9",
			times:  []float64{9, 10},
			lost:   1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hop, ok := parseTracertHop(c.line)
			if !ok {
				t.Fatalf("parseTracertHop(%q) rejected a hop line", c.line)
			}
			if hop.Number != c.number {
				t.Errorf("number = %d, want %d", hop.Number, c.number)
			}
			if hop.IP != c.ip {
				t.Errorf("ip = %q, want %q", hop.IP, c.ip)
			}
			if hop.Hostname != c.hostname {
				t.Errorf("hostname = %q, want %q", hop.Hostname, c.hostname)
			}
			if !reflect.DeepEqual(hop.TimesMS, c.times) {
				t.Errorf("timesMs = %v, want %v", hop.TimesMS, c.times)
			}
			if hop.Lost != c.lost {
				t.Errorf("lost = %d, want %d", hop.Lost, c.lost)
			}
			if len(hop.AlsoSeen) != 0 {
				t.Errorf("alsoSeen = %v, want empty", hop.AlsoSeen)
			}
		})
	}
}

func TestParseTracertHopRejects(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"header", "Tracing route to google.com [142.250.72.206]"},
		{"second header line", "over a maximum of 30 hops:"},
		{"empty", ""},
		{"trailer", "Trace complete."},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if hop, ok := parseTracertHop(c.line); ok {
				t.Errorf("parseTracertHop(%q) returned %+v, want rejected", c.line, hop)
			}
		})
	}
}

func TestHopStats(t *testing.T) {
	cases := []struct {
		name             string
		times            []float64
		best, avg, worst float64
	}{
		{"three probes", []float64{10, 20, 30}, 10, 20, 30},
		{"one probe", []float64{12.5}, 12.5, 12.5, 12.5},
		{"nothing answered", []float64{}, 0, 0, 0},
		{"rounded to two decimals", []float64{1, 2, 2}, 1, 1.67, 2},
		{"out of order", []float64{30.006, 1.114, 9.9}, 1.11, 13.67, 30.01},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			best, avg, worst := hopStats(c.times)
			if best != c.best || avg != c.avg || worst != c.worst {
				t.Errorf("hopStats(%v) = %v, %v, %v, want %v, %v, %v",
					c.times, best, avg, worst, c.best, c.avg, c.worst)
			}
		})
	}
}

func TestAnnotationNote(t *testing.T) {
	cases := []struct {
		tag  string
		want string
	}{
		{"!H", "the router said it cannot reach that host"},
		{"!N", "the router said it cannot reach that network"},
		{"!P", "the router said it cannot reach that protocol"},
		{"!X", "the router refused to pass the traffic, which usually means a firewall rule"},
		{"!A", "the router refused to pass the traffic, which usually means a firewall rule"},
		{"!S", "the source route failed"},
		{"!F", "the packet was too big for the link and needs fragmenting"},
		{"!Z", "the router reported a problem (!Z)"},
	}

	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			if got := annotationNote(c.tag); got != c.want {
				t.Errorf("annotationNote(%q) = %q, want %q", c.tag, got, c.want)
			}
		})
	}
}
