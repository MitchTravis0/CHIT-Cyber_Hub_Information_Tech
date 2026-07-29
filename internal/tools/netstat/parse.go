package netstat

import (
	"encoding/hex"
	"net/netip"
	"strconv"
	"strings"
)

// procRow is one line of /proc/net/tcp and friends, already decoded.
type procRow struct {
	Address string
	Port    int
	State   string
	Inode   uint64
}

// stateListen is what /proc/net/tcp calls a socket in the listen state. UDP has
// no listen state, so every bound UDP socket counts.
const stateListen = "0A"

// parseProcNetLine decodes one line of /proc/net/tcp, tcp6, udp or udp6. The
// header line, a short line and anything with an unreadable address all return
// ok=false rather than a half-filled row.
func parseProcNetLine(line string, v6 bool) (procRow, bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return procRow{}, false
	}
	// The header's first column is "sl", not "0:".
	if !strings.HasSuffix(fields[0], ":") {
		return procRow{}, false
	}

	host, port, ok := splitHexAddress(fields[1], v6)
	if !ok {
		return procRow{}, false
	}

	inode, err := strconv.ParseUint(fields[9], 10, 64)
	if err != nil {
		return procRow{}, false
	}

	return procRow{Address: host, Port: port, State: fields[3], Inode: inode}, true
}

// splitHexAddress decodes the "HEXADDR:HEXPORT" form /proc/net uses.
func splitHexAddress(text string, v6 bool) (string, int, bool) {
	addr, portText, found := strings.Cut(text, ":")
	if !found {
		return "", 0, false
	}
	port, err := strconv.ParseUint(portText, 16, 16)
	if err != nil {
		return "", 0, false
	}
	var host string
	var ok bool
	if v6 {
		host, ok = parseHexAddress6(addr)
	} else {
		host, ok = parseHexAddress4(addr)
	}
	if !ok {
		return "", 0, false
	}
	return host, int(port), true
}

// parseHexAddress4 decodes the eight hex digits /proc/net/tcp writes for an IPv4
// address. They are one 32-bit word in the machine's own byte order, so on the
// little-endian machines CHIT runs on "0100007F" is 127.0.0.1 and not 1.0.0.127.
func parseHexAddress4(text string) (string, bool) {
	word, ok := hexWord(text)
	if !ok {
		return "", false
	}
	return netip.AddrFrom4([4]byte{word[3], word[2], word[1], word[0]}).String(), true
}

// parseHexAddress6 decodes the thirty-two hex digits /proc/net/tcp6 writes: four
// 32-bit words, each in the machine's own byte order, in network order between
// them.
func parseHexAddress6(text string) (string, bool) {
	if len(text) != 32 {
		return "", false
	}
	var out [16]byte
	for i := 0; i < 4; i++ {
		word, ok := hexWord(text[i*8 : i*8+8])
		if !ok {
			return "", false
		}
		out[i*4+0] = word[3]
		out[i*4+1] = word[2]
		out[i*4+2] = word[1]
		out[i*4+3] = word[0]
	}
	return netip.AddrFrom16(out).Unmap().String(), true
}

func hexWord(text string) ([4]byte, bool) {
	var out [4]byte
	if len(text) != 8 {
		return out, false
	}
	raw, err := hex.DecodeString(text)
	if err != nil {
		return out, false
	}
	copy(out[:], raw)
	return out, true
}

// ntohsPort converts the DWORD Windows returns for a local port. The port sits
// in the low two bytes in network byte order, which is the single most common
// mistake in the GetExtendedTcpTable API: a DWORD of 0x00005000 is port 80.
func ntohsPort(v uint32) int {
	return int((v&0xff)<<8 | (v>>8)&0xff)
}

// lsofFile is one socket out of lsof's field output.
type lsofFile struct {
	PID      int
	Command  string
	Protocol string
	Name     string
	State    string
}

// parseLsofFields reads lsof's -F field output, which is one field per line
// prefixed by a letter. It is used rather than lsof's columns because the
// columns are aligned for a person and shift with the longest command name.
func parseLsofFields(out string) []lsofFile {
	var files []lsofFile
	var pid int
	var command string
	var current *lsofFile

	flush := func() {
		if current != nil && current.Name != "" {
			files = append(files, *current)
		}
		current = nil
	}

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		tag, value := line[0], line[1:]
		switch tag {
		case 'p':
			flush()
			pid, _ = strconv.Atoi(value)
			command = ""
		case 'c':
			command = value
		case 'f':
			flush()
			current = &lsofFile{PID: pid, Command: command}
		case 'P':
			if current != nil {
				current.Protocol = strings.ToUpper(value)
			}
		case 'n':
			if current != nil {
				current.Name = value
			}
		case 'T':
			if current != nil && strings.HasPrefix(value, "ST=") {
				current.State = strings.TrimPrefix(value, "ST=")
			}
		}
	}
	flush()
	return files
}

// splitLsofName turns lsof's "*:22", "127.0.0.1:631" or "[::1]:631" into an
// address and a port. A name holding "->" is a connection rather than a
// listener and is refused.
func splitLsofName(name string, v6 bool) (string, int, bool) {
	if name == "" || strings.Contains(name, "->") {
		return "", 0, false
	}
	at := strings.LastIndex(name, ":")
	if at < 0 {
		return "", 0, false
	}
	host, portText := name[:at], name[at+1:]
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return "", 0, false
	}
	return normalizeWildcard(trimBrackets(host), v6), port, true
}

// parseNetstatLine reads one line of macOS "netstat -an". It is the fallback for
// a Mac with no lsof, and it has no process column at all.
func parseNetstatLine(line string) (Entry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return Entry{}, false
	}

	var protocol string
	v6 := strings.HasSuffix(fields[0], "6")
	switch {
	case strings.HasPrefix(fields[0], "tcp"):
		protocol = "tcp"
		// Only a listener, and macOS puts the state in the last column.
		if fields[len(fields)-1] != "LISTEN" {
			return Entry{}, false
		}
	case strings.HasPrefix(fields[0], "udp"):
		protocol = "udp"
	default:
		return Entry{}, false
	}
	if v6 {
		protocol += "6"
	}

	// macOS writes "host.port" with a dot, so the port is after the last one.
	local := fields[3]
	at := strings.LastIndex(local, ".")
	if at < 0 {
		return Entry{}, false
	}
	port, err := strconv.Atoi(local[at+1:])
	if err != nil || port <= 0 {
		return Entry{}, false
	}

	return Entry{
		Protocol: protocol,
		Address:  normalizeWildcard(local[:at], v6),
		Port:     port,
		Source:   "netstat",
	}, true
}

// normalizeWildcard turns the "*" and "" both tools use for "every address" into
// the address the reach classification understands.
func normalizeWildcard(host string, v6 bool) string {
	if host == "*" || host == "" {
		if v6 {
			return "::"
		}
		return "0.0.0.0"
	}
	return host
}
