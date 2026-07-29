// Package core provides the shared job engine every long-running CHIT tool
// builds on: cancellable jobs, batched result streaming and a worker pool.
package core

// Wails event names. The frontend subscribes to these four names only and
// filters by the jobId carried in every payload.
const (
	EventProgress = "job:progress"
	EventResult   = "job:result"
	EventDone     = "job:done"
	EventError    = "job:error"
)

type Progress struct {
	JobID   string `json:"jobId"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Message string `json:"message"`
}

type ResultBatch struct {
	JobID string `json:"jobId"`
	Kind  string `json:"kind"`
	Items []any  `json:"items"`
}

type JobDone struct {
	JobID      string         `json:"jobId"`
	Cancelled  bool           `json:"cancelled"`
	DurationMS int64          `json:"durationMs"`
	Summary    map[string]any `json:"summary"`
}

type JobError struct {
	JobID   string `json:"jobId"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
