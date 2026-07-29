//go:build ignore

// Regenerates data/oui.txt.gz from the Wireshark "manuf" registry export.
//
//	export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH
//	go run internal/ouidb/gen.go                      # download a fresh copy
//	go run internal/ouidb/gen.go -src /path/to/manuf  # use a local copy
//
// Run it from the repository root. See internal/ouidb/SOURCE.md.
package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const sourceURL = "https://www.wireshark.org/download/automated/data/manuf"

// Long legal names ("... Technology Co., Ltd. Shenzhen Branch") add megabytes
// and tell a tech nothing the first 40 characters did not.
const maxVendor = 40

type record struct {
	prefix string // 6, 7 or 9 hex digits: 24, 28 or 36 bit allocation
	vendor string
}

func main() {
	src := flag.String("src", sourceURL, "manuf file path or URL")
	out := flag.String("out", "internal/ouidb/data/oui.txt.gz", "output file")
	flag.Parse()

	body, err := open(*src)
	if err != nil {
		fail(err)
	}
	defer body.Close()

	records, err := parse(body)
	if err != nil {
		fail(err)
	}
	if len(records) < 10000 {
		fail(fmt.Errorf("only %d records parsed, the source format probably changed", len(records)))
	}

	sort.Slice(records, func(i, j int) bool { return records[i].prefix < records[j].prefix })

	counts := map[int]int{}
	for _, r := range records {
		counts[len(r.prefix)]++
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "# CHIT OUI database. Regenerate with: go run internal/ouidb/gen.go\n")
	fmt.Fprintf(&buf, "# source: %s\n", sourceURL)
	fmt.Fprintf(&buf, "# fetched: %s\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&buf, "# records: %d (24-bit %d, 28-bit %d, 36-bit %d)\n",
		len(records), counts[6], counts[7], counts[9])
	fmt.Fprintf(&buf, "# format: <hex prefix>\\t<vendor>, prefix length gives the mask width\n")
	for _, r := range records {
		buf.WriteString(r.prefix)
		buf.WriteByte('\t')
		buf.WriteString(r.vendor)
		buf.WriteByte('\n')
	}

	if err := write(*out, buf.String()); err != nil {
		fail(err)
	}

	info, err := os.Stat(*out)
	if err != nil {
		fail(err)
	}
	fmt.Printf("%s: %d records (24-bit %d, 28-bit %d, 36-bit %d), %d bytes plain, %d bytes gzipped\n",
		*out, len(records), counts[6], counts[7], counts[9], buf.Len(), info.Size())
}

func open(src string) (io.ReadCloser, error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return os.Open(src)
	}
	resp, err := http.Get(src)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", src, resp.Status)
	}
	return resp.Body, nil
}

func parse(r io.Reader) ([]record, error) {
	var records []record
	seen := map[string]bool{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		prefix, ok := hexPrefix(strings.TrimSpace(fields[0]))
		if !ok || seen[prefix] {
			continue
		}
		vendor := ""
		if len(fields) > 2 {
			vendor = clean(fields[2])
		}
		if vendor == "" {
			vendor = clean(fields[1])
		}
		if vendor == "" {
			continue
		}
		seen[prefix] = true
		records = append(records, record{prefix: prefix, vendor: vendor})
	}
	return records, sc.Err()
}

// hexPrefix turns "00:1B:C5:00:00/36" into "001BC5000" (one hex digit per 4 bits).
func hexPrefix(field string) (string, bool) {
	addr, width := field, 0
	if slash := strings.IndexByte(field, '/'); slash >= 0 {
		addr = field[:slash]
		for _, c := range field[slash+1:] {
			if c < '0' || c > '9' {
				return "", false
			}
			width = width*10 + int(c-'0')
		}
	}
	digits := strings.ToUpper(strings.ReplaceAll(addr, ":", ""))
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		if !(c >= '0' && c <= '9') && !(c >= 'A' && c <= 'F') {
			return "", false
		}
	}
	if width == 0 {
		width = len(digits) * 4
	}
	if width != 24 && width != 28 && width != 36 {
		return "", false
	}
	nibbles := width / 4
	if len(digits) < nibbles {
		return "", false
	}
	return digits[:nibbles], true
}

func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= maxVendor {
		return s
	}
	// Cut back to a word boundary. A hard cut produced records like "Officially
	// Xerox, but 0:0:0:0:0:0 is mor", which reads as a corrupt database rather
	// than as a name that was too long to print.
	cut := string(runes[:maxVendor])
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.-")
}

func write(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(zw, content); err != nil {
		return err
	}
	return zw.Close()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}
