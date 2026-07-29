import type { Rung } from './api'

/** The StatusDot tone for a rung. */
export function statusTone(status: string): 'ok' | 'warn' | 'danger' | 'idle' {
  switch (status) {
    case 'ok':
      return 'ok'
    case 'warn':
      return 'warn'
    case 'fail':
      return 'danger'
    default:
      return 'idle'
  }
}

/** The words next to the dot. */
export function statusLabel(status: string): string {
  switch (status) {
    case 'ok':
      return 'Passed'
    case 'warn':
      return 'Worth checking'
    case 'fail':
      return 'Failed'
    default:
      return 'Not checked'
  }
}

/** The whole ladder as plain text, ready to paste into a ticket. */
export function ladderText(rungs: Rung[]): string {
  return rungs
    .map(
      (rung) =>
        `${rung.step}. ${rung.name.padEnd(18)} ${statusLabel(rung.status).toUpperCase().padEnd(15)} ${rung.detail}`,
    )
    .join('\n')
}
