import type { Hop } from './api'

/** Last emit per hop number wins; result ascending by number. */
export function mergeHops(results: Hop[]): Hop[] {
  const byNumber = new Map<number, Hop>()
  for (const hop of results) byNumber.set(hop.number, hop)
  return Array.from(byNumber.values()).sort((a, b) => a.number - b.number)
}

/**
 * 0 to 100, this hop's average as a share of the slowest hop's average.
 * 0 when the hop has no times or no hop has any.
 */
export function barWidth(hop: Hop, hops: Hop[]): number {
  if (hop.timesMs.length === 0) return 0
  let slowest = 0
  for (const other of hops) {
    if (other.timesMs.length > 0 && other.avgMs > slowest) slowest = other.avgMs
  }
  if (slowest <= 0) return 0
  return Math.round((hop.avgMs / slowest) * 100)
}

/**
 * The number of the hop with the largest increase in average over the previous
 * hop that answered, or -1 when fewer than two hops answered. Silent hops are
 * skipped so a gap in the middle of a path does not invent a jump, and a tie
 * goes to the earlier hop.
 */
export function biggestJump(hops: Hop[]): number {
  const answering = hops.filter((hop) => hop.timesMs.length > 0)
  if (answering.length < 2) return -1

  let jump = -1
  let largest = -Infinity
  for (let i = 1; i < answering.length; i++) {
    const delta = answering[i].avgMs - answering[i - 1].avgMs
    if (delta > largest) {
      largest = delta
      jump = answering[i].number
    }
  }
  return jump
}

/** 'danger' when every probe was lost, 'warn' when some were, otherwise undefined. */
export function hopTone(hop: Hop): 'warn' | 'danger' | undefined {
  if (hop.lost === 0) return undefined
  return hop.timesMs.length === 0 ? 'danger' : 'warn'
}

/** The Note column text: the note, or "No reply" for a silent hop, or "". */
export function hopLabel(hop: Hop): string {
  if (hop.note !== '') return hop.note
  if (hop.timesMs.length === 0) return 'No reply'
  return ''
}
