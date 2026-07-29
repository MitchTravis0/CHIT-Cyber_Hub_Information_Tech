import { formatBytes, formatDuration } from '../../lib/format'
import type { DupFile, Group } from './api'

export interface Totals {
  groups: number
  /** Extra copies: the number of files a tech could actually delete. */
  copies: number
  waste: number
}

/** One row of the table view: one file, carrying its group's facts. */
export interface Row {
  group: number
  hash: string
  bytes: number
  count: number
  name: string
  path: string
  modified: string
}

/**
 * The backend emits each group once, but useJob keeps every item ever received,
 * so the page folds by hash before rendering. Biggest waste first, which is the
 * order a tech works down.
 */
export function mergeGroups(results: Group[]): Group[] {
  const byHash = new Map<string, Group>()
  for (const group of results) byHash.set(group.hash, group)
  return Array.from(byHash.values()).sort((a, b) => b.waste - a.waste || b.bytes - a.bytes)
}

export function totals(groups: Group[]): Totals {
  let copies = 0
  let waste = 0
  for (const group of groups) {
    copies += group.count - 1
    waste += group.waste
  }
  return { groups: groups.length, copies, waste }
}

/** The flattened table view: one row per file, group numbers 1-based. */
export function toRows(groups: Group[]): Row[] {
  const rows: Row[] = []
  groups.forEach((group, index) => {
    for (const file of group.files ?? []) {
      rows.push({
        group: index + 1,
        hash: group.hash,
        bytes: group.bytes,
        count: group.count,
        name: file.name,
        path: file.path,
        modified: file.modified,
      })
    }
  })
  return rows
}

function plural(n: number, singular: string, many: string): string {
  return `${n.toLocaleString()} ${n === 1 ? singular : many}`
}

export function summaryLine(t: Totals, scanned: number, ms: number, cancelled: boolean): string {
  const head =
    t.groups === 0
      ? 'No identical files found'
      : `${plural(t.groups, 'group', 'groups')} of identical files, ` +
        `${plural(t.copies, 'copy', 'copies')} you could delete, ${formatBytes(t.waste)} wasted`
  const tail = `. Looked at ${plural(scanned, 'file', 'files')} in ${formatDuration(ms)}`
  return head + tail + (cancelled ? ' (stopped early)' : '') + '.'
}

/** The exported file's base name, safe on every filesystem. */
export function csvBase(path: string): string {
  const parts = path.split(/[\\/]/).filter((part) => part !== '')
  const name = parts.length === 0 ? 'root' : parts[parts.length - 1]
  return 'duplicates-' + name.replace(/[^A-Za-z0-9-]/g, '-')
}

/** A local date-time for the Modified column, tolerating a bad value. */
export function modifiedLabel(file: DupFile): string {
  const at = new Date(file.modified)
  return Number.isNaN(at.getTime()) ? '' : at.toLocaleString()
}
