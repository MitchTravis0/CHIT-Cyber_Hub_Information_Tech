package lanspeed

import (
	"html"
	"strconv"
	"strings"
)

// IndexHTML builds the page the other machine sees.
//
// It is one self-contained document: no stylesheet, no script, no image and no
// font is fetched from anywhere, because the machine on the other end may have
// no internet at all, and because a page that runs script on somebody else's
// machine is not something a diagnostic tool should hand out. The colours are
// written as literal hex, which is the one place in CHIT where that is
// allowed: this page is rendered by somebody else's browser, which has never
// heard of the app's theme tokens.
//
// The markup is deliberately XHTML-clean (every element closed, every attribute
// quoted) so a test can parse it with an XML parser rather than checking it
// against the code that produced it.
func IndexHTML(token string, sizeMB int, url string) string {
	size := strconv.Itoa(sizeMB)
	safeToken := html.EscapeString(token)

	var b strings.Builder
	b.WriteString(`<!doctype html>` + "\n")
	b.WriteString(`<html lang="en">` + "\n<head>\n")
	b.WriteString(`<meta charset="utf-8"></meta>` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1"></meta>` + "\n")
	b.WriteString(`<title>Network speed test</title>` + "\n")
	b.WriteString("<style>\n" + pageCSS + "</style>\n")
	b.WriteString("</head>\n<body>\n")

	b.WriteString(`<h1>Network speed test</h1>` + "\n")
	b.WriteString(`<p class="lead">Somebody on this network is measuring the link between their computer and this one. ` +
		`Click the button below. Your browser will download ` + size + ` MB and you can throw the file away afterwards.</p>` + "\n")

	b.WriteString(`<p><a class="go" href="/t/` + safeToken + `/dl">Start the test (` + size + ` MB)</a></p>` + "\n")

	b.WriteString(`<h2>Or from a terminal</h2>` + "\n")
	b.WriteString(`<pre>` + html.EscapeString(curlFor(url)) + `</pre>` + "\n")

	b.WriteString(`<p class="foot">The result appears on the other computer, not on this one. ` +
		`Nothing is saved on either machine: the data is made up as it is sent.</p>` + "\n")
	b.WriteString("</body>\n</html>\n")

	return b.String()
}

// pageCSS is small on purpose: it has to be readable on a phone held sideways in
// a comms cupboard, and nothing more.
const pageCSS = `body { font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
  max-width: 40rem; margin: 0 auto; padding: 1.5rem 1rem;
  background: #10131a; color: #e6e9ef; line-height: 1.5; }
h1 { font-size: 1.25rem; margin: 0 0 0.25rem; }
h2 { font-size: 1rem; margin: 1.5rem 0 0.5rem; }
.lead, .foot { color: #9aa4b8; font-size: 0.85rem; }
.foot { margin-top: 2rem; border-top: 1px solid #2a3040; padding-top: 0.75rem; }
a.go { display: inline-block; background: #2563eb; color: #fff; text-decoration: none;
  border-radius: 4px; padding: 0.6rem 1.1rem; font-size: 1rem; margin: 0.75rem 0; }
pre { border: 1px solid #2a3040; border-radius: 4px; padding: 0.6rem 0.75rem;
  overflow-x: auto; font-size: 0.85rem; color: #e6e9ef; }
`
