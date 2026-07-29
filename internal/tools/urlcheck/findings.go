package urlcheck

import (
	"fmt"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// executableExts are the extensions that download a program rather than open a
// page. .zip, .js and .pdf are deliberately absent: they are far too common on
// legitimate links, and a warning that fires constantly is one nobody reads.
var executableExts = []string{
	".exe", ".scr", ".msi", ".bat", ".cmd", ".pif", ".vbs", ".vbe", ".wsf", ".hta",
	".jar", ".apk", ".lnk", ".reg", ".msc", ".cpl", ".ps1", ".iso", ".img", ".dmg",
}

// findingURLDefenseV3 and findingMimecast are produced both by Unwrap, which
// meets these two wrappers while stripping the others, and by findings, which
// re-derives them from the finished report. They are written once here so the
// two can never drift apart.
func findingURLDefenseV3() Finding {
	return Finding{
		ID:       "urldefense-v3",
		Severity: "info",
		Text:     "This link is wrapped in Proofpoint's newer URL Defense format, which CHIT cannot unwrap. The chain below follows it, so the destination still shows up.",
	}
}

func findingMimecast() Finding {
	return Finding{
		ID:       "mimecast",
		Severity: "info",
		Text:     "This link is wrapped by Mimecast, which hides the real address behind a code only Mimecast can look up. Following it is the only way to see where it goes.",
	}
}

// punycodeHost is the host the punycode finding names. The final host comes
// first because that is where the user would land, but a lookalike domain that
// redirects to its own UTF-8 spelling ends on a host with no xn-- in it, and
// then the only address written in xn-- form is the one the user was sent.
// Looking at the final host alone leaves that link with no homograph warning
// at all.
func punycodeHost(r *Report) HostName {
	if r.FinalHost.Punycode {
		return r.FinalHost
	}
	return r.StartHost
}

// findings decides what to tell the tech about the report. Pure: it reads only
// what the walk already collected, makes no network calls and reads no clock.
func findings(r *Report) []Finding {
	out := []Finding{}
	add := func(id, severity, text string) {
		out = append(out, Finding{ID: id, Severity: severity, Text: text})
	}

	if host := blockedHop(r); host != "" {
		add("private-target", "danger",
			fmt.Sprintf("The link points at an address on this computer or the local network (%s). A link from outside should never do that.", host))
	}
	if scheme := stoppedScheme(r); scheme != "" {
		add("bad-scheme", "danger", badSchemeText(scheme))
	}
	if host := punycodeHost(r); host.Punycode {
		add("punycode", "danger",
			fmt.Sprintf("The address \"%s\" is really \"%s\". Those are not the ordinary Latin letters, which is how a fake address is made to look like a real one.",
				host.Raw, host.Decoded))
	}
	if label := mixedScriptLabel(r.FinalHost.Decoded); label != "" {
		add("mixed-script", "danger",
			fmt.Sprintf("The address mixes Latin letters with Cyrillic or Greek ones in the same word (\"%s\"). That is almost always an attempt to make a fake address look like a real one.", label))
	}
	if user, host := credentialsIn(r); user != "" {
		add("credentials", "danger",
			fmt.Sprintf("The link has \"%s@\" in front of the address. A browser ignores everything before the @, so the page you land on is %s, not what the link appears to say.", user, host))
	}
	if name := executableName(r.Final); name != "" {
		add("executable", "danger",
			fmt.Sprintf("The link ends in a file called \"%s\". That downloads a program rather than opening a page. Do not run it.", name))
	}

	if r.FinalHost.IsIP {
		add("ip-literal", "warn",
			fmt.Sprintf("The link goes to a bare IP address (%s) instead of a name. Real services almost never send people to a number.", r.FinalHost.Raw))
	}
	if schemeOf(r.Final) == "http" {
		add("insecure", "warn",
			"The page it ends on is plain http, not https, so anything typed into it travels unencrypted. A real login page is never plain http.")
	}
	if host := downgradeHop(r); host != "" {
		add("downgrade", "warn",
			fmt.Sprintf("The chain drops from https down to plain http part way through, at %s.", host))
	}
	startOwner, finalOwner := ComparableDomain(r.StartHost.Raw), ComparableDomain(r.FinalHost.Raw)
	if startOwner != "" && finalOwner != "" && startOwner != finalOwner {
		add("cross-domain", "warn",
			fmt.Sprintf("The link starts at %s and ends up at %s, which is a different owner. That is normal for a mailing list and a red flag on a bank or Microsoft link.", startOwner, finalOwner))
	}
	if short := firstShortener(r); short != "" {
		add("shortener", "warn",
			fmt.Sprintf("The link goes through %s, a link shortener, which hides the real address until you follow it.", short))
	}
	if host := Registrable(r.FinalHost.Raw); hostingSuffixes[host] {
		add("free-hosting", "warn",
			fmt.Sprintf("The page is hosted on %s, which anyone can sign up to for free. A real company login page is almost never on one of these.", host))
	}
	if port := unusualPort(r.Final); port != 0 {
		add("unusual-port", "warn",
			fmt.Sprintf("The link uses port %d rather than the usual 80 or 443. A real website almost never does that.", port))
	}
	if r.Age.Known && r.Age.Days < newDomainDays {
		add("new-domain", "warn",
			fmt.Sprintf("The domain %s was only registered on %s, %s ago. Brand new domains are what phishing campaigns run on.",
				r.FinalHost.Registrable, r.Age.Registered, r.Age.Human))
	}
	if r.Stopped == stoppedLoop {
		add("redirect-loop", "warn",
			"The chain loops back on itself. A page that does this usually depends on a cookie or a login that the inspector does not have.")
	}
	cappedOut := r.Stopped == fmt.Sprintf(stoppedHopCap, len(r.Hops))
	if cappedOut {
		add("hop-cap", "warn",
			fmt.Sprintf("The chain was still redirecting after %d hops, so where it really ends is not known.", len(r.Hops)))
	}
	if redirectWithNoLocation(r) {
		add("no-location", "warn",
			"One step said it was redirecting but did not say where to, so the chain stops there.")
	}

	if len(r.Hops) >= 4 && !cappedOut {
		add("long-chain", "info",
			fmt.Sprintf("It took %d redirects to get there. That is normal for advertising and tracking links, and unusual for a link a person sent you.", len(r.Hops)-1))
	}
	if anyURL(r, isURLDefenseV3) {
		out = append(out, findingURLDefenseV3())
	}
	if anyURL(r, isMimecastURL) {
		out = append(out, findingMimecast())
	}

	rank := map[string]int{"danger": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(a, b int) bool { return rank[out[a].Severity] < rank[out[b].Severity] })
	return out
}

// answered reports whether any step of the chain got a reply. A hop that failed
// carries Status 0 and an Error sentence.
func answered(r *Report) bool {
	for _, hop := range r.Hops {
		if hop.Status > 0 {
			return true
		}
	}
	return false
}

// levelFor is the report's verdict: the worst finding, except that a chain
// nothing ever answered is "unknown" rather than "ok". Saying "ok" about a link
// CHIT could not reach contradicts the hop list on the same page, and is the
// one verdict this tool must not give for something it did not check. Matches
// how breach-checker already reports "could not check".
func levelFor(r *Report) string {
	level := levelOf(r.Findings)
	if level == "ok" && !answered(r) {
		return "unknown"
	}
	return level
}

// levelOf is the worst severity in the list. Info findings never raise it.
func levelOf(list []Finding) string {
	level := "ok"
	for _, f := range list {
		if f.Severity == "danger" {
			return "danger"
		}
		if f.Severity == "warn" {
			level = "warn"
		}
	}
	return level
}

// headline words the report in one sentence. Pure.
func headline(r *Report) string {
	warns := 0
	for _, f := range r.Findings {
		switch f.Severity {
		case "danger":
			return fmt.Sprintf("This link ends at %s, and there is something seriously wrong with it. Read the list below before anyone clicks it.", r.FinalHost.Decoded)
		case "warn":
			warns++
		}
	}
	if warns == 1 {
		return fmt.Sprintf("This link ends at %s. There is 1 thing worth checking below before you trust it.", r.FinalHost.Decoded)
	}
	if warns > 1 {
		return fmt.Sprintf("This link ends at %s. There are %d things worth checking below before you trust it.", r.FinalHost.Decoded, warns)
	}
	if !answered(r) {
		return fmt.Sprintf("CHIT could not reach %s, so there is nothing to judge. That is not the same as safe: the address may be wrong, the site may be switched off, or your network may be blocking it.", r.FinalHost.Decoded)
	}
	if len(r.Hops) == 1 {
		return fmt.Sprintf("This link goes straight to %s and nothing about it looks wrong. That is not the same as safe: check the address is one you were expecting.", r.FinalHost.Decoded)
	}
	return fmt.Sprintf("This link goes to %s through %d redirects and nothing about it looks wrong. That is not the same as safe: check the address is one you were expecting.", r.FinalHost.Decoded, len(r.Hops)-1)
}

// humanAge turns a day count into something a tech reads at a glance.
func humanAge(days int) string {
	switch {
	case days < 1:
		return "less than a day"
	case days == 1:
		return "1 day"
	case days <= 30:
		return fmt.Sprintf("%d days", days)
	case days < 365:
		months := days / 30
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
	years := days / 365
	if years == 1 {
		return "1 year"
	}
	return fmt.Sprintf("%d years", years)
}

// mixedScriptLabel returns the first decoded label that mixes Latin letters with
// Cyrillic or Greek ones, or "" when none does. A label entirely in one script is
// fine: a genuinely Chinese, Arabic or Greek domain is normal.
func mixedScriptLabel(decodedHost string) string {
	for _, label := range strings.Split(decodedHost, ".") {
		latin, cyrillic, greek := false, false, false
		for _, r := range label {
			switch {
			case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
				latin = true
			case r >= 0x0400 && r <= 0x04FF:
				cyrillic = true
			case r >= 0x0370 && r <= 0x03FF:
				greek = true
			}
		}
		scripts := 0
		for _, present := range []bool{latin, cyrillic, greek} {
			if present {
				scripts++
			}
		}
		if scripts >= 2 {
			return label
		}
	}
	return ""
}

// badSchemeText names what the redirect pointed at, in the words a tech reads
// out to the person who was sent the link.
func badSchemeText(scheme string) string {
	switch scheme {
	case "javascript":
		return "The link redirects to a javascript: address, which is code rather than a web page. CHIT did not run it. Nothing legitimate does this."
	case "data":
		return "The link redirects to a data: address, which carries the whole page inside the link itself. That is how a fake login page is hidden from a mail filter. CHIT did not open it."
	case "file":
		return "The link redirects to a file: address, which points at a file on the computer rather than at a website. CHIT did not open it."
	}
	return fmt.Sprintf("The link redirects to \"%s:\", which is not a web address. CHIT did not open it. A link that hands off to another program on the machine is worth asking about.", scheme)
}

// executableName is the file at the end of the link when that file is a program,
// and "" otherwise. The query is not part of the file name.
func executableName(final string) string {
	u, err := url.Parse(final)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(u.Path)
	for _, ext := range executableExts {
		if strings.HasSuffix(lower, ext) {
			return u.Path[strings.LastIndexByte(u.Path, '/')+1:]
		}
	}
	return ""
}

// blockedHop is the host of the first hop that was refused because it is on this
// machine or the local network.
func blockedHop(r *Report) string {
	for _, hop := range r.Hops {
		if hop.Error == msgBlockedHop {
			return hop.Host
		}
		if addr, err := netip.ParseAddr(hop.Host); err == nil && isBlocked(addr) {
			return hop.Host
		}
	}
	return ""
}

// stoppedScheme is the scheme of the redirect target that was refused, or "".
func stoppedScheme(r *Report) string {
	for _, hop := range r.Hops {
		if scheme, bad := nonWebScheme(hop.Next); bad {
			return scheme
		}
	}
	return ""
}

// credentialsIn finds the first address in the report that hides its real host
// behind a user:password@ part.
func credentialsIn(r *Report) (user, host string) {
	for _, raw := range append([]string{r.Start}, hopURLs(r)...) {
		u, err := url.Parse(raw)
		if err != nil || u.User == nil {
			continue
		}
		return u.User.String(), u.Hostname()
	}
	return "", ""
}

// downgradeHop is the host of the first hop that redirects an https address to a
// plain http one.
func downgradeHop(r *Report) string {
	for _, hop := range r.Hops {
		if schemeOf(hop.URL) == "https" && schemeOf(hop.Next) == "http" {
			return hop.Host
		}
	}
	return ""
}

// firstShortener is the registrable domain of the first shortener in the chain.
func firstShortener(r *Report) string {
	for _, hop := range r.Hops {
		if IsShortener(hop.Host) {
			return Registrable(hop.Host)
		}
	}
	return ""
}

// unusualPort is the port written into the final address when it is not one of
// the two a website normally answers on, and 0 otherwise.
func unusualPort(final string) int {
	u, err := url.Parse(final)
	if err != nil || u.Port() == "" {
		return 0
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port == 80 || port == 443 {
		return 0
	}
	return port
}

func redirectWithNoLocation(r *Report) bool {
	for _, hop := range r.Hops {
		if hop.Status >= 300 && hop.Status <= 399 && hop.Location == "" {
			return true
		}
	}
	return false
}

func hopURLs(r *Report) []string {
	out := make([]string, 0, len(r.Hops))
	for _, hop := range r.Hops {
		out = append(out, hop.URL)
	}
	return out
}

// anyURL reports whether the first address requested, or any hop, matches.
func anyURL(r *Report, match func(*url.URL) bool) bool {
	for _, raw := range append([]string{r.Start}, hopURLs(r)...) {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if match(u) {
			return true
		}
	}
	return false
}

func isURLDefenseV3(u *url.URL) bool {
	host := cleanHost(u.Hostname())
	return (host == "urldefense.proofpoint.com" || host == "urldefense.com") &&
		strings.HasPrefix(u.Path, "/v3/")
}

func isMimecastURL(u *url.URL) bool {
	return strings.HasSuffix(cleanHost(u.Hostname()), ".mimecast.com")
}

func schemeOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme
}

// nonWebScheme returns the scheme of raw when it is something other than http or
// https, so the walk knows not to fetch it and the findings know to say so.
func nonWebScheme(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Scheme == "http" || u.Scheme == "https" {
		return "", false
	}
	return u.Scheme, true
}
