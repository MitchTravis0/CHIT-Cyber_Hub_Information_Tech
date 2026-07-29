import { formatDuration } from '../../lib/format'
import type { Sample } from './api'

/** One rate, to one decimal place. Never renders a negative. */
export function rateText(mbps: number): string {
  if (!Number.isFinite(mbps) || mbps <= 0) return '0 MB/s'
  return `${mbps.toFixed(1)} MB/s`
}

/** The Phase column. */
export function phaseLabel(phase: string): string {
  switch (phase) {
    case 'write':
      return 'Write'
    case 'read':
      return 'Read'
  }
  return phase
}

/** How long one phase had been running when a sample was taken. */
export function secondsText(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0 s'
  return `${seconds.toFixed(1)} s`
}

/** The line under the two headline figures. */
export function runLine(sizeMb: number, ms: number): string {
  return `${sizeMb} MB written and read back in ${formatDuration(ms)}`
}

/** A stable key for one sample. */
export function sampleId(sample: Sample): string {
  return `${sample.phase}|${sample.bytes}`
}
