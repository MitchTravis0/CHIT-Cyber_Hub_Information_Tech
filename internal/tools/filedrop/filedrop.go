// Package filedrop hands files to another machine on the same network through
// its web browser. It runs a small HTTP server for as long as the job runs and
// closes it the moment the job is cancelled.
//
// The security design is in the routing: a file is served by its index in a
// slice resolved before the server starts, so no filesystem path is ever built
// from anything in a request. Path traversal is not defended against here, it
// is made unrepresentable.
package filedrop

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chit/internal/core"
)

// JobKind is the job manager kind for a file drop session.
const JobKind = "filedrop"

// KindTransfer is the result kind of every request the server handles.
const KindTransfer = "transfer"

const (
	// defaultPort is above 1024 (so no elevation), out of the way, and easy to
	// read out loud.
	defaultPort = 8722
	minPort     = 1024
	maxPort     = 65535

	// tokenBytes is the random path segment. 8 bytes is 16 hex characters,
	// short enough to read out and far beyond guessing on a local network.
	tokenBytes = 8

	// maxFiles is how many files one session may share. Past this the served
	// page becomes unusable and the tech should zip them first.
	maxFiles = 50

	// maxUploadBytes caps one upload, so a mistake cannot fill the tech's disk.
	maxUploadBytes = 4 << 30

	// shutdownGrace is how long Stop waits for a transfer in flight before
	// closing the listener anyway.
	shutdownGrace = 2 * time.Second

	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
)

// Address is one place another machine could reach this one.
type Address struct {
	IP      string `json:"ip"`
	Adapter string `json:"adapter"`
	// Primary is true for the address on the adapter this machine routes
	// through, which is the one to offer first.
	Primary bool `json:"primary"`
}

// Params is what the UI sends.
type Params struct {
	Files []string `json:"files"`
	// Port of zero means the default.
	Port int `json:"port"`
	// AllowUpload turns the receive form on. ReceiveDir is required when it is
	// true and ignored when it is false.
	AllowUpload bool   `json:"allowUpload"`
	ReceiveDir  string `json:"receiveDir"`
}

// Transfer is one request the server handled.
type Transfer struct {
	Time string `json:"time"`
	// Direction is "download" (the other machine took a file) or "upload".
	Direction string `json:"direction"`
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
	Peer      string `json:"peer"`
	// Status is "ok" or "failed".
	Status string `json:"status"`
	// Message is a sentence, empty when Status is "ok".
	Message string `json:"message"`
}

// Share is one file being offered, resolved once at start so the server never
// touches a path that came off the wire.
type Share struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Session is what the page needs in order to show the link and the QR code.
type Session struct {
	Token string `json:"token"`
	Port  int    `json:"port"`
	URL   string `json:"url"`
	Files int    `json:"files"`
}

// options is the validated form of Params, produced before anything starts.
type options struct {
	shares      []Share
	port        int
	allowUpload bool
	receiveDir  string
	// uploadLimit is a field rather than the constant so a test can drive the
	// over-limit path without writing four gigabytes.
	uploadLimit int64
}

// validate catches everything a user can get wrong, so bad input rejects the
// StartDrop call instead of starting a server that immediately fails.
func (p Params) validate() (options, error) {
	opts := options{
		port:        p.Port,
		allowUpload: p.AllowUpload,
		receiveDir:  strings.TrimSpace(p.ReceiveDir),
		uploadLimit: maxUploadBytes,
	}

	if len(p.Files) == 0 {
		return opts, core.Errorf(core.CodeInvalidInput, "Choose at least one file to share.")
	}
	if len(p.Files) > maxFiles {
		return opts, core.Errorf(core.CodeInvalidInput,
			"That is %d files. Share at most %d at a time, or put them in a zip first.",
			len(p.Files), maxFiles)
	}

	if opts.port == 0 {
		opts.port = defaultPort
	}
	if opts.port < minPort || opts.port > maxPort {
		return opts, core.Errorf(core.CodeInvalidInput,
			"The port must be between %d and %d. Below %d needs administrator rights, which CHIT never asks for.",
			minPort, maxPort, minPort)
	}

	opts.shares = make([]Share, 0, len(p.Files))
	for _, path := range p.Files {
		info, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return opts, core.Errorf(core.CodeNotFound,
				"There is no file at %s any more. Remove it from the list and try again.", path)
		case errors.Is(err, fs.ErrPermission):
			return opts, core.Errorf(core.CodePermission,
				"This computer will not let CHIT read %s, so it cannot be shared.", path)
		case err != nil:
			return opts, core.Errorf(core.CodeInternal,
				"%s could not be opened. It may be on a drive or network share that is no longer connected.", path)
		case info.IsDir():
			return opts, core.Errorf(core.CodeInvalidInput,
				"%s is a folder. Pick the files inside it: whole folders are not shared.", path)
		}

		f, err := os.Open(path)
		if err != nil {
			return opts, core.Errorf(core.CodePermission,
				"This computer will not let CHIT read %s, so it cannot be shared.", path)
		}
		f.Close()

		opts.shares = append(opts.shares, Share{
			Name: filepath.Base(path), Path: path, Bytes: info.Size(),
		})
	}

	if opts.allowUpload {
		if opts.receiveDir == "" {
			return opts, core.Errorf(core.CodeInvalidInput,
				"Choose the folder that files sent to you should land in.")
		}
		info, err := os.Stat(opts.receiveDir)
		if err != nil || !info.IsDir() {
			return opts, core.Errorf(core.CodeInvalidInput,
				"%s is not a folder, so files sent to you have nowhere to land.", opts.receiveDir)
		}
		if !writable(opts.receiveDir) {
			return opts, core.Errorf(core.CodePermission,
				"This computer will not let CHIT write into %s. Pick a folder you own, such as your Downloads folder.",
				opts.receiveDir)
		}
	}

	return opts, nil
}

// writable proves the receive folder can be written to now, rather than finding
// out when a transfer is half done.
func writable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".chit-drop-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

// newToken is the random path segment that scopes a session.
func newToken() (string, error) {
	var b [tokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", core.Errorf(core.CodeInternal,
			"CHIT could not create a private link for this session. Try again.")
	}
	return hex.EncodeToString(b[:]), nil
}

// tokenMatches compares in constant time, and a wrong token gets the same 404
// as a wrong path so the server never confirms a session exists.
func tokenMatches(want, got string) bool {
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// URLFor builds the address to read out or scan. An IPv6 literal is bracketed.
func URLFor(ip string, port int, token string) string {
	host := ip
	if strings.Contains(ip, ":") {
		host = "[" + ip + "]"
	}
	return "http://" + host + ":" + strconv.Itoa(port) + "/d/" + token
}

// Addresses lists the IPv4 addresses another machine on this network could
// reach this one on: unicast, not loopback, not link-local.
func Addresses(primaryIP string) ([]Address, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, core.Errorf(core.CodeInternal,
			"Could not read this machine's network adapters. Check that the network service is running.")
	}

	out := []Address{}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !usableForSharing(ip) {
				continue
			}
			out = append(out, Address{
				IP: ip.String(), Adapter: ifi.Name, Primary: ip.String() == primaryIP,
			})
		}
	}

	orderForSharing(out)
	return out, nil
}

// orderForSharing puts the address another machine is most likely to reach
// first, because the page builds its link from out[0] and most techs never
// open the picker.
//
// Three ranks: the address this machine actually routes through, then ordinary
// adapters, then the container and hypervisor adapters that look like a network
// and are not one. Reported from real use on 2026-07-29, on a machine running
// Docker: the page offered http://172.17.0.1:8722/... first, which nothing else
// on the network can reach, and the tech had no way to tell why. Before this
// the only ordering was "move the primary to the front", so when
// netinfo.PrimarySubnet could not name a primary (it returns an error rather
// than a guess) the order was whatever the operating system listed, and on that
// machine that was Docker.
//
// The sort is stable, so within a rank the operating system's own order is kept.
func orderForSharing(addrs []Address) {
	sort.SliceStable(addrs, func(i, j int) bool {
		return shareRank(addrs[i]) < shareRank(addrs[j])
	})
}

func shareRank(a Address) int {
	switch {
	case a.Primary:
		return 0
	case virtualAdapter(a.Adapter):
		return 2
	default:
		return 1
	}
}

// virtualPrefixes matches on the adapter NAME, never on the address range: a
// real office LAN can legitimately sit anywhere in 172.16/12, which is also
// where Docker's default bridge lives, so the range tells you nothing.
//
// "br-" carries the hyphen on purpose. That is Docker's own bridge naming
// (br-9f2c...); a plain "br0" is usually a real bridge doing real work on a
// server and must not be demoted.
var virtualPrefixes = []string{
	// Linux: docker, docker user networks, container veth pairs, libvirt,
	// VMware, VirtualBox.
	"docker", "br-", "veth", "virbr", "vmnet", "vboxnet",
	// Windows: Hyper-V and WSL switches, VMware, VirtualBox.
	"vethernet", "vmware", "virtualbox",
	// VPN and overlay tunnels, on every OS.
	"utun", "tun", "tap", "wg", "zt", "tailscale",
}

// virtualAdapter reports whether the name looks like a container, hypervisor or
// tunnel adapter rather than a network the other machine is on. Primary always
// outranks this, so a machine whose only route out is a Hyper-V "vEthernet"
// bridge still offers that first.
func virtualAdapter(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// usableForSharing rejects the addresses another machine could never reach this
// one on.
func usableForSharing(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

// sessions holds the live drop sessions so the page can ask for the URL of the
// job it just started. app.go is frozen and cannot carry a field, so the
// service is reached through a sync.Once accessor, exactly as totp does.
type Service struct {
	jobs *core.JobManager

	mu   sync.Mutex
	live map[string]Session
}

var (
	sharedOnce sync.Once
	shared     *Service
)

// Shared returns the one Service instance. A new one per bound call would
// forget every session as soon as it was created.
func Shared(jobs *core.JobManager) *Service {
	sharedOnce.Do(func() {
		shared = &Service{jobs: jobs, live: map[string]Session{}}
	})
	return shared
}

// SessionFor returns the session a job id is running, or an empty Session when
// that job is not a file drop or is no longer running.
func (s *Service) SessionFor(jobID string) Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live[jobID]
}

func (s *Service) remember(jobID string, session Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[jobID] = session
}

func (s *Service) forget(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, jobID)
}

// sessionNote explains a session in which nothing happened, so an empty log
// never reads as a broken tool.
func sessionNote(downloads, uploads int) string {
	if downloads == 0 && uploads == 0 {
		return "Nothing was transferred. If the other machine could not reach the address, check that both machines are on the same network and that this computer's firewall allowed CHIT through."
	}
	return ""
}

// summaryFor is the job:done summary. Its keys are a contract with the page and
// are checked nowhere by the compiler, in either language, so they are built in
// one named place that a test can pin.
func summaryFor(url string, port, files, downloads, uploads int, out, in int64) map[string]any {
	return map[string]any{
		"url":       url,
		"port":      port,
		"files":     files,
		"downloads": downloads,
		"uploads":   uploads,
		"bytesOut":  out,
		"bytesIn":   in,
		"note":      sessionNote(downloads, uploads),
	}
}

// sanitizeName reduces whatever a browser sent to a plain file name that cannot
// escape the receive folder or collide with a device name on Windows.
//
// The rules are applied on every operating system, not only Windows, because a
// file received on Linux is very often copied to a Windows machine next. They
// match internal/tools/renamer, which solved the same problem for a different
// shape of input; that package is deliberately not imported, because its
// functions are built around a rename plan rather than one name.
func sanitizeName(raw string) string {
	// Both separators are stripped whichever operating system sent the name.
	name := raw
	if at := strings.LastIndexAny(name, `/\`); at >= 0 {
		name = name[at+1:]
	}
	name = strings.TrimSpace(name)

	if name == "" || name == "." || name == ".." {
		return "upload"
	}

	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	name = b.String()

	// Windows silently strips a trailing dot or space, which breaks the round
	// trip: the file you saved is not the file you asked for.
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "upload"
	}

	stem := strings.TrimSuffix(name, filepath.Ext(name))
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}

	if len(name) > 200 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		name = name[:200-len(ext)] + ext
	}

	if name == "" {
		return "upload"
	}
	return name
}

// uniqueName appends " (1)", " (2)" and so on before the extension until the
// name is free, so an upload never silently replaces a file.
func uniqueName(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, fs.ErrNotExist) {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 10000; n++ {
		candidate := stem + " (" + strconv.Itoa(n) + ")" + ext
		if _, err := os.Stat(filepath.Join(dir, candidate)); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
	return stem + " (" + strconv.FormatInt(time.Now().UnixNano(), 36) + ")" + ext
}

// copyLimited streams src into dst, refusing anything past limit. It returns
// the number of bytes written and whether the limit was exceeded.
func copyLimited(dst io.Writer, src io.Reader, limit int64) (int64, bool, error) {
	written, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if written > limit {
		return written, true, err
	}
	return written, false, err
}
