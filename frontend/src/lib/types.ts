// Mirrors the Go payloads in internal/core. Keep both sides in step.

export const JOB_PROGRESS = 'job:progress'
export const JOB_RESULT = 'job:result'
export const JOB_DONE = 'job:done'
export const JOB_ERROR = 'job:error'

export interface Progress {
  jobId: string
  done: number
  total: number
  message: string
}

export interface ResultBatch<T = unknown> {
  jobId: string
  kind: string
  items: T[]
}

export interface JobDone {
  jobId: string
  cancelled: boolean
  durationMs: number
  summary: Record<string, unknown>
}

export interface JobError {
  jobId: string
  code: string
  message: string
}

// The Wails bindings are generated at build time, so useJob reaches the
// central CancelJob method through the runtime global instead of importing a
// generated file that may not exist yet.
declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          CancelJob?: (jobId: string) => Promise<void>
        }
      }
    }
  }
}
