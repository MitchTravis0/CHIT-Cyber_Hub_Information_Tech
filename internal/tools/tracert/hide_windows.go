//go:build windows

package tracert

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a console window from flashing over the app when tracert
// runs.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
