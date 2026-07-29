package urlcheck

// This is an approximation of the public suffix list, not the list itself.
// It is wrong in three known ways:
//
//  1. A country domain whose second level is not in multiSuffixes gets two
//     labels when it should get three. example.com.gt returns com.gt.
//  2. A shared hosting domain (github.io, blogspot.com and the rest) is a public
//     suffix in reality, so Registrable returns the hosting provider rather than
//     the site owner. ComparableDomain works around it for the domains in
//     hostingSuffixes and nowhere else.
//  3. New multi-label suffixes appear over time and this list does not update.
//
// Every one of those makes the cross-domain finding read as "same owner" when it
// is really "different owner", so the failure mode is a missed warning rather
// than a false alarm. That is the right way round for a tool a tech reads, but
// it is not a substitute for the real list.

import (
	"net/netip"
	"strings"
)

// multiSuffixes are the two-label suffixes under which names are registered, so
// a host below one of them needs three labels rather than two.
var multiSuffixes = map[string]bool{
	"ac.uk": true, "co.uk": true, "gov.uk": true, "ltd.uk": true, "me.uk": true,
	"net.uk": true, "nhs.uk": true, "org.uk": true, "plc.uk": true, "police.uk": true,
	"sch.uk": true,
	"com.au": true, "net.au": true, "org.au": true, "edu.au": true, "gov.au": true,
	"asn.au": true, "id.au": true,
	"co.nz": true, "net.nz": true, "org.nz": true, "govt.nz": true, "ac.nz": true,
	"school.nz": true,
	"co.za":     true, "org.za": true, "net.za": true, "gov.za": true, "ac.za": true,
	"web.za": true,
	"co.jp":  true, "ne.jp": true, "or.jp": true, "ac.jp": true, "go.jp": true,
	"ad.jp": true, "lg.jp": true,
	"com.br": true, "net.br": true, "org.br": true, "gov.br": true, "edu.br": true,
	"com.cn": true, "net.cn": true, "org.cn": true, "gov.cn": true, "edu.cn": true,
	"ac.cn": true,
	"co.in": true, "net.in": true, "org.in": true, "gen.in": true, "firm.in": true,
	"ind.in": true, "gov.in": true, "ac.in": true, "edu.in": true, "res.in": true,
	"com.mx": true, "org.mx": true, "gob.mx": true, "edu.mx": true, "net.mx": true,
	"com.sg": true, "net.sg": true, "org.sg": true, "gov.sg": true, "edu.sg": true,
	"com.hk": true, "net.hk": true, "org.hk": true, "gov.hk": true, "edu.hk": true,
	"idv.hk": true,
	"com.tr": true, "net.tr": true, "org.tr": true, "gov.tr": true, "edu.tr": true,
	"com.tw": true, "net.tw": true, "org.tw": true, "gov.tw": true, "edu.tw": true,
	"co.kr": true, "ne.kr": true, "or.kr": true, "go.kr": true, "re.kr": true,
	"pe.kr":  true,
	"com.ar": true, "net.ar": true, "org.ar": true, "gov.ar": true, "edu.ar": true,
	"co.il": true, "net.il": true, "org.il": true, "gov.il": true, "ac.il": true,
	"com.my": true, "net.my": true, "org.my": true, "gov.my": true, "edu.my": true,
	"co.id": true, "or.id": true, "ac.id": true, "go.id": true, "web.id": true,
	"com.pl": true, "net.pl": true, "org.pl": true, "gov.pl": true, "edu.pl": true,
	"com.ua": true, "net.ua": true, "org.ua": true, "gov.ua": true, "edu.ua": true,
	"com.ru": true, "net.ru": true, "org.ru": true,
	"com.ph": true, "net.ph": true, "org.ph": true, "gov.ph": true, "edu.ph": true,
	"com.vn": true, "net.vn": true, "org.vn": true, "gov.vn": true, "edu.vn": true,
	"com.pk": true, "net.pk": true, "org.pk": true, "gov.pk": true, "edu.pk": true,
	"com.eg": true, "net.eg": true, "org.eg": true, "gov.eg": true, "edu.eg": true,
	"com.sa": true, "net.sa": true, "org.sa": true, "gov.sa": true, "edu.sa": true,
	"com.co": true, "net.co": true, "org.co": true, "gov.co": true, "edu.co": true,
	"co.ke": true, "or.ke": true, "ac.ke": true, "go.ke": true,
	"com.ng": true, "net.ng": true, "org.ng": true, "gov.ng": true, "edu.ng": true,
	"com.gh": true, "org.gh": true, "gov.gh": true, "edu.gh": true,
}

// hostingSuffixes are the shared hosting domains where the registrable domain is
// the hosting company rather than the person who put the page there.
var hostingSuffixes = map[string]bool{
	"github.io": true, "blogspot.com": true, "s3.amazonaws.com": true, "web.app": true,
	"pages.dev": true, "workers.dev": true, "vercel.app": true, "netlify.app": true,
	"azurewebsites.net": true, "sharepoint.com": true, "cloudfront.net": true,
	"firebaseapp.com": true, "herokuapp.com": true, "wixsite.com": true, "weebly.com": true,
	"glitch.me": true, "repl.co": true, "onrender.com": true, "surge.sh": true,
	"000webhostapp.com": true,
}

// shorteners are the link shorteners worth naming, matched against the
// registrable domain so www.bit.ly and bit.ly are the same service.
var shorteners = map[string]bool{
	"bit.ly": true, "tinyurl.com": true, "t.co": true, "goo.gl": true, "ow.ly": true,
	"is.gd": true, "buff.ly": true, "rebrand.ly": true, "cutt.ly": true, "shorturl.at": true,
	"rb.gy": true, "bl.ink": true, "lnkd.in": true, "tiny.cc": true, "s.id": true,
	"t.ly": true, "short.io": true, "v.gd": true, "x.co": true, "tr.im": true,
	"po.st": true, "mcaf.ee": true, "soo.gd": true, "u.to": true, "clck.ru": true,
	"qr.ae": true, "adf.ly": true, "bitly.com": true, "trib.al": true, "dlvr.it": true,
	"ift.tt": true, "amzn.to": true, "youtu.be": true, "fb.me": true, "wp.me": true,
	"git.io": true, "shorte.st": true, "linktr.ee": true, "bit.do": true, "zpr.io": true,
}

// Registrable approximates the registrable domain (eTLD+1) without the public
// suffix list, which CHIT cannot depend on. It is the last two labels, or the
// last three when the last two are one of the multi-label suffixes below. It is
// an approximation: see the note in domain.go for where it is wrong.
func Registrable(host string) string {
	h := cleanHost(host)
	if _, err := netip.ParseAddr(h); err == nil {
		return h
	}
	labels := strings.Split(h, ".")
	if len(labels) < 3 {
		return h
	}
	if multiSuffixes[strings.Join(labels[len(labels)-2:], ".")] {
		return strings.Join(labels[len(labels)-3:], ".")
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

// ComparableDomain is Registrable, except on the shared hosting suffixes where
// two different people own two different names under the same registrable
// domain. There it uses one more label, so evil.github.io and good.github.io
// compare as different owners, which for phishing is the answer that matters.
func ComparableDomain(host string) string {
	h := cleanHost(host)
	// Matching the suffix rather than testing Registrable's answer against the
	// list is what makes the three-label entry (s3.amazonaws.com) work: its
	// registrable domain is amazonaws.com, which is not in the list.
	for suffix := range hostingSuffixes {
		if !strings.HasSuffix(h, "."+suffix) {
			continue
		}
		owner := strings.Split(strings.TrimSuffix(h, "."+suffix), ".")
		return owner[len(owner)-1] + "." + suffix
	}
	return Registrable(h)
}

// IsShortener reports whether host is a known link shortener.
func IsShortener(host string) bool {
	return shorteners[Registrable(host)]
}

// cleanHost puts a host into the one form the lists are written in. A trailing
// dot is the same name, and a port is not part of it.
func cleanHost(host string) string {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if _, err := netip.ParseAddr(h); err == nil {
		return h
	}
	// An IPv6 literal is already dealt with above, so a colon here is a port.
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	return h
}
