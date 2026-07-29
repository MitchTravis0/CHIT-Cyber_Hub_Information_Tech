package filedrop

import (
	"html"
	"strconv"
	"strings"
)

// IndexHTML builds the page the other machine sees.
//
// It is one self-contained document: no stylesheet, no script, no image, no
// font is fetched from anywhere, because the machine on the other end may have
// no internet at all. The colours are written as literal hex, which is the one
// place in CHIT where that is allowed: this page is rendered by somebody
// else's browser, which has never heard of the app's theme tokens.
//
// The markup is deliberately XHTML-clean (every element closed, every attribute
// quoted) so a test can parse it with an XML parser rather than checking it
// against the code that produced it.
func IndexHTML(token string, shares []Share, allowUpload bool) string {
	var b strings.Builder

	b.WriteString(`<!doctype html>` + "\n")
	b.WriteString(`<html lang="en">` + "\n<head>\n")
	b.WriteString(`<meta charset="utf-8"></meta>` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1"></meta>` + "\n")
	b.WriteString(`<title>Files shared with you</title>` + "\n")
	b.WriteString("<style>\n" + pageCSS + "</style>\n")
	b.WriteString("</head>\n<body>\n")

	b.WriteString(`<h1>Files shared with you</h1>` + "\n")
	b.WriteString(`<p class="lead">Somebody on this network is handing you these files from CHIT. ` +
		`The link stops working the moment they press Stop sharing.</p>` + "\n")

	if len(shares) == 0 {
		b.WriteString(`<p class="empty">There is nothing to download.</p>` + "\n")
	} else {
		b.WriteString("<ul>\n")
		for i, share := range shares {
			b.WriteString(`<li><a href="f/` + strconv.Itoa(i) + `">` + html.EscapeString(share.Name) +
				`</a> <span class="size">` + humanBytes(share.Bytes) + `</span></li>` + "\n")
		}
		b.WriteString("</ul>\n")
	}

	if allowUpload {
		b.WriteString(`<h2>Send a file back</h2>` + "\n")
		b.WriteString(`<form method="post" action="up" enctype="multipart/form-data">` + "\n")
		b.WriteString(`<input type="file" name="file"></input>` + "\n")
		b.WriteString(`<button type="submit">Send</button>` + "\n")
		b.WriteString("</form>\n")
	}

	b.WriteString(`<p class="foot">Shared with CHIT over your local network. Nothing here is encrypted, ` +
		`so treat it the way you would treat a USB stick left on a desk.</p>` + "\n")
	b.WriteString("</body>\n</html>\n")

	return b.String()
}

// pageCSS is small on purpose: it has to be readable on a phone held sideways
// in a comms cupboard, and nothing more.
const pageCSS = `body { font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
  max-width: 40rem; margin: 0 auto; padding: 1.5rem 1rem;
  background: #10131a; color: #e6e9ef; line-height: 1.5; }
h1 { font-size: 1.25rem; margin: 0 0 0.25rem; }
h2 { font-size: 1rem; margin: 1.5rem 0 0.5rem; }
.lead, .foot { color: #9aa4b8; font-size: 0.85rem; }
.foot { margin-top: 2rem; border-top: 1px solid #2a3040; padding-top: 0.75rem; }
.empty { color: #9aa4b8; }
ul { list-style: none; padding: 0; margin: 1rem 0; }
li { border: 1px solid #2a3040; border-radius: 4px; padding: 0.6rem 0.75rem; margin-bottom: 0.4rem;
  display: flex; justify-content: space-between; gap: 0.75rem; align-items: baseline; }
a { color: #6ea8fe; text-decoration: none; word-break: break-all; }
a:hover { text-decoration: underline; }
.size { color: #9aa4b8; font-size: 0.85rem; white-space: nowrap; }
form { border: 1px solid #2a3040; border-radius: 4px; padding: 0.75rem; }
button { background: #2563eb; color: #fff; border: 0; border-radius: 4px;
  padding: 0.45rem 0.9rem; font-size: 0.9rem; cursor: pointer; margin-top: 0.5rem; }
input[type="file"] { color: #e6e9ef; max-width: 100%; }
`

// humanBytes writes a size the way the app does, kept here rather than shared
// because this string goes into somebody else's browser and must not move when
// the app's own formatting changes.
func humanBytes(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + units[unit]
}
