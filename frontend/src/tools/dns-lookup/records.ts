import type { DnsRecord } from './api'

/** Display and sort order for the eight types. */
export const TYPE_ORDER = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'NS', 'SRV', 'PTR'] as const

function typeRank(type: string): number {
  const index = TYPE_ORDER.indexOf(type as (typeof TYPE_ORDER)[number])
  return index === -1 ? TYPE_ORDER.length : index
}

/**
 * Grouped by server (first seen first), then TYPE_ORDER, then value. Stable.
 * Servers keep the order the user ticked them in so the two answers a tech is
 * comparing sit next to each other.
 */
export function sortRecords(records: DnsRecord[]): DnsRecord[] {
  const serverRank = new Map<string, number>()
  for (const record of records) {
    if (!serverRank.has(record.server)) serverRank.set(record.server, serverRank.size)
  }

  return records
    .map((record, index) => ({ record, index }))
    .sort((a, b) => {
      const byServer =
        (serverRank.get(a.record.server) ?? 0) - (serverRank.get(b.record.server) ?? 0)
      if (byServer !== 0) return byServer
      const byType = typeRank(a.record.type) - typeRank(b.record.type)
      if (byType !== 0) return byType
      if (a.record.value !== b.record.value) return a.record.value < b.record.value ? -1 : 1
      return a.index - b.index
    })
    .map((entry) => entry.record)
}

/** Drops "empty" rows when showEmpty is false. "error" rows are always kept. */
export function visibleRecords(records: DnsRecord[], showEmpty: boolean): DnsRecord[] {
  if (showEmpty) return records
  return records.filter((record) => record.status !== 'empty')
}

/** How many rows carry a real answer. */
export function answerCount(records: DnsRecord[]): number {
  return records.filter((record) => record.status === 'ok').length
}
