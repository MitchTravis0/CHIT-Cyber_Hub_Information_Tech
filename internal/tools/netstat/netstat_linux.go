//go:build linux

package netstat

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procNetFiles are world-readable and need no privileges at all.
var procNetFiles = []struct {
	path     string
	protocol string
	v6       bool
	tcp      bool
}{
	{"/proc/net/tcp", "tcp", false, true},
	{"/proc/net/tcp6", "tcp6", true, true},
	{"/proc/net/udp", "udp", false, false},
	{"/proc/net/udp6", "udp6", true, false},
}

// collect reads /proc/net for the sockets and /proc/<pid>/fd for the names.
//
// The inode-to-pid map is built once, not once per socket: a machine with 400
// processes and 60 listeners would otherwise readlink tens of thousands of times.
func collect(r *Report) {
	owners := socketOwners()

	read := 0
	hidden := 0
	for _, source := range procNetFiles {
		data, err := os.ReadFile(source.path)
		if err != nil {
			continue
		}
		read++
		for _, line := range strings.Split(string(data), "\n") {
			row, ok := parseProcNetLine(line, source.v6)
			if !ok {
				continue
			}
			// UDP has no listen state, so every bound socket counts. TCP only
			// counts in the listen state.
			if source.tcp && row.State != stateListen {
				continue
			}
			if row.Port == 0 {
				continue
			}

			entry := Entry{
				Protocol: source.protocol,
				Address:  row.Address,
				Port:     row.Port,
				Source:   "/proc/net",
			}
			if owner, ok := owners[row.Inode]; ok {
				entry.PID = owner.pid
				entry.Process = owner.name
			} else {
				hidden++
			}
			r.Entries = append(r.Entries, entry)
		}
	}

	if read == 0 {
		r.Note = noteFor("linux", false, 0) + failProc
		return
	}
	r.Note = noteFor("linux", true, hidden)
}

type owner struct {
	pid  int
	name string
}

// socketOwners maps a socket inode to the process holding it. Another user's
// /proc/<pid>/fd is not readable, which is normal and is what leaves a row
// without a name rather than an error.
func socketOwners() map[uint64]owner {
	out := map[uint64]owner{}

	procs, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, entry := range procs {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		name := ""
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			inode, ok := socketInode(link)
			if !ok {
				continue
			}
			if name == "" {
				name = processName(entry.Name())
			}
			out[inode] = owner{pid: pid, name: name}
		}
	}
	return out
}

// socketInode reads the number out of a "socket:[12345]" symlink target.
func socketInode(link string) (uint64, bool) {
	rest, ok := strings.CutPrefix(link, "socket:[")
	if !ok {
		return 0, false
	}
	rest, ok = strings.CutSuffix(rest, "]")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func processName(pid string) string {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
