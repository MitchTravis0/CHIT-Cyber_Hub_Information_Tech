import { useCallback, useEffect, useRef, useState } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { errorMessage } from './format'
import {
  JOB_DONE,
  JOB_ERROR,
  JOB_PROGRESS,
  JOB_RESULT,
  type JobDone,
  type JobError,
  type Progress,
  type ResultBatch,
} from './types'

export interface JobProgress {
  done: number
  total: number
  message: string
}

const NO_PROGRESS: JobProgress = { done: 0, total: 0, message: '' }

type JobEvent =
  | { type: 'progress'; payload: Progress }
  | { type: 'result'; payload: ResultBatch }
  | { type: 'done'; payload: JobDone }
  | { type: 'error'; payload: JobError }

/**
 * Drives one streaming backend job: subscribes to the four job events, keeps
 * only the ones carrying its own jobId, and unsubscribes on unmount.
 * Every streaming tool uses this instead of touching the events directly.
 *
 * `done` is the final job:done payload, which carries how long the job took,
 * whether it was cancelled, and whatever summary the tool put in it.
 */
export function useJob<T>() {
  const [running, setRunning] = useState(false)
  const [progress, setProgress] = useState<JobProgress>(NO_PROGRESS)
  const [results, setResults] = useState<T[]>([])
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState<JobDone | null>(null)

  const jobIdRef = useRef<string | null>(null)
  const startingRef = useRef(false)
  // Events can arrive before the StartX promise resolves, so they are held
  // until the jobId is known and then replayed.
  const queuedRef = useRef<JobEvent[]>([])

  const apply = useCallback((event: JobEvent) => {
    switch (event.type) {
      case 'progress':
        setProgress({
          done: event.payload.done,
          total: event.payload.total,
          message: event.payload.message,
        })
        break
      case 'result':
        setResults((prev) => prev.concat(event.payload.items as T[]))
        break
      case 'done':
        jobIdRef.current = null
        setDone(event.payload)
        setRunning(false)
        break
      case 'error':
        jobIdRef.current = null
        setError(event.payload.message)
        setRunning(false)
        break
    }
  }, [])

  useEffect(() => {
    const route = (event: JobEvent) => {
      if (jobIdRef.current === null) {
        if (startingRef.current) queuedRef.current.push(event)
        return
      }
      if (event.payload.jobId !== jobIdRef.current) return
      apply(event)
    }

    const unsubscribe = [
      EventsOn(JOB_PROGRESS, (payload: Progress) => route({ type: 'progress', payload })),
      EventsOn(JOB_RESULT, (payload: ResultBatch) => route({ type: 'result', payload })),
      EventsOn(JOB_DONE, (payload: JobDone) => route({ type: 'done', payload })),
      EventsOn(JOB_ERROR, (payload: JobError) => route({ type: 'error', payload })),
    ]
    return () => unsubscribe.forEach((off) => off())
  }, [apply])

  const start = useCallback(
    async (starter: () => Promise<string>) => {
      jobIdRef.current = null
      queuedRef.current = []
      startingRef.current = true
      setResults([])
      setError(null)
      setDone(null)
      setProgress(NO_PROGRESS)
      setRunning(true)

      try {
        const jobId = await starter()
        jobIdRef.current = jobId
        startingRef.current = false
        const queued = queuedRef.current.filter((event) => event.payload.jobId === jobId)
        queuedRef.current = []
        queued.forEach(apply)
      } catch (err) {
        startingRef.current = false
        queuedRef.current = []
        setError(errorMessage(err))
        setRunning(false)
      }
    },
    [apply],
  )

  const cancel = useCallback(() => {
    const jobId = jobIdRef.current
    if (jobId === null) return

    const stopped = () => {
      jobIdRef.current = null
      setRunning(false)
    }
    const cancelJob = window.go?.main?.App?.CancelJob
    if (!cancelJob) {
      stopped()
      return
    }
    // A job that finished between the click and the call rejects; the UI is
    // already in its final state either way.
    cancelJob(jobId).catch(stopped)
  }, [])

  const reset = useCallback(() => {
    jobIdRef.current = null
    startingRef.current = false
    queuedRef.current = []
    setResults([])
    setError(null)
    setDone(null)
    setProgress(NO_PROGRESS)
    setRunning(false)
  }, [])

  return { running, progress, results, error, done, start, cancel, reset }
}
