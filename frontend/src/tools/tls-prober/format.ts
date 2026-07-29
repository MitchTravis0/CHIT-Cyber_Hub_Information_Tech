import type { TlsAttempt } from './api'

/** The word shown in the Result column. */
export function resultLabel(attempt: TlsAttempt): string {
  if (!attempt.testable) return 'Not testable'
  return attempt.accepted ? 'Accepted' : 'Refused'
}

/** The StatusDot tone for a row. */
export function resultTone(attempt: TlsAttempt): 'ok' | 'danger' | 'idle' {
  if (!attempt.testable) return 'idle'
  return attempt.accepted ? 'ok' : 'danger'
}

/** Base name of the exported CSV, with the characters a file name cannot hold. */
export function csvNameFor(host: string, port: number): string {
  return `tls-${host.replace(/[.:[\]]/g, '-')}-${port}`
}

/** The whole report as text, one line per version, for pasting into a ticket. */
export function reportText(attempts: TlsAttempt[]): string {
  return attempts
    .map((a) => `${a.version.padEnd(8)} ${resultLabel(a).padEnd(13)} ${a.cipher}`.trimEnd())
    .join('\n')
}
