import test from 'node:test'
import assert from 'node:assert/strict'
import {
  addHistory,
  buildUrl,
  builtInVendors,
  docWarning,
  guessVendor,
  migrateDoc,
  stampOf,
  validateSerial,
  validateVendor,
  MAX_HISTORY,
  NEWER_VERSION_MESSAGE,
  SERIAL_EMPTY_MESSAGE,
  SERIAL_LONG_MESSAGE,
  VENDOR_NAME_LONG_MESSAGE,
  VENDOR_NAME_MESSAGE,
  VENDOR_NO_PLACEHOLDER_MESSAGE,
  VENDOR_SCHEME_MESSAGE,
  type Lookup,
} from '../src/tools/warranty-lookup/vendors.ts'

test('guessVendor recognises a Dell service tag', () => {
  assert.equal(guessVendor('7XKQ2H3'), 'dell')
  assert.equal(guessVendor('7xkq2h3'), 'dell')
  assert.equal(guessVendor('  7XKQ2H3  '), 'dell')
})

test('guessVendor will not call seven characters Dell without both a letter and a digit', () => {
  assert.equal(guessVendor('1234567'), '')
  assert.equal(guessVendor('ABCDEFG'), '')
})

test('guessVendor recognises Lenovo, Apple and HP shapes', () => {
  assert.equal(guessVendor('PF0ABCDE'), 'lenovo')
  assert.equal(guessVendor('C02X1234ABCD'), 'apple')
  assert.equal(guessVendor('F1234567890'.slice(0, 10)), 'apple')
  assert.equal(guessVendor('DM123456ABCD'), 'apple')
  assert.equal(guessVendor('5CD1234ABC'), 'hp')
})

test('guessVendor gives up rather than guessing wrong', () => {
  assert.equal(guessVendor(''), '')
  assert.equal(guessVendor('   '), '')
  assert.equal(guessVendor('7XKQ 2H3'), '')
  assert.equal(guessVendor('7XKQ-2H3'), '')
  assert.equal(guessVendor('ABC123'), '')
  assert.equal(guessVendor('ABC12345'), 'lenovo')
  assert.equal(guessVendor('1BC12345'), '')
  assert.equal(guessVendor('1234567890'), '')
})

test('buildUrl puts the serial in and encodes it', () => {
  const dell = builtInVendors()[0]
  assert.equal(
    buildUrl(dell.url, '7XKQ2H3'),
    'https://www.dell.com/support/home/en-us/product-support/servicetag/7XKQ2H3/overview',
  )
  assert.equal(buildUrl('https://x/?sn={serial}', 'a b'), 'https://x/?sn=a%20b')
  assert.equal(buildUrl('https://x/?sn={serial}', 'a/b'), 'https://x/?sn=a%2Fb')
  assert.equal(buildUrl('https://x/?sn={serial}', 'a#b'), 'https://x/?sn=a%23b')
  assert.equal(buildUrl('https://x/?sn={serial}', 'a&b'), 'https://x/?sn=a%26b')
})

test('buildUrl leaves a template with no placeholder alone', () => {
  assert.equal(
    buildUrl('https://support.hp.com/us-en/check-warranty', '7XKQ2H3'),
    'https://support.hp.com/us-en/check-warranty',
  )
})

test('buildUrl does not upper-case the serial, because some vendors care', () => {
  assert.equal(buildUrl('https://x/?sn={serial}', 'abcDEF'), 'https://x/?sn=abcDEF')
})

test('buildUrl trims the ends only', () => {
  assert.equal(buildUrl('https://x/?sn={serial}', '  7XKQ2H3  '), 'https://x/?sn=7XKQ2H3')
})

test('the built-in vendors are the six the spec names', () => {
  const vendors = builtInVendors()
  assert.equal(vendors.length, 6)
  assert.deepEqual(
    vendors.map((v) => v.id),
    ['dell', 'lenovo', 'apple', 'hp', 'microsoft', 'other'],
  )
  for (const vendor of vendors) {
    assert.deepEqual(validateVendor(vendor), {}, `${vendor.name} does not pass validation`)
    assert.notEqual(vendor.note, '')
    assert.match(vendor.url, /^https:\/\//)
  }
})

test('the vendors that carry a serial have the placeholder, and the two that do not say so', () => {
  const byId = new Map(builtInVendors().map((v) => [v.id, v]))
  for (const id of ['dell', 'lenovo', 'apple', 'other']) {
    assert.equal(byId.get(id)?.carriesSerial, true, id)
    assert.equal(byId.get(id)?.url.includes('{serial}'), true, id)
  }
  for (const id of ['hp', 'microsoft']) {
    assert.equal(byId.get(id)?.carriesSerial, false, id)
    assert.equal(byId.get(id)?.url.includes('{serial}'), false, id)
    assert.match(byId.get(id)?.note ?? '', /clipboard|Sign in/)
  }
})

test('validateSerial checks empty, whitespace and the length boundary', () => {
  assert.equal(validateSerial(''), SERIAL_EMPTY_MESSAGE)
  assert.equal(validateSerial('   '), SERIAL_EMPTY_MESSAGE)
  assert.equal(validateSerial('a'.repeat(64)), undefined)
  assert.equal(validateSerial('a'.repeat(65)), SERIAL_LONG_MESSAGE)
})

test('validateVendor needs a name and an http(s) link', () => {
  assert.equal(
    validateVendor({ name: '', url: 'https://x/{serial}', carriesSerial: true }).name,
    VENDOR_NAME_MESSAGE,
  )
  assert.equal(
    validateVendor({ name: 'a'.repeat(41), url: 'https://x/{serial}', carriesSerial: true }).name,
    VENDOR_NAME_LONG_MESSAGE,
  )
  assert.equal(
    validateVendor({ name: 'a'.repeat(40), url: 'https://x/{serial}', carriesSerial: true }).name,
    undefined,
  )
  assert.equal(
    validateVendor({ name: 'X', url: 'ftp://x/{serial}', carriesSerial: true }).url,
    VENDOR_SCHEME_MESSAGE,
  )
  assert.equal(
    validateVendor({ name: 'X', url: 'javascript:alert(1)', carriesSerial: false }).url,
    VENDOR_SCHEME_MESSAGE,
  )
  assert.equal(validateVendor({ name: 'X', url: 'http://x/{serial}', carriesSerial: true }).url, undefined)
})

test('validateVendor insists on the placeholder only when the link carries the serial', () => {
  assert.equal(
    validateVendor({ name: 'X', url: 'https://x/warranty', carriesSerial: true }).url,
    VENDOR_NO_PLACEHOLDER_MESSAGE,
  )
  assert.equal(
    validateVendor({ name: 'X', url: 'https://x/warranty', carriesSerial: false }).url,
    undefined,
  )
})

function lookup(over: Partial<Lookup> = {}): Lookup {
  return {
    id: 'look-1',
    serial: '7XKQ2H3',
    vendorId: 'dell',
    vendorName: 'Dell',
    stamp: '2026-07-26 09:14',
    ...over,
  }
}

test('addHistory puts the newest first', () => {
  const history = addHistory([lookup({ id: 'a' })], lookup({ id: 'b', serial: 'OTHER1A' }))
  assert.deepEqual(
    history.map((h) => h.id),
    ['b', 'a'],
  )
})

test('addHistory moves a repeat to the top instead of duplicating it', () => {
  const history = addHistory([lookup({ id: 'a' }), lookup({ id: 'b', serial: 'OTHER1A' })], lookup({ id: 'c' }))
  assert.equal(history.length, 2)
  assert.equal(history[0].id, 'c')
  assert.equal(history[1].id, 'b')
})

test('addHistory keeps the same serial under a different vendor as a separate entry', () => {
  const history = addHistory([lookup({ id: 'a' })], lookup({ id: 'b', vendorId: 'hp', vendorName: 'HP' }))
  assert.equal(history.length, 2)
})

test('addHistory caps the list at twenty', () => {
  let history: Lookup[] = []
  for (let i = 0; i < 25; i++) {
    history = addHistory(history, lookup({ id: `look-${i}`, serial: `SERIAL${i}` }))
  }
  assert.equal(history.length, MAX_HISTORY)
  assert.equal(history[0].id, 'look-24')
})

test('migrateDoc gives the built-in vendors when there is nothing saved', () => {
  assert.equal(migrateDoc(null).vendors.length, 6)
  assert.equal(migrateDoc({}).vendors.length, 6)
  assert.equal(migrateDoc({ version: 2, vendors: [] }).vendors.length, 6)
  assert.deepEqual(migrateDoc(null).history, [])
})

test('migrateDoc keeps a saved vendor list exactly, including an empty one', () => {
  const emptied = migrateDoc({ version: 1, vendors: [], history: [] })
  assert.deepEqual(emptied.vendors, [])

  const custom = migrateDoc({
    version: 1,
    vendors: [{ id: 'x', name: 'Acme', url: 'https://acme/{serial}' }],
    history: [],
  })
  assert.equal(custom.vendors.length, 1)
  // carriesSerial defaults to true when the file does not say.
  assert.equal(custom.vendors[0].carriesSerial, true)
})

test('migrateDoc drops a vendor with no name or no url, and a lookup with no serial', () => {
  const doc = migrateDoc({
    version: 1,
    vendors: [{ name: '', url: 'https://x' }, { name: 'X', url: '' }, null, { name: 'Y', url: 'https://y' }],
    history: [lookup(), { serial: '' }, null, 7],
  })
  assert.equal(doc.vendors.length, 1)
  assert.equal(doc.history.length, 1)
})

test('migrateDoc caps a saved history that grew too long', () => {
  const history = Array.from({ length: 40 }, (_, i) => lookup({ id: `l${i}`, serial: `S${i}` }))
  assert.equal(migrateDoc({ version: 1, vendors: [], history }).history.length, MAX_HISTORY)
})

test('migrateDoc tolerates vendors or history not being arrays', () => {
  const doc = migrateDoc({ version: 1, vendors: 'no', history: 'no' })
  assert.equal(doc.vendors.length, 6)
  assert.deepEqual(doc.history, [])
})

test('docWarning only fires for a document from the future', () => {
  assert.equal(docWarning({ version: 2 }), NEWER_VERSION_MESSAGE)
  assert.equal(docWarning({ version: 1 }), '')
  assert.equal(docWarning(null), '')
})

test('stampOf writes the local date and time, zero padded', () => {
  assert.equal(stampOf(new Date(2026, 6, 26, 9, 4)), '2026-07-26 09:04')
})
