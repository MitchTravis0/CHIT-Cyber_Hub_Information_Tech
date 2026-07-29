import type { Device } from './api'

export type Filter = 'all' | 'connected' | 'storage' | 'seen'

const KIND_LABELS: Record<string, string> = {
  storage: 'Storage',
  input: 'Keyboard or mouse',
  audio: 'Audio',
  video: 'Camera',
  network: 'Network',
  printer: 'Printer',
  hub: 'Hub',
  other: 'Other',
}

export function kindLabel(kind: string): string {
  return KIND_LABELS[kind] ?? 'Other'
}

/**
 * The two ids together, as a tech would paste them into a search. Half an id
 * with a stray colon would be worse than nothing, so both must be present.
 */
export function vidPid(d: Device): string {
  if (d.vendorId === '' || d.productId === '') return ''
  return `${d.vendorId}:${d.productId}`
}

function plural(n: number, singular: string, many: string): string {
  return `${n.toLocaleString()} ${n === 1 ? singular : many}`
}

export function countLine(devices: Device[], history: boolean): string {
  const connected = devices.filter((d) => d.connected).length
  const remembered = devices.length - connected

  const head = connected === 0 ? 'Nothing is connected now' : `${plural(connected, 'device', 'devices')} connected now`
  if (!history) return `${head}.`
  return `${head}, ${remembered.toLocaleString()} seen before.`
}

export function filterDevices(devices: Device[], filter: Filter): Device[] {
  switch (filter) {
    case 'connected':
      return devices.filter((d) => d.connected)
    case 'storage':
      return devices.filter((d) => d.kind === 'storage')
    case 'seen':
      return devices.filter((d) => !d.connected)
    default:
      return devices
  }
}

const KIND_ORDER = ['storage', 'input', 'audio', 'video', 'network', 'printer', 'hub', 'other']

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** Connected first, then by kind, then by name with numbers as numbers. */
export function sortDevices(devices: Device[]): Device[] {
  return devices.slice().sort((a, b) => {
    if (a.connected !== b.connected) return a.connected ? -1 : 1
    const rank = (kind: string) => {
      const at = KIND_ORDER.indexOf(kind)
      return at === -1 ? KIND_ORDER.length : at
    }
    const byKind = rank(a.kind) - rank(b.kind)
    if (byKind !== 0) return byKind
    return collator.compare(a.name, b.name)
  })
}

/** A local date for the First seen column, tolerating a value it cannot read. */
export function firstSeenLabel(d: Device): string {
  if (d.firstSeen === '') return ''
  const at = new Date(d.firstSeen)
  return Number.isNaN(at.getTime()) ? '' : at.toLocaleDateString()
}
