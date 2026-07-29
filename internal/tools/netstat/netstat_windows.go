//go:build windows

package netstat

import (
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// iphlpapi is reached through the lazy DLL loader because golang.org/x/sys does
// not wrap GetExtendedTcpTable. internal/tools/sysinfo does the same for
// kernel32, and neither changes go.mod.
var (
	iphlpapi              = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcp    = iphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdp    = iphlpapi.NewProc("GetExtendedUdpTable")
	errInsufficientBuffer = uintptr(122) // ERROR_INSUFFICIENT_BUFFER
)

const (
	afInet  = 2
	afInet6 = 23

	tcpTableOwnerPIDListener = 3
	udpTableOwnerPID         = 1
)

// The fixed-size rows iphlpapi returns after a DWORD count. Every field is a
// DWORD unless it is an address, and the ports are in network byte order in the
// low two bytes (see ntohsPort).
type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

type mibTCP6RowOwnerPID struct {
	LocalAddr     [16]byte
	LocalScopeID  uint32
	LocalPort     uint32
	RemoteAddr    [16]byte
	RemoteScopeID uint32
	RemotePort    uint32
	State         uint32
	OwningPID     uint32
}

type mibUDPRowOwnerPID struct {
	LocalAddr uint32
	LocalPort uint32
	OwningPID uint32
}

type mibUDP6RowOwnerPID struct {
	LocalAddr    [16]byte
	LocalScopeID uint32
	LocalPort    uint32
	OwningPID    uint32
}

// collect reads the four tables and names the owning process from one snapshot.
func collect(r *Report) {
	names := processNames()

	ok := false
	if rows, err := tcpTable(afInet); err == nil {
		ok = true
		for _, row := range rows {
			r.Entries = append(r.Entries, Entry{
				Protocol: "tcp",
				Address:  netip.AddrFrom4(*(*[4]byte)(unsafe.Pointer(&row.LocalAddr))).String(),
				Port:     ntohsPort(row.LocalPort),
				PID:      int(row.OwningPID),
				Process:  names[row.OwningPID],
				Source:   "GetExtendedTcpTable",
			})
		}
	}
	if rows, err := tcp6Table(); err == nil {
		ok = true
		for _, row := range rows {
			r.Entries = append(r.Entries, Entry{
				Protocol: "tcp6",
				Address:  netip.AddrFrom16(row.LocalAddr).Unmap().String(),
				Port:     ntohsPort(row.LocalPort),
				PID:      int(row.OwningPID),
				Process:  names[row.OwningPID],
				Source:   "GetExtendedTcpTable",
			})
		}
	}
	if rows, err := udpTable(); err == nil {
		ok = true
		for _, row := range rows {
			r.Entries = append(r.Entries, Entry{
				Protocol: "udp",
				Address:  netip.AddrFrom4(*(*[4]byte)(unsafe.Pointer(&row.LocalAddr))).String(),
				Port:     ntohsPort(row.LocalPort),
				PID:      int(row.OwningPID),
				Process:  names[row.OwningPID],
				Source:   "GetExtendedUdpTable",
			})
		}
	}
	if rows, err := udp6Table(); err == nil {
		ok = true
		for _, row := range rows {
			r.Entries = append(r.Entries, Entry{
				Protocol: "udp6",
				Address:  netip.AddrFrom16(row.LocalAddr).Unmap().String(),
				Port:     ntohsPort(row.LocalPort),
				PID:      int(row.OwningPID),
				Process:  names[row.OwningPID],
				Source:   "GetExtendedUdpTable",
			})
		}
	}

	if !ok {
		r.Note = noteFor("windows", false, 0) + failWindows
		return
	}
	r.Note = noteFor("windows", true, 0)
}

// extendedTable makes the documented two-call sequence: once with no buffer to
// learn the size, then again with a buffer of that size.
func extendedTable(proc *windows.LazyProc, family, class uint32) ([]byte, uint32, error) {
	var size uint32
	ret, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&size)), 0,
		uintptr(family), uintptr(class), 0)
	if ret != errInsufficientBuffer && ret != 0 {
		return nil, 0, windows.Errno(ret)
	}
	if size == 0 {
		return nil, 0, nil
	}

	buf := make([]byte, size)
	ret, _, _ = proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0,
		uintptr(family), uintptr(class), 0)
	if ret != 0 {
		return nil, 0, windows.Errno(ret)
	}

	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	return buf, count, nil
}

// rowsOf reads the fixed-size rows that follow the DWORD count. The offset is
// the size of one row, so the struct definitions above are load bearing.
func rowsOf[T any](buf []byte, count uint32) []T {
	if count == 0 || len(buf) == 0 {
		return nil
	}
	// The table follows the DWORD count. Every one of these row types aligns to
	// 4 bytes, the same as the count itself, so there is no padding between them.
	head := unsafe.Pointer(&buf[0])
	first := unsafe.Add(head, unsafe.Sizeof(count))
	return unsafe.Slice((*T)(first), int(count))
}

func tcpTable(family uint32) ([]mibTCPRowOwnerPID, error) {
	buf, count, err := extendedTable(procGetExtendedTcp, family, tcpTableOwnerPIDListener)
	if err != nil {
		return nil, err
	}
	return rowsOf[mibTCPRowOwnerPID](buf, count), nil
}

func tcp6Table() ([]mibTCP6RowOwnerPID, error) {
	buf, count, err := extendedTable(procGetExtendedTcp, afInet6, tcpTableOwnerPIDListener)
	if err != nil {
		return nil, err
	}
	return rowsOf[mibTCP6RowOwnerPID](buf, count), nil
}

func udpTable() ([]mibUDPRowOwnerPID, error) {
	buf, count, err := extendedTable(procGetExtendedUdp, afInet, udpTableOwnerPID)
	if err != nil {
		return nil, err
	}
	return rowsOf[mibUDPRowOwnerPID](buf, count), nil
}

func udp6Table() ([]mibUDP6RowOwnerPID, error) {
	buf, count, err := extendedTable(procGetExtendedUdp, afInet6, udpTableOwnerPID)
	if err != nil {
		return nil, err
	}
	return rowsOf[mibUDP6RowOwnerPID](buf, count), nil
}

// processNames walks one snapshot into a map, rather than opening a handle per
// socket. A PID that has exited by then leaves an empty name, which is normal.
func processNames() map[uint32]string {
	out := map[uint32]string{}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return out
	}
	for {
		out[entry.ProcessID] = windows.UTF16ToString(entry.ExeFile[:])
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			return out
		}
	}
}
