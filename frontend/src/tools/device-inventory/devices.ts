// The saved device inventory: schema, validation, merge, import and export.
// Everything here is pure so the whole list can be tested without a DOM, with
// ids and clocks injected by the caller.
//
// Two imports reach into sibling tool folders, which the repo owner approved for
// Phase 4 rather than have this tool carry a second copy of either:
//   - canonicalMac and isIPv4 from wake-on-lan. These MUST be shared: this tool
//     imports Wake-on-LAN device lists and merges them by MAC address, so if the
//     two tools canonicalised differently the merge key would silently miss and
//     every device would import twice.
//   - parseCsv and detectDelimiter from csv-json-viewer. A second RFC 4180
//     parser is a fork, and forks drift.
import { canonicalMac, isIPv4 } from '../wake-on-lan/devices'
import { detectDelimiter, parseCsv } from '../csv-json-viewer/csv'

export const INVENTORY_NAMESPACE = 'device-inventory'
export const INVENTORY_DOC_VERSION = 1
export const INVENTORY_EXPORT_KIND = 'chit/device-inventory'

/** An inventory export is a few kilobytes. Anything much bigger is not one. */
export const MAX_IMPORT_BYTES = 5 * 1024 * 1024

export interface Device {
  id: string
  name: string
  site: string
  ip: string
  mac: string
  vendor: string
  kind: string
  notes: string
  addedAt: string
  updatedAt: string
}

export interface InventoryDoc {
  version: number
  devices: Device[]
}

/** What the add / edit form holds. Every field is text, as typed. */
export interface DeviceDraft {
  name: string
  site: string
  ip: string
  mac: string
  vendor: string
  kind: string
  notes: string
}

export type DeviceErrors = Partial<Record<keyof DeviceDraft, string>>

export const EMPTY_DRAFT: DeviceDraft = {
  name: '',
  site: '',
  ip: '',
  mac: '',
  vendor: '',
  kind: '',
  notes: '',
}

export const NAME_MESSAGE = 'Give the device a name so you can find it later.'
export const NAME_LONG_MESSAGE = 'Keep the name to 64 characters or fewer.'
export const IPV4_MESSAGE = 'That is not an IPv4 address. Leave it empty if you do not know it.'
export const MAC_MESSAGE =
  'That is not a MAC address. Enter 12 hex digits, for example AA:BB:CC:DD:EE:FF. Leave it empty if you do not know it.'
export const NOTES_LONG_MESSAGE = 'Keep the notes to 500 characters or fewer.'
export const SITE_LONG_MESSAGE = 'Keep the site name to 64 characters or fewer.'
export const FIELD_LONG_MESSAGE = 'Keep this to 64 characters or fewer.'

export const NOT_A_LIST_MESSAGE =
  'That file is not a device list. Import a JSON file exported from CHIT, or a CSV with a column named IP or MAC.'
export const NO_CSV_COLUMN_MESSAGE =
  "That CSV has no column named name, IP or MAC, so there is nothing to import. The IP Range Scanner's Export CSV file works here."
export const NEWER_VERSION_MESSAGE =
  'This inventory was written by a newer version of CHIT and could not be read. Update CHIT, or export the list again from the machine that wrote it.'
export const NEWER_FILE_MESSAGE =
  'That file was written by a newer version of CHIT and could not be read. Update CHIT, or export it again from the machine that wrote it.'
export const TOO_BIG_MESSAGE =
  'That file is larger than 5 MB. An inventory export is normally a few kilobytes, so this is probably not one.'
export const NO_STORE_MESSAGE =
  'Saving needs the desktop app. Anything you add here will be gone when you close the page.'
export const SAVE_FAILED_MESSAGE =
  'The inventory could not be saved. Check that the CHIT data folder is still there.'

// The same collator ResultsTable sorts with, so "PC 2" comes before "PC 10".
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** 'inv-' plus eight hex characters, never derived from the MAC: a machine can
 *  get a new network card and keep its place in the list. */
export function newDeviceId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(4))
  return 'inv-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export function validateDevice(
  draft: DeviceDraft,
): { ok: true; fields: Omit<Device, 'id' | 'addedAt' | 'updatedAt'> } | { ok: false; errors: DeviceErrors } {
  const errors: DeviceErrors = {}

  const name = draft.name.trim()
  if (name === '') errors.name = NAME_MESSAGE
  else if (name.length > 64) errors.name = NAME_LONG_MESSAGE

  const site = draft.site.trim()
  if (site.length > 64) errors.site = SITE_LONG_MESSAGE

  const ip = draft.ip.trim()
  if (ip !== '' && !isIPv4(ip)) errors.ip = IPV4_MESSAGE

  const rawMac = draft.mac.trim()
  const mac = rawMac === '' ? '' : canonicalMac(rawMac)
  if (mac === null) errors.mac = MAC_MESSAGE

  const vendor = draft.vendor.trim()
  if (vendor.length > 64) errors.vendor = FIELD_LONG_MESSAGE

  const kind = draft.kind.trim()
  if (kind.length > 64) errors.kind = FIELD_LONG_MESSAGE

  const notes = draft.notes.trim()
  if (notes.length > 500) errors.notes = NOTES_LONG_MESSAGE

  if (mac === null || Object.keys(errors).length > 0) return { ok: false, errors }
  return { ok: true, fields: { name, site, ip, mac, vendor, kind, notes } }
}

export function draftOf(device: Device): DeviceDraft {
  return {
    name: device.name,
    site: device.site,
    ip: device.ip,
    mac: device.mac,
    vendor: device.vendor,
    kind: device.kind,
    notes: device.notes,
  }
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

/** One entry from a file on disk, or null when it is not a device at all. */
function readDevice(raw: unknown): Device | null {
  if (typeof raw !== 'object' || raw === null) return null
  const entry = raw as Record<string, unknown>
  const device: Device = {
    id: text(entry.id),
    name: text(entry.name),
    site: text(entry.site),
    ip: text(entry.ip),
    mac: canonicalMac(text(entry.mac)) ?? '',
    vendor: text(entry.vendor),
    kind: text(entry.kind),
    notes: text(entry.notes),
    addedAt: text(entry.addedAt),
    updatedAt: text(entry.updatedAt),
  }
  if (device.name === '' && device.ip === '' && device.mac === '') return null
  return device
}

function readList(raw: unknown): Device[] | null {
  if (typeof raw !== 'object' || raw === null) return null
  const doc = raw as Record<string, unknown>
  if (!Array.isArray(doc.devices)) return null
  const devices: Device[] = []
  for (const entry of doc.devices) {
    const device = readDevice(entry)
    if (device !== null) devices.push(device)
  }
  return devices
}

/** Accepts anything read from disk and returns a valid document. A shape that
 *  cannot be read becomes an empty list rather than an error, so a corrupt file
 *  never blocks the tool. */
export function migrateDoc(raw: unknown): InventoryDoc {
  const empty: InventoryDoc = { version: INVENTORY_DOC_VERSION, devices: [] }
  if (typeof raw !== 'object' || raw === null) return empty
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > INVENTORY_DOC_VERSION) return empty
  return { version: INVENTORY_DOC_VERSION, devices: readList(doc) ?? [] }
}

/** The sentence to show about a document that could not be read, or ''. */
export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > INVENTORY_DOC_VERSION) {
    return NEWER_VERSION_MESSAGE
  }
  return ''
}

/** Fills in an id for a device saved by hand or by an older file. */
export function ensureIds(devices: Device[], makeId: () => string): Device[] {
  return devices.map((device) => (device.id === '' ? { ...device, id: makeId() } : device))
}

export function sortDevices(devices: Device[]): Device[] {
  return devices
    .slice()
    .sort((a, b) => collator.compare(a.site, b.site) || collator.compare(a.name, b.name))
}

/**
 * The key an import merges on. A MAC identifies a device best, an IP next, and
 * a name last, all inside one site so two offices can both have a "Reception
 * printer" without one overwriting the other.
 */
export function deviceKey(device: Pick<Device, 'site' | 'ip' | 'mac' | 'name'>): string {
  const site = device.site.trim().toLowerCase()
  if (device.mac !== '') return `${site}|mac:${device.mac}`
  if (device.ip !== '') return `${site}|ip:${device.ip}`
  return `${site}|name:${device.name.trim().toLowerCase()}`
}

export interface MergeReport {
  added: number
  updated: number
  unchanged: number
  skipped: number
  devices: Device[]
  error: string
}

const MERGEABLE = ['name', 'ip', 'mac', 'vendor', 'kind', 'notes'] as const

/**
 * Merges an imported list into the current one. An import never overwrites a
 * field a tech has already filled in: it fills blanks and adds what is new, so
 * running it twice, or against a colleague's file, is safe.
 *
 * Pure: ids and the clock come from the caller.
 */
export function mergeDevices(
  current: Device[],
  incoming: Device[],
  site: string,
  makeId: () => string,
  now: string,
): MergeReport {
  const chosenSite = site.trim()
  const merged = current.map((device) => ({ ...device }))
  const byKey = new Map(merged.map((device) => [deviceKey(device), device]))

  let added = 0
  let updated = 0
  let unchanged = 0
  let skipped = 0

  for (const raw of incoming) {
    const entry: Device = { ...raw, site: raw.site !== '' ? raw.site : chosenSite }
    if (entry.name === '' && entry.ip === '' && entry.mac === '') {
      skipped++
      continue
    }

    const key = deviceKey(entry)
    const existing = byKey.get(key)
    if (existing === undefined) {
      const device: Device = { ...entry, id: makeId(), addedAt: now, updatedAt: now }
      merged.push(device)
      byKey.set(key, device)
      added++
      continue
    }

    let changed = false
    for (const field of MERGEABLE) {
      if (existing[field] === '' && entry[field] !== '') {
        existing[field] = entry[field]
        changed = true
      }
    }
    if (changed) {
      existing.updatedAt = now
      updated++
    } else {
      unchanged++
    }
  }

  return { added, updated, unchanged, skipped, devices: sortDevices(merged), error: '' }
}

/** The devices matching the filter box and the site picker. */
export function filterDevices(devices: Device[], query: string, site: string): Device[] {
  const needle = query.trim().toLowerCase()
  return devices.filter((device) => {
    if (site !== '' && device.site !== site) return false
    if (needle === '') return true
    return [device.name, device.ip, device.mac, device.vendor, device.kind, device.notes]
      .join(' ')
      .toLowerCase()
      .includes(needle)
  })
}

/** The distinct sites in the list, sorted, for the site picker. */
export function siteNames(devices: Device[]): string[] {
  const sites = new Set<string>()
  for (const device of devices) {
    if (device.site !== '') sites.add(device.site)
  }
  return Array.from(sites).sort(collator.compare)
}

export interface InventoryExport {
  version: number
  kind: string
  exportedAt: string
  devices: Device[]
}

export function exportDoc(devices: Device[], now: string): InventoryExport {
  return {
    version: INVENTORY_DOC_VERSION,
    kind: INVENTORY_EXPORT_KIND,
    exportedAt: now,
    devices,
  }
}

export function slug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
}

/** chit-inventory-head-office-2026-07-26.json */
export function exportFileName(site: string, now: string): string {
  const name = slug(site)
  return `chit-inventory-${name === '' ? 'all-sites' : name}-${now.slice(0, 10)}.json`
}

/** The base name ResultsTable's CSV export uses. */
export function csvName(site: string): string {
  const name = slug(site)
  return `inventory-${name === '' ? 'all-sites' : name}`
}

// Column headers this tool understands, matched case-insensitively after the
// spaces and underscores are stripped. The IP Range Scanner's own CSV export
// uses IP, Status, Hostname, MAC, Vendor, Latency, Responded Via and Open Ports,
// so four of its columns land and the rest are ignored.
const CSV_COLUMNS: Record<keyof Omit<Device, 'id' | 'addedAt' | 'updatedAt'>, string[]> = {
  name: ['name', 'hostname', 'device', 'devicename', 'computername'],
  ip: ['ip', 'ipaddress', 'address', 'ipv4'],
  mac: ['mac', 'macaddress', 'hardwareaddress', 'physicaladdress'],
  vendor: ['vendor', 'manufacturer', 'make'],
  kind: ['kind', 'type', 'devicetype', 'category'],
  notes: ['notes', 'note', 'comment', 'comments', 'description'],
  site: ['site', 'location', 'office', 'building'],
}

function headerKey(header: string): string {
  // The trim is load-bearing and not just tidiness: ResultsTable writes its CSV
  // with a UTF-8 BOM so Excel reads it correctly, which makes the first header
  // arrive as "﻿IP". JavaScript's trim() counts U+FEFF as whitespace, so it
  // goes. Drop the trim and every scanner export silently loses its IP column.
  return header.trim().toLowerCase().replace(/[\s_]+/g, '')
}

export type ReadResult = { ok: true; devices: Device[] } | { ok: false; error: string }

/** Reads a CSV export into devices, mapping columns by their header name. */
export function readCsvDevices(text: string): ReadResult {
  const parsed = parseCsv(text, detectDelimiter(text), true)
  if (!parsed.ok) return { ok: false, error: parsed.error }

  const index: Partial<Record<keyof typeof CSV_COLUMNS, number>> = {}
  parsed.table.headers.forEach((header, at) => {
    const key = headerKey(header)
    for (const [field, names] of Object.entries(CSV_COLUMNS) as Array<
      [keyof typeof CSV_COLUMNS, string[]]
    >) {
      if (index[field] === undefined && names.includes(key)) index[field] = at
    }
  })

  if (index.name === undefined && index.ip === undefined && index.mac === undefined) {
    return { ok: false, error: NO_CSV_COLUMN_MESSAGE }
  }

  const cell = (row: string[], at: number | undefined): string =>
    at === undefined ? '' : (row[at] ?? '').trim()

  const devices: Device[] = []
  for (const row of parsed.table.rows) {
    const device = readDevice({
      name: cell(row, index.name),
      site: cell(row, index.site),
      ip: cell(row, index.ip),
      mac: cell(row, index.mac),
      vendor: cell(row, index.vendor),
      kind: cell(row, index.kind),
      notes: cell(row, index.notes),
    })
    if (device !== null) devices.push(device)
  }
  return { ok: true, devices }
}

/** Reads a CHIT inventory export, or a Wake-on-LAN device list, into devices. */
export function readJsonDevices(raw: unknown): ReadResult {
  if (typeof raw !== 'object' || raw === null) return { ok: false, error: NOT_A_LIST_MESSAGE }
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > INVENTORY_DOC_VERSION) {
    return { ok: false, error: NEWER_FILE_MESSAGE }
  }
  const devices = readList(doc)
  if (devices === null) return { ok: false, error: NOT_A_LIST_MESSAGE }
  return { ok: true, devices }
}

/** Reads whatever file the tech picked, JSON or CSV, deciding by its contents
 *  rather than its extension: a scanner export saved as .txt still works. */
export function readImport(body: string): ReadResult {
  if (body.length > MAX_IMPORT_BYTES) return { ok: false, error: TOO_BIG_MESSAGE }

  const looksJson = /^\s*[[{]/.test(body)
  if (looksJson) {
    let parsed: unknown
    try {
      parsed = JSON.parse(body)
    } catch {
      return { ok: false, error: NOT_A_LIST_MESSAGE }
    }
    return readJsonDevices(parsed)
  }
  if (body.trim() === '') return { ok: false, error: NOT_A_LIST_MESSAGE }
  return readCsvDevices(body)
}
