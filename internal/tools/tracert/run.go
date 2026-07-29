package tracert

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	// resolveTimeout bounds the name lookup done before the tool starts.
	resolveTimeout = 5 * time.Second
	// maxRuntime stops a trace that a wedged path would otherwise leave
	// running for the rest of the session.
	maxRuntime = 5 * time.Minute
)

// toolCandidates lists where the system traceroute lives, best first.
// /usr/sbin is named explicitly because an app started from a desktop icon
// does not always inherit a login shell's PATH.
func toolCandidates(goos string) []string {
	switch goos {
	case "windows":
		return []string{filepath.Join(os.Getenv("SystemRoot"), "System32", "tracert.exe"), "tracert"}
	case "darwin":
		return []string{"/usr/sbin/traceroute", "traceroute"}
	}
	return []string{"traceroute", "/usr/sbin/traceroute", "/usr/bin/traceroute"}
}

// lookupTool finds the traceroute command that ships with this OS. CHIT never
// installs one and never needs rights of its own: the system binary already
// carries whatever privilege it needs.
func lookupTool(goos string) (string, error) {
	for _, candidate := range toolCandidates(goos) {
		if strings.ContainsAny(candidate, `/\`) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", toolMissing(goos)
}

func toolMissing(goos string) error {
	switch goos {
	case "windows":
		return core.Errorf(core.CodeNotFound,
			"This computer does not have the tracert command, which is unusual for Windows. CHIT cannot follow the path without it.")
	case "darwin":
		return core.Errorf(core.CodeNotFound,
			"This Mac does not have the traceroute command, so CHIT cannot follow the path.")
	}
	return core.Errorf(core.CodeNotFound,
		"This computer does not have the traceroute command installed, so CHIT cannot follow the path. On Ubuntu or Debian install it with: sudo apt install traceroute. On Fedora or RHEL: sudo dnf install traceroute. On Arch: sudo pacman -S traceroute.")
}

// buildArgs returns the arguments for the system tool, without the executable.
// goos is a parameter so every platform's command line can be tested from one
// machine.
func buildArgs(goos, ip string, s settings) []string {
	if goos == "windows" {
		args := []string{"-h", strconv.Itoa(s.MaxHops), "-w", strconv.Itoa(s.TimeoutMS)}
		if s.NoNames {
			args = append(args, "-d")
		}
		return append(args, ip)
	}
	// Windows -w is milliseconds, BSD and GNU traceroute take whole seconds.
	waitSeconds := max(1, (s.TimeoutMS+999)/1000)
	args := []string{
		"-m", strconv.Itoa(s.MaxHops),
		"-q", strconv.Itoa(s.Queries),
		"-w", strconv.Itoa(waitSeconds),
	}
	if s.NoNames {
		args = append(args, "-n")
	}
	return append(args, ip)
}

// runTool runs the system traceroute and hands every hop it prints to onHop as
// the line arrives, so the page fills in live.
func runTool(jc *core.JobContext, tool string, args []string, parse func(string) (Hop, bool), onHop func(Hop)) error {
	runCtx, cancel := context.WithTimeout(jc.Ctx(), maxRuntime)
	defer cancel()

	cmd := exec.CommandContext(runCtx, tool, args...)
	hideWindow(cmd)

	// Both streams share one pipe because some BSD traceroute builds put the
	// header line on stderr.
	pr, pw, err := os.Pipe()
	if err != nil {
		return core.Errorf(core.CodeInternal,
			"CHIT could not start the traceroute command on this computer.")
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return core.Errorf(core.CodeInternal,
			"CHIT could not start the traceroute command on this computer.")
	}
	pw.Close() // the parent's end, so the scanner sees EOF when the child exits

	found := 0
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		hop, ok := parse(scanner.Text())
		if !ok {
			continue
		}
		found++
		onHop(hop)
	}
	pr.Close()
	waitErr := cmd.Wait()

	if jc.Ctx().Err() != nil {
		return jc.Ctx().Err()
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return core.Errorf(core.CodeTimeout,
			"The trace took longer than 5 minutes and was stopped. Lower the maximum number of hops and try again.")
	}
	// tracert and traceroute both exit non-zero in ordinary situations, so a
	// failure only matters when nothing at all came back.
	if waitErr != nil && found == 0 {
		return core.Errorf(core.CodePermission,
			"The traceroute command stopped without following the path. This computer may not be allowed to send the packets it needs.")
	}
	return nil
}
