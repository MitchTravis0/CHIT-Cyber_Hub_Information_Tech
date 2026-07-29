//go:build windows

package diskbench

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fileFlagNoBuffering    = 0x20000000
	fileFlagSequentialScan = 0x08000000

	// sectorAlign is 4096, which every sector size in use divides: 512-byte
	// drives, 4K native drives and the 4K logical sectors advanced-format drives
	// report. FILE_FLAG_NO_BUFFERING needs the buffer address, the transfer
	// length and the file offset all to be multiples of it.
	sectorAlign = 4096
)

// openUncached opens path for reading with Windows's cache turned off. Some
// network redirectors refuse FILE_FLAG_NO_BUFFERING; that is not an error, it
// just means the read figure may be coming from memory, and the page says so.
func openUncached(path string, size int64) (*os.File, bool, error) {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateFile(wide,
		windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, fileFlagNoBuffering|fileFlagSequentialScan, 0)
	if err != nil {
		file, openErr := os.Open(path)
		return file, false, openErr
	}
	return os.NewFile(uintptr(handle), path), true, nil
}

// readBuffer returns a buffer whose first byte sits on a sector boundary, which
// is what FILE_FLAG_NO_BUFFERING requires. Go's collector does not move heap
// objects, so an address computed once stays valid; this is the same approach
// every Go library that opens a file unbuffered takes.
func readBuffer(n int) []byte {
	raw := make([]byte, n+sectorAlign)
	offset := int(uintptr(unsafe.Pointer(&raw[0])) % sectorAlign)
	if offset != 0 {
		offset = sectorAlign - offset
	}
	return raw[offset : offset+n]
}
