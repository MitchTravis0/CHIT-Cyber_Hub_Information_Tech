//go:build !windows

package tracert

import "os/exec"

func hideWindow(*exec.Cmd) {}
