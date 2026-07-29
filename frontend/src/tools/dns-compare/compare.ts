import type { DnsAnswer, ServerOption } from './api'

/** The most resolvers one comparison may ask, matching dnscmp.MaxServers. */
export const MAX_SERVERS = 8

/** The word shown in the Agreement column. */
export function agreementLabel(row: DnsAnswer): string {
  // An errored row carries inStep true (it has not disagreed about anything),
  // so the status has to be checked first or it would read "Agrees".
  if (row.status === 'error') return 'No answer'
  return row.inStep ? 'Agrees' : 'Out of step'
}

/** The StatusDot tone for a row. */
export function agreementTone(row: DnsAnswer): 'ok' | 'warn' | 'idle' {
  if (row.status === 'error') return 'idle'
  return row.inStep ? 'ok' : 'warn'
}

/** Base name of the exported CSV. */
export function csvNameFor(name: string, type: string): string {
  return `dns-compare-${name.replace(/\./g, '-')}-${type}`
}

/**
 * The resolvers ticked when the page opens: the system resolver, this machine's
 * own servers, then the public ones, capped at what the backend will accept.
 * Quad9 is offered but left unticked so a typical machine stays under the cap.
 */
export function defaultTicks(options: ServerOption[]): string[] {
  const wanted = options.filter((o) => o.id !== '9.9.9.9').map((o) => o.id)
  return wanted.slice(0, MAX_SERVERS)
}

/** One line per resolver, for pasting into a ticket. */
export function comparisonText(answers: DnsAnswer[]): string {
  return answers
    .map((row) => {
      const values = row.status === 'error' ? 'no answer' : row.values.join(', ') || 'no record'
      return `${row.label}  ${values}  (${Math.round(row.queryMs)} ms)`
    })
    .join('\n')
}
