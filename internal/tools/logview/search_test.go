package logview

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
)

func runSearchRecording(ctx context.Context, p SearchParams) ([]Match, map[string]any, error) {
	var matches []Match
	summary, err := Search(ctx, p, Sink{
		Emit:     func(m Match) { matches = append(matches, m) },
		Progress: func(int, int, string) {},
	})
	return matches, summary, err
}

func TestSearchFindsEveryMatch(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 1000; i++ {
		if i == 3 || i == 500 || i == 999 {
			b.WriteString("line " + strconv.Itoa(i) + " has the WORD in it\n")
			continue
		}
		b.WriteString("line " + strconv.Itoa(i) + " is ordinary\n")
	}
	path := fixture(t, "a.log", b.String())

	matches, summary, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "word"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("found %d matches, want 3", len(matches))
	}
	for i, want := range []int{3, 500, 999} {
		if matches[i].Number != want {
			t.Errorf("match %d is on line %d, want %d", i, matches[i].Number, want)
		}
	}
	if summary["matches"].(int) != 3 {
		t.Errorf("summary matches = %v, want 3", summary["matches"])
	}
	if summary["lines"].(int) != 1000 {
		t.Errorf("summary lines = %v, want 1000", summary["lines"])
	}
}

func TestSearchMatchCase(t *testing.T) {
	path := fixture(t, "a.log", "the WORD here\nthe word here\n")

	insensitive, _, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "word"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(insensitive) != 2 {
		t.Errorf("case-insensitive found %d, want 2", len(insensitive))
	}

	sensitive, _, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "word", MatchCase: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(sensitive) != 1 {
		t.Fatalf("case-sensitive found %d, want 1", len(sensitive))
	}
	if sensitive[0].Number != 2 {
		t.Errorf("case-sensitive matched line %d, want 2", sensitive[0].Number)
	}
}

func TestSearchCol(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		query string
		want  int
	}{
		{"at the very start", "target here", "target", 0},
		{"in the middle", "abc target def", "target", 4},
		{"at the very end", "abc target", "target", 4},
		// Col is a byte index, so a multi-byte character before the hit must
		// shift it: "cafe" plus U+00E9 is 5 bytes, then a space.
		{"after a multi-byte character", "café target", "target", 6},
		{"after an emoji", "🔥 target", "target", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := fixture(t, "a.log", tt.line+"\n")
			matches, _, err := runSearchRecording(context.Background(),
				SearchParams{Path: path, Query: tt.query, MatchCase: true})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(matches) != 1 {
				t.Fatalf("found %d matches, want 1", len(matches))
			}
			if matches[0].Col != tt.want {
				t.Errorf("Col = %d, want %d", matches[0].Col, tt.want)
			}
			// The page slices Text at Col, so the two must agree.
			if got := matches[0].Text[matches[0].Col:]; !strings.HasPrefix(got, tt.query) {
				t.Errorf("Text[Col:] = %q, does not start with %q", got, tt.query)
			}
		})
	}
}

func TestSearchOffsetPointsAtTheLine(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	matches, _, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "line 4"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d matches, want 1", len(matches))
	}

	// Clicking a result reads at that offset, so the two must line up.
	chunk, err := Read(ReadParams{Path: path, Where: WhereAt, Offset: matches[0].Offset, Lines: 1})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if chunk.Lines[0].Text != "line 4" {
		t.Errorf("reading at the match offset gave %q, want \"line 4\"", chunk.Lines[0].Text)
	}
}

func TestSearchCapsAtMaxMatches(t *testing.T) {
	// 5000 is quoted to the user in the capped note, so it is written here as a
	// literal rather than read from the constant.
	const want = 5000
	if maxMatches != want {
		t.Fatalf("maxMatches = %d, want %d: the summary note quotes this number", maxMatches, want)
	}

	var b strings.Builder
	for i := 0; i < want+1; i++ {
		b.WriteString("hit\n")
	}
	path := fixture(t, "many.log", b.String())

	matches, summary, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "hit"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != want {
		t.Errorf("emitted %d matches, want exactly %d", len(matches), want)
	}
	if !summary["capped"].(bool) {
		t.Error("capped is false after hitting the limit")
	}
	if !strings.Contains(summary["note"].(string), "5000") {
		t.Errorf("the capped note does not quote the real limit: %q", summary["note"])
	}
}

func TestSearchNote(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		matches int
		capped  bool
		want    string
	}{
		{
			name: "capped", query: "e", matches: 5000, capped: true,
			want: "Stopped after the first 5000 matching lines. Search for something more specific to see the rest.",
		},
		{
			name: "nothing found", query: "zebra", matches: 0,
			want: "Nothing in that file contains \"zebra\". Check the spelling, and remember that this search is exact text, not a pattern. Turn Match case off if you are not sure of the capitals.",
		},
		{name: "a normal result says nothing", query: "error", matches: 6, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchNote(tt.query, tt.matches, tt.capped); got != tt.want {
				t.Errorf("searchNote =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestSearchCancelReturnsContextError(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString("some ordinary line of log text\n")
	}
	path := fixture(t, "a.log", b.String())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := runSearchRecording(ctx, SearchParams{Path: path, Query: "log"})
	if err == nil {
		t.Fatal("Search returned nil after a cancel, want the context error so job:done says cancelled")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want a context cancellation", err)
	}
}

func TestValidateSearchParams(t *testing.T) {
	tests := []struct {
		name    string
		in      SearchParams
		wantErr bool
		wantMsg string
	}{
		{name: "a normal query", in: SearchParams{Path: "x", Query: "error"}},
		{name: "one character", in: SearchParams{Path: "x", Query: "e"}},
		{name: "exactly the maximum", in: SearchParams{Path: "x", Query: strings.Repeat("a", 1000)}},
		{
			name: "one past the maximum", in: SearchParams{Path: "x", Query: strings.Repeat("a", 1001)},
			wantErr: true, wantMsg: "That search text is too long. Use a shorter piece of the line.",
		},
		{
			name: "an empty query", in: SearchParams{Path: "x"},
			wantErr: true, wantMsg: "Type something to look for.",
		},
		{
			name: "an empty path", in: SearchParams{Query: "e"},
			wantErr: true, wantMsg: "Choose a log file, or type the full path to it.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.in.validate()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validate accepted bad params")
			}
			if got := core.MessageOf(err); got != tt.wantMsg {
				t.Errorf("message = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestStartSearchRejectsBadInputBeforeJob(t *testing.T) {
	jobs := core.NewJobManager()

	id, err := New(jobs).StartSearch(SearchParams{Path: "x", Query: ""})
	if err == nil {
		t.Fatal("StartSearch accepted an empty query")
	}
	if id != "" {
		t.Errorf("job id = %q, want none: validation must happen before the job starts", id)
	}
	if running := jobs.Running(); running != 0 {
		t.Errorf("%d jobs are running, want none", running)
	}
}

func TestSearchSummaryKeys(t *testing.T) {
	got := searchSummary(SearchParams{Path: "/x", Query: "e"}, 0, 0, 0, false)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	want := []string{"bytes", "capped", "lines", "matches", "note", "path", "query"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("summary keys = %v, want %v", keys, want)
	}
}

// runSearch is the wiring between a JobContext and Search. Driving it through a
// real JobManager stops it sitting at 0% coverage.
func TestRunSearchCompletesInsideARealJob(t *testing.T) {
	path := fixture(t, "a.log", tenLines())

	jobs := core.NewJobManager()
	returned := make(chan error, 1)
	jobs.Start(JobKind, 0, func(jc *core.JobContext) error {
		err := runSearch(jc, SearchParams{Path: path, Query: "line"})
		returned <- err
		return err
	})

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("runSearch: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runSearch did not return within 10 seconds")
	}
}

func TestSearchTruncatesAnAbsurdLine(t *testing.T) {
	path := fixture(t, "a.log", strings.Repeat("a", 10000)+" needle\n")

	matches, _, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "a"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d matches, want 1", len(matches))
	}
	if len(matches[0].Text) != maxLineBytes {
		t.Errorf("match text is %d characters, want %d", len(matches[0].Text), maxLineBytes)
	}
	if !matches[0].Truncated {
		t.Error("Truncated is false for a cut line")
	}
}

func TestSearchCarriesTheLevel(t *testing.T) {
	path := fixture(t, "a.log", "ERROR the thing broke\nall fine here\n")

	matches, _, err := runSearchRecording(context.Background(),
		SearchParams{Path: path, Query: "the"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 || matches[0].Level != LevelError {
		t.Errorf("match level = %q, want %q", matches[0].Level, LevelError)
	}
}
