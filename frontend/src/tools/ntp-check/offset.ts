import type { NtpServer } from './api'

/**
 * describeOffset words a clock difference the way a tech would say it, always
 * signed so the direction is never in doubt. Under a second it stays in
 * milliseconds, because that is the range where the answer is "fine".
 */
export function describeOffset(ms: number): string {
  const sign = ms < 0 ? '-' : '+'
  const abs = Math.abs(ms)
  if (abs < 1000) return `${sign}${Math.round(abs)} ms`

  const total = Math.round(abs / 1000)
  const seconds = total % 60
  const minutes = Math.floor(total / 60) % 60
  const hours = Math.floor(total / 3600)
  if (hours > 0) return `${sign}${hours} h ${minutes} m ${seconds} s`
  if (minutes > 0) return `${sign}${minutes} m ${seconds} s`
  return `${sign}${seconds} s`
}

/** The word shown in the Result column for each row status. */
export function resultLabel(status: string): string {
  switch (status) {
    case 'ok':
      return 'Fine'
    case 'warn':
      return 'Drifting'
    case 'error':
      return 'Too far out'
    default:
      return 'No answer'
  }
}

/** The StatusDot tone for each row status. */
export function resultTone(status: string): 'ok' | 'warn' | 'danger' | 'idle' {
  switch (status) {
    case 'ok':
      return 'ok'
    case 'warn':
      return 'warn'
    case 'error':
      return 'danger'
    default:
      return 'idle'
  }
}

/** One line per server, for pasting into a ticket. */
export function summaryText(servers: NtpServer[], checkedAt: string): string {
  return servers
    .map((row) =>
      row.status === 'unreachable'
        ? `${row.server}: no answer (checked ${checkedAt})`
        : `${row.server}: this computer is ${describeOffset(row.offsetMs)} (checked ${checkedAt})`,
    )
    .join('\n')
}
