//go:build darwin

package swlist

import (
	"context"
	"os/exec"
	"time"
)

// collect asks system_profiler for the applications. It is genuinely slow, ten
// to twenty seconds on a full machine, which is why it gets a thirty second
// budget and the page shows a spinner.
func collect(r *Report) {
	ctx, cancel := context.WithTimeout(context.Background(), profilerCommandTimeout*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "system_profiler", "-json", "SPApplicationsDataType").Output()
	if err != nil {
		r.Note = noteFor("darwin", r.Sources) + failProfiler
		return
	}

	programs := parseApplicationsJSON(out)
	if len(programs) > 0 {
		r.addSource(SourceApplications)
		r.Programs = append(r.Programs, programs...)
	}
	r.Note = noteFor("darwin", r.Sources)
}
