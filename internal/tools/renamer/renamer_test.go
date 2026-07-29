package renamer

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"chit/internal/core"
)

// rename runs the whole pure pipeline the way preview does: validate the rules
// once, compile the pattern once, then work out one name.
func rename(t *testing.T, name string, p Params, index int) string {
	t.Helper()
	checked, re, err := p.rules()
	if err != nil {
		t.Fatalf("rules() rejected a good set of rules: %v", err)
	}
	return applyRules(name, checked, index, re)
}

func TestSplitName(t *testing.T) {
	tests := []struct {
		name string
		base string
		ext  string
	}{
		{name: "report.txt", base: "report", ext: ".txt"},
		{name: "archive.tar.gz", base: "archive.tar", ext: ".gz"},
		{name: ".gitignore", base: ".gitignore", ext: ""},
		{name: "README", base: "README", ext: ""},
		{name: "file.", base: "file.", ext: ""},
		{name: "a.b.", base: "a.b.", ext: ""},
		{name: ".", base: ".", ext: ""},
		{name: "..", base: "..", ext: ""},
		{name: "", base: "", ext: ""},
		{name: "a.b.c.d", base: "a.b.c", ext: ".d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, ext := splitName(tt.name)
			if base != tt.base || ext != tt.ext {
				t.Errorf("splitName(%q) = %q, %q, want %q, %q", tt.name, base, ext, tt.base, tt.ext)
			}
		})
	}
}

func TestApplyRulesFindReplace(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		params Params
		want   string
	}{
		{
			name:   "literal replace",
			old:    "SKMBT_C360_0001.pdf",
			params: Params{Find: "SKMBT_C360_", Replace: "ACME-invoice-", KeepExtension: true},
			want:   "ACME-invoice-0001.pdf",
		},
		{
			name:   "literal replace that matches nothing",
			old:    "report.txt",
			params: Params{Find: "invoice", Replace: "bill", KeepExtension: true},
			want:   "report.txt",
		},
		{
			name:   "literal replace is case sensitive",
			old:    "IMG_1.jpg",
			params: Params{Find: "img", Replace: "photo", KeepExtension: true},
			want:   "IMG_1.jpg",
		},
		{
			name:   "empty replace deletes the match",
			old:    "SKMBT_C360_0001.pdf",
			params: Params{Find: "SKMBT_C360_", KeepExtension: true},
			want:   "0001.pdf",
		},
		{
			name:   "empty find skips the step",
			old:    "report.txt",
			params: Params{Replace: "never used", KeepExtension: true},
			want:   "report.txt",
		},
		{
			name:   "find without keep extension searches the whole name",
			old:    "report.txt",
			params: Params{Find: ".txt", Replace: ".pdf"},
			want:   "report.pdf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rename(t, tt.old, tt.params, 0); got != tt.want {
				t.Errorf("applyRules(%q) = %q, want %q", tt.old, got, tt.want)
			}
		})
	}
}

func TestApplyRulesRegex(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		params Params
		want   string
	}{
		{
			name:   "leading scanner prefix removed",
			old:    "SKMBT_36018_0007.pdf",
			params: Params{Find: `^SKMBT_\d+_`, UseRegex: true, KeepExtension: true},
			want:   "0007.pdf",
		},
		{
			name:   "captured group put back",
			old:    "file12.txt",
			params: Params{Find: `(\d+)`, Replace: "no-$1", UseRegex: true, KeepExtension: true},
			want:   "fileno-12.txt",
		},
		{
			name:   "double dollar gives a literal dollar",
			old:    "x.txt",
			params: Params{Find: "x", Replace: "$$", UseRegex: true, KeepExtension: true},
			want:   "$.txt",
		},
		{
			name:   "case insensitive flag",
			old:    "IMG_1.jpg",
			params: Params{Find: "(?i)img", Replace: "photo", UseRegex: true, KeepExtension: true},
			want:   "photo_1.jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rename(t, tt.old, tt.params, 0); got != tt.want {
				t.Errorf("applyRules(%q) = %q, want %q", tt.old, got, tt.want)
			}
		})
	}
}

func TestInvalidRegex(t *testing.T) {
	for _, pattern := range []string{"(", "[a-", "*"} {
		t.Run(pattern, func(t *testing.T) {
			_, _, err := Params{Find: pattern, UseRegex: true}.rules()
			if err == nil {
				t.Fatalf("rules() accepted the broken pattern %q", pattern)
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
			}
			want := fmt.Sprintf("That search pattern could not be read. The part it did not understand is %q. "+
				"If you did not mean to use a pattern, untick \"Use a pattern\" and the text will be matched exactly as typed.",
				pattern)
			if got := core.MessageOf(err); got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "my_report-final v2", want: "My_Report-Final V2"},
		{in: "HELLO WORLD", want: "Hello World"},
		{in: "über bericht", want: "Über Bericht"},
		{in: "a", want: "A"},
		{in: "", want: ""},
		{in: "2fast", want: "2fast"},
		{in: "one.two", want: "One.Two"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := titleCase(tt.in); got != tt.want {
				t.Errorf("titleCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestApplyRulesCase(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		params Params
		want   string
	}{
		{name: "lower keeps the extension", old: "REPORT.TXT", params: Params{Case: caseLower, KeepExtension: true}, want: "report.TXT"},
		{name: "lower over the whole name", old: "REPORT.TXT", params: Params{Case: caseLower}, want: "report.txt"},
		{name: "upper keeps the extension", old: "report.txt", params: Params{Case: caseUpper, KeepExtension: true}, want: "REPORT.txt"},
		{name: "upper over the whole name", old: "report.txt", params: Params{Case: caseUpper}, want: "REPORT.TXT"},
		{name: "title keeps the extension", old: "my report.txt", params: Params{Case: caseTitle, KeepExtension: true}, want: "My Report.txt"},
		{name: "title over the whole name", old: "my report.txt", params: Params{Case: caseTitle}, want: "My Report.Txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rename(t, tt.old, tt.params, 0); got != tt.want {
				t.Errorf("applyRules(%q) = %q, want %q", tt.old, got, tt.want)
			}
		})
	}
}

func TestApplyRulesPrefixSuffix(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		params Params
		want   string
	}{
		{name: "prefix only", old: "report.txt", params: Params{Prefix: "ACME-", KeepExtension: true}, want: "ACME-report.txt"},
		{name: "suffix only", old: "report.txt", params: Params{Suffix: "-v2", KeepExtension: true}, want: "report-v2.txt"},
		{name: "both", old: "report.txt", params: Params{Prefix: "ACME-", Suffix: "-v2", KeepExtension: true}, want: "ACME-report-v2.txt"},
		{name: "suffix after the extension when it is not kept", old: "report.txt", params: Params{Suffix: "-v2"}, want: "report.txt-v2"},
		{name: "prefix sits in the same place either way", old: "report.txt", params: Params{Prefix: "ACME-"}, want: "ACME-report.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rename(t, tt.old, tt.params, 0); got != tt.want {
				t.Errorf("applyRules(%q) = %q, want %q", tt.old, got, tt.want)
			}
		})
	}
}

func TestApplyRulesNumbering(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		params Params
		index  int
		want   string
	}{
		{name: "first of five", old: "a.txt", params: Params{Number: true, Start: 1, Step: 1, KeepExtension: true}, index: 0, want: "a1.txt"},
		{name: "second of five", old: "a.txt", params: Params{Number: true, Start: 1, Step: 1, KeepExtension: true}, index: 1, want: "a2.txt"},
		{name: "third of five", old: "a.txt", params: Params{Number: true, Start: 1, Step: 1, KeepExtension: true}, index: 2, want: "a3.txt"},
		{name: "fourth of five", old: "a.txt", params: Params{Number: true, Start: 1, Step: 1, KeepExtension: true}, index: 3, want: "a4.txt"},
		{name: "fifth of five", old: "a.txt", params: Params{Number: true, Start: 1, Step: 1, KeepExtension: true}, index: 4, want: "a5.txt"},
		{name: "padded to three", old: "a.txt", params: Params{Number: true, Start: 1, Step: 1, Padding: 3, KeepExtension: true}, index: 0, want: "a001.txt"},
		{name: "wider than the padding is not truncated", old: "a.txt", params: Params{Number: true, Start: 1000, Step: 1, Padding: 3, KeepExtension: true}, index: 0, want: "a1000.txt"},
		{name: "start at zero", old: "a.txt", params: Params{Number: true, Start: 0, Step: 1, KeepExtension: true}, index: 0, want: "a0.txt"},
		{name: "step of ten", old: "a.txt", params: Params{Number: true, Start: 10, Step: 10, KeepExtension: true}, index: 2, want: "a30.txt"},
		{name: "token in the prefix", old: "a.txt", params: Params{Prefix: "{n}-", Number: true, Start: 7, Step: 1, KeepExtension: true}, index: 0, want: "7-a.txt"},
		{name: "token in the suffix", old: "a.txt", params: Params{Suffix: "-{n}", Number: true, Start: 7, Step: 1, KeepExtension: true}, index: 0, want: "a-7.txt"},
		{name: "token in the replacement", old: "scan_x.txt", params: Params{Find: "x", Replace: "{n}", Number: true, Start: 4, Step: 1, KeepExtension: true}, index: 0, want: "scan_4.txt"},
		{name: "no token appends the number", old: "a.txt", params: Params{Number: true, Start: 4, Step: 1, KeepExtension: true}, index: 0, want: "a4.txt"},
		{name: "both tokens are replaced", old: "a.txt", params: Params{Prefix: "{n}-", Suffix: "-{n}", Number: true, Start: 4, Step: 1, KeepExtension: true}, index: 0, want: "4-a-4.txt"},
		{name: "an upper case token is not the token", old: "a.txt", params: Params{Suffix: "-{N}", Number: true, Start: 4, Step: 1, KeepExtension: true}, index: 0, want: "a-{N}4.txt"},
		{name: "numbering off leaves the name alone", old: "a.txt", params: Params{Start: 4, Step: 1, KeepExtension: true}, index: 3, want: "a.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rename(t, tt.old, tt.params, tt.index); got != tt.want {
				t.Errorf("applyRules(%q, index %d) = %q, want %q", tt.old, tt.index, got, tt.want)
			}
		})
	}
}

// TestApplyRulesOrder pins the pipeline order down with one name that goes
// through every step. Find runs before the case change (the replaced words are
// title cased), the case change runs before the prefix (the prefix keeps its
// small letters) and the numbering runs last (the token typed into the suffix
// is filled in).
func TestApplyRulesOrder(t *testing.T) {
	p := Params{
		Find:          "skmbt_c36_",
		Replace:       "invoice ",
		Case:          caseTitle,
		Prefix:        "acme-",
		Suffix:        " v2{n}",
		Number:        true,
		Start:         1,
		Step:          1,
		Padding:       2,
		KeepExtension: true,
	}
	const want = "acme-Invoice Report v204.PDF"
	if got := rename(t, "skmbt_c36_report.PDF", p, 3); got != want {
		t.Errorf("applyRules = %q, want %q", got, want)
	}
}

func TestRulesValidation(t *testing.T) {
	tests := []struct {
		name   string
		params Params
		want   string
	}{
		{
			name:   "unknown case option",
			params: Params{Case: "sentence"},
			want:   "That is not one of the case options. Choose Leave alone, UPPER CASE, lower case or Title Case.",
		},
		{
			name:   "start too high",
			params: Params{Number: true, Start: 1000000},
			want:   "The first number must be between 0 and 999999.",
		},
		{
			name:   "start below zero",
			params: Params{Number: true, Start: -1},
			want:   "The first number must be between 0 and 999999.",
		},
		{
			name:   "step too high",
			params: Params{Number: true, Step: 1001},
			want:   "The step must be between 1 and 1000.",
		},
		{
			name:   "step below one",
			params: Params{Number: true, Step: -1},
			want:   "The step must be between 1 and 1000.",
		},
		{
			name:   "padding too wide",
			params: Params{Number: true, Padding: 11},
			want:   "Pad to must be between 0 and 10 digits.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := tt.params.rules()
			if err == nil {
				t.Fatal("rules() accepted a set of rules it should have rejected")
			}
			if code := core.CodeOf(err); code != core.CodeInvalidInput {
				t.Errorf("code = %q, want %q", code, core.CodeInvalidInput)
			}
			if got := core.MessageOf(err); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRulesFillsInTheDefaults(t *testing.T) {
	checked, re, err := Params{Number: true, Start: 1}.rules()
	if err != nil {
		t.Fatalf("rules() rejected a good set of rules: %v", err)
	}
	if checked.Step != 1 {
		t.Errorf("Step = %d, want 1", checked.Step)
	}
	if re != nil {
		t.Error("a pattern was compiled for rules that do not use one")
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "single digit before double", a: "IMG_9.jpg", b: "IMG_10.jpg", want: true},
		{name: "double digit after single", a: "IMG_10.jpg", b: "IMG_9.jpg", want: false},
		{name: "a2 before a10", a: "a2", b: "a10", want: true},
		{name: "file1 before file01", a: "file1", b: "file01", want: true},
		{name: "file01 after file1", a: "file01", b: "file1", want: false},
		{name: "prefix first", a: "a", b: "ab", want: true},
		{name: "longer after its prefix", a: "ab", b: "a", want: false},
		{name: "empty first", a: "", b: "a", want: true},
		{name: "empty is not after itself", a: "", b: "", want: false},
		{name: "capital before small", a: "A", b: "a", want: true},
		{name: "small after capital", a: "a", b: "A", want: false},
		{name: "2 before 10", a: "2", b: "10", want: true},
		{name: "v1.2 before v1.10", a: "v1.2", b: "v1.10", want: true},
		{name: "case insensitive letters first", a: "apple", b: "Banana", want: true},
		{name: "digits before letters", a: "1file", b: "afile", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := naturalLess(tt.a, tt.b); got != tt.want {
				t.Errorf("naturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCheckNameIllegalCharacters(t *testing.T) {
	for _, char := range []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"} {
		t.Run(char, func(t *testing.T) {
			want := fmt.Sprintf("A file name cannot contain the character %q. Windows will not allow it.", char)
			if got := checkName("report" + char + "1.txt"); got != want {
				t.Errorf("checkName = %q, want %q", got, want)
			}
		})
	}

	t.Run("control character", func(t *testing.T) {
		const want = "That name would contain an invisible control character, which no operating system will accept."
		if got := checkName("report\x01.txt"); got != want {
			t.Errorf("checkName = %q, want %q", got, want)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		const want = "The rules leave this file with no name at all. Add a prefix, a suffix, or some text to keep."
		if got := checkName(""); got != want {
			t.Errorf("checkName = %q, want %q", got, want)
		}
	})

	t.Run("empty stem with an extension kept", func(t *testing.T) {
		// invoice.txt renamed to ".txt" is a hidden file with no name, which on
		// Linux disappears from the folder listing entirely. The spec requires
		// preview to catch empty names, and this is one.
		const want = "The rules leave this file with no name in front of its extension, so it would become a hidden file called \".txt\". Add a prefix, a suffix, or some text to keep."
		if got := checkName(".txt"); got != want {
			t.Errorf("checkName = %q, want %q", got, want)
		}
	})

	t.Run("a legal name passes", func(t *testing.T) {
		if got := checkName("ACME-invoice-001.pdf"); got != "" {
			t.Errorf("checkName = %q, want an empty string", got)
		}
	})
}

func TestCheckNameTrailing(t *testing.T) {
	const want = "A file name cannot end with a dot or a space on Windows. Windows quietly removes them, which would give the file a name you did not ask for."
	for _, name := range []string{"name.", "name ", "name.txt.", "name.txt "} {
		t.Run(name, func(t *testing.T) {
			if got := checkName(name); got != want {
				t.Errorf("checkName(%q) = %q, want %q", name, got, want)
			}
		})
	}
	if got := checkName("name.txt"); got != "" {
		t.Errorf("checkName(\"name.txt\") = %q, want an empty string", got)
	}
}

func TestCheckNameReserved(t *testing.T) {
	blocked := []string{"CON", "con", "CON.txt", "NUL", "COM1", "COM9", "LPT1", "LPT9"}
	for _, name := range blocked {
		t.Run("blocked "+name, func(t *testing.T) {
			stem, _, _ := strings.Cut(name, ".")
			want := fmt.Sprintf("%s is a name Windows reserves for hardware, so no file can be called that. Choose something else.", stem)
			if got := checkName(name); got != want {
				t.Errorf("checkName(%q) = %q, want %q", name, got, want)
			}
		})
	}
	for _, name := range []string{"COM0", "LPT0", "CONSOLE", "CONSOLE.txt", "MYCON"} {
		t.Run("allowed "+name, func(t *testing.T) {
			if got := checkName(name); got != "" {
				t.Errorf("checkName(%q) = %q, want an empty string", name, got)
			}
		})
	}
}

func TestCheckNameLength(t *testing.T) {
	t.Run("exactly 255 runes is fine", func(t *testing.T) {
		if got := checkName(strings.Repeat("a", 255)); got != "" {
			t.Errorf("checkName = %q, want an empty string", got)
		}
	})
	t.Run("256 runes is blocked", func(t *testing.T) {
		const want = "That name would be 256 characters long. Most file systems stop at 255, so this file was not renamed."
		if got := checkName(strings.Repeat("a", 256)); got != want {
			t.Errorf("checkName = %q, want %q", got, want)
		}
	})
	t.Run("multi-byte characters count as runes", func(t *testing.T) {
		// 200 three-byte runes is 600 bytes, so a byte count would block this.
		if got := checkName(strings.Repeat("字", 200)); got != "" {
			t.Errorf("checkName = %q, want an empty string", got)
		}
	})
}

// applyRules is handed a compiled pattern, so this guards the pairing the whole
// package relies on: a nil pattern means the find step is literal text.
func TestApplyRulesUsesTheCompiledPattern(t *testing.T) {
	re := regexp.MustCompile(`\d+`)
	if got := applyRules("file12.txt", Params{Find: `\d+`, Replace: "n", UseRegex: true, KeepExtension: true}, 0, re); got != "filen.txt" {
		t.Errorf("with a pattern: got %q, want %q", got, "filen.txt")
	}
	if got := applyRules(`file\d+.txt`, Params{Find: `\d+`, Replace: "n"}, 0, nil); got != "filen.txt" {
		t.Errorf("without a pattern: got %q, want %q", got, "filen.txt")
	}
}
