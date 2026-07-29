// The vendor list, the URL builder, the serial guess and the lookup history.
// All pure, so it can be tested without a DOM or a browser.

export const WARRANTY_NAMESPACE = 'warranty-lookup'
export const WARRANTY_DOC_VERSION = 1

export const MAX_SERIAL = 64
export const MAX_VENDOR_NAME = 40
export const MAX_HISTORY = 20

export interface Vendor {
  id: string
  name: string
  url: string
  note: string
  /** False when the vendor's page will not take a serial in a link. */
  carriesSerial: boolean
}

export interface Lookup {
  id: string
  serial: string
  vendorId: string
  vendorName: string
  stamp: string
}

export interface WarrantyDoc {
  version: number
  vendors: Vendor[]
  history: Lookup[]
}

export const SERIAL_EMPTY_MESSAGE = 'Type the serial number or service tag first.'
export const SERIAL_LONG_MESSAGE =
  'That is longer than any serial number. Check you have not pasted the whole line from the sticker.'
export const VENDOR_NAME_MESSAGE = 'Give the vendor a name so you can pick it from the list.'
export const VENDOR_NAME_LONG_MESSAGE = 'Keep the vendor name to 40 characters or fewer.'
export const VENDOR_SCHEME_MESSAGE =
  'A vendor link has to start with https://. Anything else will not open in a browser.'
export const VENDOR_NO_PLACEHOLDER_MESSAGE =
  'That link has no {serial} in it, so the serial would not reach the vendor. Add {serial} where the number goes, or untick "This link carries the serial".'
export const NO_VENDORS_MESSAGE = 'No vendors. Press Restore the built-in list.'
export const NO_GUESS_MESSAGE =
  'CHIT could not tell which vendor this is from the serial. Pick the vendor yourself.'
export const GUESS_MESSAGE = 'Best guess from the shape of the serial. Change it if that is wrong.'
export const PASTE_TOAST =
  'Serial copied. Paste it into the box on the page that just opened.'
export const NO_APP_MESSAGE =
  'Opening a link needs the desktop app. The address is above, and Copy link puts it on your clipboard.'
export const NEWER_VERSION_MESSAGE =
  'This vendor list was written by a newer version of CHIT and could not be read. Update CHIT, or export it again from the machine that wrote it.'
export const NO_STORE_MESSAGE =
  'Saving needs the desktop app. Any vendor you add here will be gone when you close the page.'
export const SAVE_FAILED_MESSAGE =
  'The vendor list could not be saved. Check that the CHIT data folder is still there.'

/**
 * The vendors CHIT knows about. Every note is honest about what the link
 * actually does: the two that cannot take a serial say so, rather than the tool
 * pretending the page arrives filled in.
 *
 * The list is saved and editable on purpose. A vendor changing their address is
 * then a five second fix on site instead of a bug report.
 */
export function builtInVendors(): Vendor[] {
  return [
    {
      id: 'dell',
      name: 'Dell',
      url: 'https://www.dell.com/support/home/en-us/product-support/servicetag/{serial}/overview',
      note: 'Dell calls it a Service Tag: seven characters on a sticker or under System Information. The Express Service Code is the same tag as a number, and does not work in this link.',
      carriesSerial: true,
    },
    {
      id: 'lenovo',
      name: 'Lenovo',
      url: 'https://pcsupport.lenovo.com/us/en/warrantylookup?serialNumber={serial}',
      note: 'Lenovo serials are usually eight characters. On ThinkPads it is on the underside next to the machine type.',
      carriesSerial: true,
    },
    {
      id: 'apple',
      name: 'Apple',
      url: 'https://checkcoverage.apple.com/?sn={serial}',
      note: 'Apple wants the full serial from About This Mac, or Settings then General then About on an iPhone or iPad.',
      carriesSerial: true,
    },
    {
      id: 'hp',
      name: 'HP',
      url: 'https://support.hp.com/us-en/check-warranty',
      note: "HP's warranty page will not take a serial in the link, so CHIT opens the page and puts the serial on your clipboard. Paste it into the box on the page.",
      carriesSerial: false,
    },
    {
      id: 'microsoft',
      name: 'Microsoft Surface',
      url: 'https://account.microsoft.com/devices',
      note: 'Surface coverage is tied to a Microsoft account rather than a public serial lookup, so this opens the devices page. Sign in with the account the Surface is registered to.',
      carriesSerial: false,
    },
    {
      id: 'other',
      name: 'Other (search)',
      url: 'https://duckduckgo.com/?q={serial}+warranty+check',
      note: 'No vendor page is built in for this one, so CHIT searches for the serial and the word warranty. Add the vendor\'s real link in the Vendors and links panel below and it will be there next time.',
      carriesSerial: true,
    },
  ]
}

const ALPHANUMERIC = /^[0-9A-Z]+$/

/**
 * The vendor a serial's shape suggests, or '' when nothing is clear enough.
 *
 * Deliberately conservative: a confident wrong guess costs more than no guess,
 * because a tech does not check the thing they did not choose. The UI only ever
 * calls this a guess.
 */
export function guessVendor(serial: string): string {
  const text = serial.trim().toUpperCase()
  if (text === '' || !ALPHANUMERIC.test(text)) return ''

  const hasDigit = /[0-9]/.test(text)
  const hasLetter = /[A-Z]/.test(text)

  if (text.length === 7 && hasDigit && hasLetter) return 'dell'
  if (text.length === 8 && /^[A-Z]{2}/.test(text)) return 'lenovo'
  if (
    (text.length === 10 || text.length === 12) &&
    (text.startsWith('C02') ||
      text.startsWith('C07') ||
      text.startsWith('C1M') ||
      /^F[0-9]/.test(text) ||
      text.startsWith('DM'))
  ) {
    return 'apple'
  }
  if (text.length === 10 && /[A-Z]{2}$/.test(text)) return 'hp'
  return ''
}

/** Puts the serial into the vendor's template. A template with no placeholder
 *  is returned as it is, which is how the two lookup-page-only vendors work. */
export function buildUrl(template: string, serial: string): string {
  return template.replace('{serial}', encodeURIComponent(serial.trim()))
}

export function validateSerial(serial: string): string | undefined {
  const text = serial.trim()
  if (text === '') return SERIAL_EMPTY_MESSAGE
  if (text.length > MAX_SERIAL) return SERIAL_LONG_MESSAGE
  return undefined
}

export type VendorErrors = { name?: string; url?: string }

export function validateVendor(vendor: Pick<Vendor, 'name' | 'url' | 'carriesSerial'>): VendorErrors {
  const errors: VendorErrors = {}

  const name = vendor.name.trim()
  if (name === '') errors.name = VENDOR_NAME_MESSAGE
  else if (name.length > MAX_VENDOR_NAME) errors.name = VENDOR_NAME_LONG_MESSAGE

  const url = vendor.url.trim()
  if (!/^https?:\/\//i.test(url)) errors.url = VENDOR_SCHEME_MESSAGE
  else if (vendor.carriesSerial && !url.includes('{serial}')) {
    errors.url = VENDOR_NO_PLACEHOLDER_MESSAGE
  }

  return errors
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

export function stampOf(date: Date): string {
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    ` ${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

/** Newest first, one entry per serial and vendor, capped so the list stays a
 *  list of recent lookups rather than a log. */
export function addHistory(history: Lookup[], entry: Lookup): Lookup[] {
  const rest = history.filter(
    (item) => !(item.serial === entry.serial && item.vendorId === entry.vendorId),
  )
  return [entry, ...rest].slice(0, MAX_HISTORY)
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function readVendor(raw: unknown): Vendor | null {
  if (typeof raw !== 'object' || raw === null) return null
  const entry = raw as Record<string, unknown>
  const name = text(entry.name)
  const url = text(entry.url)
  if (name === '' || url === '') return null
  return {
    id: text(entry.id),
    name,
    url,
    note: text(entry.note),
    carriesSerial: entry.carriesSerial !== false,
  }
}

function readLookup(raw: unknown): Lookup | null {
  if (typeof raw !== 'object' || raw === null) return null
  const entry = raw as Record<string, unknown>
  const serial = text(entry.serial)
  if (serial === '') return null
  return {
    id: text(entry.id),
    serial,
    vendorId: text(entry.vendorId),
    vendorName: text(entry.vendorName),
    stamp: text(entry.stamp),
  }
}

/**
 * Reads the saved document. A saved vendor list is used exactly as it is,
 * including an empty one: a tech who deleted every vendor meant it, and
 * "Restore the built-in list" is how they come back.
 */
export function migrateDoc(raw: unknown): WarrantyDoc {
  const fresh: WarrantyDoc = {
    version: WARRANTY_DOC_VERSION,
    vendors: builtInVendors(),
    history: [],
  }
  if (typeof raw !== 'object' || raw === null) return fresh
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > WARRANTY_DOC_VERSION) return fresh

  let vendors = fresh.vendors
  if (Array.isArray(doc.vendors)) {
    vendors = []
    for (const entry of doc.vendors) {
      const vendor = readVendor(entry)
      if (vendor !== null) vendors.push(vendor)
    }
  }

  const history: Lookup[] = []
  if (Array.isArray(doc.history)) {
    for (const entry of doc.history) {
      const lookup = readLookup(entry)
      if (lookup !== null) history.push(lookup)
    }
  }

  return { version: WARRANTY_DOC_VERSION, vendors, history: history.slice(0, MAX_HISTORY) }
}

export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > WARRANTY_DOC_VERSION) {
    return NEWER_VERSION_MESSAGE
  }
  return ''
}

export function newVendorId(): string {
  return 'vendor-' + randomHex()
}

export function newLookupId(): string {
  return 'look-' + randomHex()
}

function randomHex(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(4))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}
