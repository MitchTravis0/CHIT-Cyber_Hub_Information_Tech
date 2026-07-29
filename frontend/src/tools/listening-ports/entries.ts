import type { Entry } from './api'

export type Filter = 'all' | 'tcp' | 'udp' | 'reachable'

/** The Protocol column. An unknown value is shown as it came, uppercased. */
export function protocolLabel(protocol: string): string {
  switch (protocol) {
    case 'tcp':
      return 'TCP'
    case 'tcp6':
      return 'TCP (IPv6)'
    case 'udp':
      return 'UDP'
    case 'udp6':
      return 'UDP (IPv6)'
  }
  return protocol.toUpperCase()
}

/**
 * The Reachable column. Anything unrecognised reads "One address": claiming
 * "Local only" for a value we do not understand would understate the exposure,
 * which is the dangerous direction.
 */
export function reachLabel(reach: string): string {
  switch (reach) {
    case 'everywhere':
      return 'Everywhere'
    case 'local':
      return 'Local only'
  }
  return 'One address'
}

/** Green only for a socket no other machine can reach. */
export function reachTone(reach: string): 'ok' | 'warn' {
  return reach === 'local' ? 'ok' : 'warn'
}

/** Whether another machine on the network could reach this socket. */
export function isReachable(entry: Entry): boolean {
  return entry.reach !== 'local'
}

export function countLine(entries: Entry[]): string {
  if (entries.length === 0) return 'Nothing is listening on this machine.'

  const tcp = entries.filter((e) => e.protocol.startsWith('tcp')).length
  const udp = entries.filter((e) => e.protocol.startsWith('udp')).length
  const reachable = entries.filter(isReachable).length

  const head = `${entries.length} listening: ${tcp} TCP, ${udp} UDP.`
  if (reachable === 0) return head
  const word = reachable === 1 ? 'is' : 'are'
  return `${head} ${reachable} ${word} reachable from other machines.`
}

export function filterEntries(entries: Entry[], filter: Filter): Entry[] {
  switch (filter) {
    case 'tcp':
      return entries.filter((e) => e.protocol.startsWith('tcp'))
    case 'udp':
      return entries.filter((e) => e.protocol.startsWith('udp'))
    case 'reachable':
      return entries.filter(isReachable)
  }
  return entries
}

const familyOrder: Record<string, number> = { tcp: 0, tcp6: 1, udp: 2, udp6: 3 }

/** By port, because a tech arrives knowing the number. */
export function sortEntries(entries: Entry[]): Entry[] {
  return entries.slice().sort((a, b) => {
    if (a.port !== b.port) return a.port - b.port
    const fa = familyOrder[a.protocol] ?? 4
    const fb = familyOrder[b.protocol] ?? 4
    if (fa !== fb) return fa - fb
    return a.address.localeCompare(b.address)
  })
}

export function entryId(entry: Entry): string {
  return `${entry.protocol}|${entry.address}|${entry.port}`
}
