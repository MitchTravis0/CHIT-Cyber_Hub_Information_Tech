import type { Host } from './api'
import { numberToIP, type IPRange } from './range'

/** 0 not checked yet, 1 free, 2 in use. One byte per address keeps a /22 cheap. */
export const PENDING = 0
export const FREE = 1
export const USED = 2

export interface FreeBlock {
  start: string
  end: string
  count: number
}

export interface Tally {
  total: number
  scanned: number
  used: number
  free: number
  viaIcmp: number
  viaTcp: number
  viaArp: number
  withMac: number
}

/**
 * Collapses the streamed batches into one record per address. The ARP stage
 * re-emits a host to promote it or to fill in its MAC, so the last record for
 * an address wins.
 */
export function mergeHosts(results: Host[]): Map<string, Host> {
  const byIP = new Map<string, Host>()
  for (const host of results) byIP.set(host.ip, host)
  return byIP
}

export function cellStates(range: IPRange, byIP: Map<string, Host>): Uint8Array {
  const states = new Uint8Array(range.count)
  for (let i = 0; i < range.count; i++) {
    const host = byIP.get(numberToIP(range.start + i))
    if (host === undefined) continue
    states[i] = host.status === 'up' ? USED : FREE
  }
  return states
}

export function tally(range: IPRange, states: Uint8Array, byIP: Map<string, Host>): Tally {
  const counts: Tally = {
    total: range.count,
    scanned: 0,
    used: 0,
    free: 0,
    viaIcmp: 0,
    viaTcp: 0,
    viaArp: 0,
    withMac: 0,
  }
  for (const state of states) {
    if (state === PENDING) continue
    counts.scanned++
    if (state === USED) counts.used++
    else counts.free++
  }
  for (const host of byIP.values()) {
    if (host.status !== 'up') continue
    if (host.respondedVia === 'icmp') counts.viaIcmp++
    else if (host.respondedVia === 'tcp') counts.viaTcp++
    else if (host.respondedVia === 'arp') counts.viaArp++
    if (host.mac !== '') counts.withMac++
  }
  return counts
}

/**
 * The longest run of addresses nothing answered on, which is what a tech is
 * really after when picking a static IP. Addresses still being checked break a
 * run rather than joining it, so the answer never over-promises mid-scan.
 */
export function largestFreeBlock(range: IPRange, states: Uint8Array): FreeBlock | null {
  let bestStart = -1
  let bestLength = 0
  let runStart = -1

  for (let i = 0; i <= states.length; i++) {
    if (i < states.length && states[i] === FREE) {
      if (runStart === -1) runStart = i
      continue
    }
    if (runStart !== -1) {
      const length = i - runStart
      if (length > bestLength) {
        bestLength = length
        bestStart = runStart
      }
      runStart = -1
    }
  }

  if (bestLength === 0) return null
  return {
    start: numberToIP(range.start + bestStart),
    end: numberToIP(range.start + bestStart + bestLength - 1),
    count: bestLength,
  }
}

export function firstFreeAddress(range: IPRange, states: Uint8Array): string | null {
  for (let i = 0; i < states.length; i++) {
    if (states[i] === FREE) return numberToIP(range.start + i)
  }
  return null
}
