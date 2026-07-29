import type { Result } from './api'

export interface Tally {
  total: number
  open: number
  closed: number
  filtered: number
}

/**
 * Collapses the streamed batches into one record per port. The backend emits a
 * port once, so the last record simply wins and the tallies below cannot
 * double-count a re-delivered batch.
 */
export function mergePorts(results: Result[]): Map<number, Result> {
  const byPort = new Map<number, Result>()
  for (const row of results) byPort.set(row.port, row)
  return byPort
}

/** Counts by state over the merged map. */
export function tally(byPort: Map<number, Result>): Tally {
  const counts: Tally = { total: 0, open: 0, closed: 0, filtered: 0 }
  for (const row of byPort.values()) {
    counts.total++
    if (row.state === 'open') counts.open++
    else if (row.state === 'closed') counts.closed++
    else counts.filtered++
  }
  return counts
}

/** Sorted ascending by port; open only unless showAll. */
export function visibleRows(byPort: Map<number, Result>, showAll: boolean): Result[] {
  const rows = Array.from(byPort.values()).filter((row) => showAll || row.state === 'open')
  return rows.sort((a, b) => a.port - b.port)
}
