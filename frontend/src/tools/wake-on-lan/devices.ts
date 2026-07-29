// The saved device list: schema, validation, merge and export. Everything here
// is pure so the whole list can be tested without a DOM, with ids and clocks
// injected by the caller.

export const WOL_NAMESPACE = 'wol-devices'
export const WOL_DOC_VERSION = 1
export const WOL_EXPORT_KIND = 'chit/wol-devices'

export const DEFAULT_PORT = 9

export interface WolDevice {
  id: string
  name: string
  mac: string
  ip: string
  site: string
  broadcast: string
  port: number
  notes: string
  addedAt: string
  lastWokenAt: string
}

export interface WolDoc {
  version: number
  devices: WolDevice[]
}

/** What the add / edit form holds. Every field is text, as typed. */
export interface DeviceDraft {
  name: string
  mac: string
  ip: string
  site: string
  broadcast: string
  port: string
  notes: string
}

export type DeviceErrors = Partial<Record<keyof DeviceDraft, string>>

export interface DeviceFields {
  name: string
  mac: string
  ip: string
  site: string
  broadcast: string
  port: number
  notes: string
}

export const EMPTY_DRAFT: DeviceDraft = {
  name: '',
  mac: '',
  ip: '',
  site: '',
  broadcast: '',
  port: String(DEFAULT_PORT),
  notes: '',
}

export const MAC_MESSAGE =
  'That is not a MAC address. Enter 12 hex digits, for example AA:BB:CC:DD:EE:FF.'
export const IPV4_MESSAGE = 'That is not an IPv4 address. Leave it empty if you do not know it.'
export const NAME_MESSAGE = 'Give the device a name so you can find it later.'
export const NOT_A_LIST_MESSAGE =
  'That file is not a CHIT device list. Export one from the Wake-on-LAN tool to see the format.'
export const NEWER_VERSION_MESSAGE =
  'This device list was written by a newer version of CHIT and could not be read. Update CHIT, or export the list again from the machine that wrote it.'

const MAC_PATTERN =
  /^[0-9a-fA-F]{2}([:-][0-9a-fA-F]{2}){5}$|^[0-9a-fA-F]{12}$|^[0-9a-fA-F]{4}(\.[0-9a-fA-F]{4}){2}$/
const IPV4_PATTERN = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/

// The same collator ResultsTable sorts with, so "PC 2" comes before "PC 10".
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** Canonical AA:BB:CC:DD:EE:FF, or null when the text is not a MAC address. */
export function canonicalMac(text: string): string | null {
  const trimmed = text.trim()
  if (!MAC_PATTERN.test(trimmed)) return null
  const bare = trimmed.replace(/[:.-]/g, '').toUpperCase()
  return (bare.match(/.{2}/g) ?? []).join(':')
}

export function isIPv4(text: string): boolean {
  const match = IPV4_PATTERN.exec(text.trim())
  if (match === null) return false
  return match.slice(1).every((part) => part.length <= 3 && Number(part) <= 255)
}

/** 'wol-' plus eight hex characters. Never derived from the MAC: a machine can
 *  get a new network card and keep its place in the list. */
export function newDeviceId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(4))
  return 'wol-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export function validateDevice(
  draft: DeviceDraft,
): { ok: true; fields: DeviceFields } | { ok: false; errors: DeviceErrors } {
  const errors: DeviceErrors = {}

  const name = draft.name.trim()
  if (name === '') errors.name = NAME_MESSAGE
  else if (name.length > 64) errors.name = 'Keep the name to 64 characters or fewer.'

  const mac = canonicalMac(draft.mac)
  if (mac === null) errors.mac = MAC_MESSAGE

  const ip = draft.ip.trim()
  if (ip !== '' && !isIPv4(ip)) errors.ip = IPV4_MESSAGE

  const site = draft.site.trim()
  if (site.length > 64) errors.site = 'Keep the site name to 64 characters or fewer.'

  const broadcast = draft.broadcast.trim()
  if (broadcast !== '' && !isIPv4(broadcast)) errors.broadcast = IPV4_MESSAGE

  const port = Number(draft.port.trim())
  if (!/^\d{1,5}$/.test(draft.port.trim()) || port < 1 || port > 65535) {
    errors.port = 'Ports run from 1 to 65535. Wake-on-LAN normally uses 9.'
  }

  const notes = draft.notes.trim()
  if (notes.length > 500) errors.notes = 'Keep the notes to 500 characters or fewer.'

  if (mac === null || Object.keys(errors).length > 0) return { ok: false, errors }
  return { ok: true, fields: { name, mac, ip, site, broadcast, port, notes } }
}

export function draftOf(device: WolDevice): DeviceDraft {
  return {
    name: device.name,
    mac: device.mac,
    ip: device.ip,
    site: device.site,
    broadcast: device.broadcast,
    port: String(device.port),
    notes: device.notes,
  }
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function portOf(value: unknown): number {
  if (typeof value !== 'number' || !Number.isInteger(value) || value < 1 || value > 65535) {
    return DEFAULT_PORT
  }
  return value
}

/** One entry from a file on disk. Null when it is not a device at all. */
function readDevice(raw: unknown): WolDevice | null {
  if (typeof raw !== 'object' || raw === null) return null
  const entry = raw as Record<string, unknown>
  const mac = canonicalMac(text(entry.mac))
  if (mac === null) return null
  return {
    id: text(entry.id),
    name: text(entry.name),
    mac,
    ip: text(entry.ip),
    site: text(entry.site),
    broadcast: text(entry.broadcast),
    port: portOf(entry.port),
    notes: text(entry.notes),
    addedAt: text(entry.addedAt),
    lastWokenAt: text(entry.lastWokenAt),
  }
}

/** Every device in raw plus how many entries were dropped, or null when raw is
 *  not a device document at all. */
function readList(raw: unknown): { devices: WolDevice[]; dropped: number } | null {
  if (typeof raw !== 'object' || raw === null) return null
  const doc = raw as Record<string, unknown>
  if (!Array.isArray(doc.devices)) return null
  const devices: WolDevice[] = []
  for (const entry of doc.devices) {
    const device = readDevice(entry)
    if (device !== null) devices.push(device)
  }
  return { devices, dropped: doc.devices.length - devices.length }
}

/** Accepts anything read from disk and returns a valid document. Unknown or
 *  missing shapes become an empty list rather than an error, so a corrupt file
 *  never blocks the tool. */
export function migrateDoc(raw: unknown): WolDoc {
  const empty: WolDoc = { version: WOL_DOC_VERSION, devices: [] }
  if (typeof raw !== 'object' || raw === null) return empty
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > WOL_DOC_VERSION) return empty
  return { version: WOL_DOC_VERSION, devices: readList(doc)?.devices ?? [] }
}

/** The sentence to show about a document that could not be read, or ''. */
export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > WOL_DOC_VERSION) return NEWER_VERSION_MESSAGE
  return ''
}

/** Fills in an id for a device saved by hand or by an older file, so every row
 *  has a stable key to edit and delete by. */
export function ensureIds(devices: WolDevice[], makeId: () => string): WolDevice[] {
  return devices.map((device) => (device.id === '' ? { ...device, id: makeId() } : device))
}

export function sortDevices(devices: WolDevice[]): WolDevice[] {
  return devices
    .slice()
    .sort((a, b) => collator.compare(a.site, b.site) || collator.compare(a.name, b.name))
}

export interface MergeReport {
  added: number
  updated: number
  skipped: number
  devices: WolDevice[]
  error: string
}

/** Merges an imported document into the current list. Pure: no clock, no
 *  randomness, so ids and timestamps for new entries come from the caller. */
export function mergeDevices(
  current: WolDevice[],
  incoming: unknown,
  makeId: () => string,
  now: string,
): MergeReport {
  const parsed = readList(incoming)
  if (parsed === null) {
    return { added: 0, updated: 0, skipped: 0, devices: current, error: NOT_A_LIST_MESSAGE }
  }

  // Anything the reader dropped had no usable MAC address.
  let skipped = parsed.dropped
  let added = 0
  let updated = 0

  const merged = current.map((device) => ({ ...device }))
  const byMac = new Map(merged.map((device) => [device.mac, device]))

  for (const entry of parsed.devices) {
    const existing = byMac.get(entry.mac)
    if (existing === undefined) {
      const device: WolDevice = { ...entry, id: makeId(), addedAt: now, lastWokenAt: '' }
      merged.push(device)
      byMac.set(device.mac, device)
      added++
      continue
    }
    // An import never overwrites something a tech already filled in, so running
    // it twice, or against a colleague's file, is safe.
    let changed = false
    for (const field of ['name', 'ip', 'site', 'broadcast', 'notes'] as const) {
      if (existing[field] === '' && entry[field] !== '') {
        existing[field] = entry[field]
        changed = true
      }
    }
    if (changed) updated++
    else skipped++
  }

  return { added, updated, skipped, devices: sortDevices(merged), error: '' }
}

export interface WolExport {
  version: number
  kind: string
  exportedAt: string
  devices: WolDevice[]
}

export function exportDoc(devices: WolDevice[], now: string): WolExport {
  return { version: WOL_DOC_VERSION, kind: WOL_EXPORT_KIND, exportedAt: now, devices }
}

/** chit-wol-head-office-2026-07-25.json */
export function exportFileName(site: string, now: string): string {
  const slug = site.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
  return `chit-wol-${slug === '' ? 'devices' : slug}-${now.slice(0, 10)}.json`
}

/** The devices matching the filter box and the site picker. */
export function filterDevices(devices: WolDevice[], query: string, site: string): WolDevice[] {
  const needle = query.trim().toLowerCase()
  return devices.filter((device) => {
    if (site !== '' && device.site !== site) return false
    if (needle === '') return true
    return [device.name, device.mac, device.ip, device.site]
      .join(' ')
      .toLowerCase()
      .includes(needle)
  })
}

/** The distinct sites in the list, sorted, for the site picker. */
export function siteNames(devices: WolDevice[]): string[] {
  const sites = new Set<string>()
  for (const device of devices) {
    if (device.site !== '') sites.add(device.site)
  }
  return Array.from(sites).sort(collator.compare)
}

/** The list split into site groups, in the order they should be rendered. */
export function groupBySite(devices: WolDevice[]): Array<{ site: string; devices: WolDevice[] }> {
  const groups: Array<{ site: string; devices: WolDevice[] }> = []
  for (const device of sortDevices(devices)) {
    const last = groups[groups.length - 1]
    if (last !== undefined && last.site === device.site) last.devices.push(device)
    else groups.push({ site: device.site, devices: [device] })
  }
  return groups
}
