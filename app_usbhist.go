package main

import "chit/internal/tools/usbhist"

// USBDevices lists the USB devices connected to this machine now, and on
// Windows the ones the operating system remembers seeing before. Everything is
// read only: nothing is ejected, removed or changed.
func (a *App) USBDevices() (usbhist.Report, error) {
	return usbhist.List()
}
