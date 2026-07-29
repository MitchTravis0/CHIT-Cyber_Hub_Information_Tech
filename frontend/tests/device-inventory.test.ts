import test from 'node:test'
import assert from 'node:assert/strict'
import {
  csvName,
  deviceKey,
  docWarning,
  exportDoc,
  exportFileName,
  filterDevices,
  mergeDevices,
  migrateDoc,
  readCsvDevices,
  readImport,
  readJsonDevices,
  siteNames,
  sortDevices,
  validateDevice,
  EMPTY_DRAFT,
  FIELD_LONG_MESSAGE,
  IPV4_MESSAGE,
  MAC_MESSAGE,
  NAME_LONG_MESSAGE,
  NAME_MESSAGE,
  NEWER_FILE_MESSAGE,
  NEWER_VERSION_MESSAGE,
  NOTES_LONG_MESSAGE,
  NOT_A_LIST_MESSAGE,
  NO_CSV_COLUMN_MESSAGE,
  SITE_LONG_MESSAGE,
  TOO_BIG_MESSAGE,
  type Device,
} from '../src/tools/device-inventory/devices.ts'

const NOW = '2026-07-26T10:00:00.000Z'

let counter = 0
const makeId = () => `inv-test-${++counter}`

function device(over: Partial<Device> = {}): Device {
  return {
    id: 'inv-1',
    name: 'Reception printer',
    site: 'Head Office',
    ip: '192.168.1.50',
    mac: 'AA:BB:CC:DD:EE:FF',
    vendor: 'Hewlett Packard',
    kind: 'Printer',
    notes: '',
    addedAt: NOW,
    updatedAt: NOW,
    ...over,
  }
}

test('validateDevice accepts a full device and canonicalises the MAC', () => {
  const result = validateDevice({
    name: '  Reception printer  ',
    site: ' Head Office ',
    ip: ' 192.168.1.50 ',
    mac: 'aabb.ccdd.eeff',
    vendor: 'HP',
    kind: 'Printer',
    notes: ' Static ',
  })
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.fields.name, 'Reception printer')
  assert.equal(result.fields.mac, 'AA:BB:CC:DD:EE:FF')
  assert.equal(result.fields.notes, 'Static')
})

test('validateDevice needs a name', () => {
  const result = validateDevice({ ...EMPTY_DRAFT })
  assert.equal(result.ok, false)
  if (result.ok) return
  assert.equal(result.errors.name, NAME_MESSAGE)
})

test('validateDevice checks the name length at the boundary', () => {
  const ok = validateDevice({ ...EMPTY_DRAFT, name: 'a'.repeat(64) })
  assert.equal(ok.ok, true)
  const tooLong = validateDevice({ ...EMPTY_DRAFT, name: 'a'.repeat(65) })
  assert.equal(tooLong.ok, false)
  if (tooLong.ok) return
  assert.equal(tooLong.errors.name, NAME_LONG_MESSAGE)
})

test('validateDevice leaves an empty IP and an empty MAC alone', () => {
  const result = validateDevice({ ...EMPTY_DRAFT, name: 'Switch' })
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.fields.ip, '')
  assert.equal(result.fields.mac, '')
})

test('validateDevice rejects a bad IP and a bad MAC with the exact wording', () => {
  const bad = validateDevice({ ...EMPTY_DRAFT, name: 'Switch', ip: '192.168.1.999', mac: 'nope' })
  assert.equal(bad.ok, false)
  if (bad.ok) return
  assert.equal(bad.errors.ip, IPV4_MESSAGE)
  assert.equal(bad.errors.mac, MAC_MESSAGE)
})

test('validateDevice checks the remaining length limits', () => {
  const bad = validateDevice({
    ...EMPTY_DRAFT,
    name: 'Switch',
    site: 'a'.repeat(65),
    vendor: 'b'.repeat(65),
    kind: 'c'.repeat(65),
    notes: 'd'.repeat(501),
  })
  assert.equal(bad.ok, false)
  if (bad.ok) return
  assert.equal(bad.errors.site, SITE_LONG_MESSAGE)
  assert.equal(bad.errors.vendor, FIELD_LONG_MESSAGE)
  assert.equal(bad.errors.kind, FIELD_LONG_MESSAGE)
  assert.equal(bad.errors.notes, NOTES_LONG_MESSAGE)

  const ok = validateDevice({
    ...EMPTY_DRAFT,
    name: 'Switch',
    site: 'a'.repeat(64),
    notes: 'd'.repeat(500),
  })
  assert.equal(ok.ok, true)
})

test('migrateDoc copes with everything a file on disk can be', () => {
  assert.deepEqual(migrateDoc(null).devices, [])
  assert.deepEqual(migrateDoc('a string').devices, [])
  assert.deepEqual(migrateDoc({}).devices, [])
  assert.deepEqual(migrateDoc({ version: 1 }).devices, [])
  assert.deepEqual(migrateDoc({ version: 1, devices: 'no' }).devices, [])
  assert.deepEqual(migrateDoc({ version: 2, devices: [device()] }).devices, [])
  assert.deepEqual(migrateDoc({ version: 1, devices: [null, 7, 'x'] }).devices, [])
  assert.equal(migrateDoc({ version: 1, devices: [device()] }).devices.length, 1)
  assert.equal(migrateDoc({ version: 1, devices: [device()] }).version, 1)
})

test('migrateDoc keeps only the fields the tool knows about', () => {
  const doc = migrateDoc({
    version: 1,
    devices: [{ name: 'Switch', ip: '10.0.0.1', somethingElse: 'ignored' }],
  })
  assert.equal(doc.devices.length, 1)
  assert.equal(Object.hasOwn(doc.devices[0], 'somethingElse'), false)
  assert.equal(doc.devices[0].vendor, '')
})

test('migrateDoc drops a row with no name, IP and no MAC', () => {
  const doc = migrateDoc({ version: 1, devices: [{ vendor: 'Cisco', notes: 'somewhere' }] })
  assert.deepEqual(doc.devices, [])
})

test('docWarning only fires for a document from the future', () => {
  assert.equal(docWarning({ version: 2 }), NEWER_VERSION_MESSAGE)
  assert.equal(docWarning({ version: 1 }), '')
  assert.equal(docWarning({}), '')
  assert.equal(docWarning(null), '')
})

test('deviceKey prefers MAC, then IP, then name, always inside the site', () => {
  assert.equal(
    deviceKey({ site: 'Head Office', mac: 'AA:BB:CC:DD:EE:FF', ip: '10.0.0.1', name: 'x' }),
    'head office|mac:AA:BB:CC:DD:EE:FF',
  )
  assert.equal(
    deviceKey({ site: 'Head Office', mac: '', ip: '10.0.0.1', name: 'x' }),
    'head office|ip:10.0.0.1',
  )
  assert.equal(deviceKey({ site: 'Head Office', mac: '', ip: '', name: 'Switch' }), 'head office|name:switch')
  assert.equal(
    deviceKey({ site: 'HEAD OFFICE', mac: '', ip: '', name: 'Switch' }),
    deviceKey({ site: 'head office', mac: '', ip: '', name: 'switch' }),
  )
  assert.notEqual(
    deviceKey({ site: 'Branch', mac: '', ip: '', name: 'Switch' }),
    deviceKey({ site: 'Head Office', mac: '', ip: '', name: 'Switch' }),
  )
})

test('mergeDevices adds a device it has not seen', () => {
  const report = mergeDevices([], [device({ id: '', addedAt: '', updatedAt: '' })], '', makeId, NOW)
  assert.equal(report.added, 1)
  assert.equal(report.updated, 0)
  assert.equal(report.devices.length, 1)
  assert.equal(report.devices[0].addedAt, NOW)
  assert.notEqual(report.devices[0].id, '')
})

test('mergeDevices fills a blank field but never overwrites a filled one', () => {
  const current = [device({ vendor: '', notes: 'Do not move' })]
  const incoming = [device({ id: '', vendor: 'Hewlett Packard', notes: 'From the scan' })]
  const report = mergeDevices(current, incoming, '', makeId, '2026-08-01T00:00:00.000Z')

  assert.equal(report.added, 0)
  assert.equal(report.updated, 1)
  assert.equal(report.devices.length, 1)
  assert.equal(report.devices[0].vendor, 'Hewlett Packard')
  assert.equal(report.devices[0].notes, 'Do not move')
  assert.equal(report.devices[0].updatedAt, '2026-08-01T00:00:00.000Z')
})

test('mergeDevices counts a device that had everything already as unchanged', () => {
  const current = [device()]
  const report = mergeDevices(current, [device({ id: '' })], '', makeId, NOW)
  assert.equal(report.added, 0)
  assert.equal(report.updated, 0)
  assert.equal(report.unchanged, 1)
  assert.equal(report.devices[0].updatedAt, NOW)
})

test('mergeDevices applies the chosen site only to rows without one', () => {
  const incoming = [
    device({ id: '', site: '', mac: '11:22:33:44:55:66' }),
    device({ id: '', site: 'Branch', mac: '22:33:44:55:66:77' }),
  ]
  const report = mergeDevices([], incoming, 'Head Office', makeId, NOW)
  const sites = report.devices.map((d) => d.site).sort()
  assert.deepEqual(sites, ['Branch', 'Head Office'])
})

test('mergeDevices skips a row with nothing to identify it', () => {
  const incoming = [device({ id: '', name: '', ip: '', mac: '', vendor: 'Cisco' })]
  const report = mergeDevices([], incoming, 'Head Office', makeId, NOW)
  assert.equal(report.skipped, 1)
  assert.equal(report.added, 0)
  assert.equal(report.devices.length, 0)
})

test('mergeDevices is pure: the same inputs twice give the same result', () => {
  const current = [device()]
  const incoming = [device({ id: '', mac: '11:22:33:44:55:66', name: 'Switch' })]
  const first = mergeDevices(current, incoming, '', () => 'inv-fixed', NOW)
  const second = mergeDevices(current, incoming, '', () => 'inv-fixed', NOW)
  assert.deepEqual(first.devices, second.devices)
  assert.equal(current[0].vendor, 'Hewlett Packard')
  assert.equal(current.length, 1)
})

test('importing the same file twice adds nothing the second time', () => {
  const incoming = [device({ id: '' })]
  const first = mergeDevices([], incoming, '', makeId, NOW)
  const second = mergeDevices(first.devices, incoming, '', makeId, NOW)
  assert.equal(second.added, 0)
  assert.equal(second.devices.length, 1)
})

test('readCsvDevices reads the IP Range Scanner export', () => {
  const csv = [
    'IP,Status,Hostname,MAC,Vendor,Latency,Responded Via,Open Ports',
    '192.168.1.1,Up,router.lan,AA:BB:CC:DD:EE:01,Ubiquiti,2 ms,icmp,"80, 443"',
    '192.168.1.50,Up,,AA:BB:CC:DD:EE:02,Hewlett Packard,9 ms,arp,',
    '192.168.1.51,Down,,,,,,',
  ].join('\n')

  const result = readCsvDevices(csv)
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.devices.length, 3)
  assert.equal(result.devices[0].name, 'router.lan')
  assert.equal(result.devices[0].ip, '192.168.1.1')
  assert.equal(result.devices[0].mac, 'AA:BB:CC:DD:EE:01')
  assert.equal(result.devices[0].vendor, 'Ubiquiti')
  // An address that never answered is still worth recording: that is what a
  // static-IP note is for.
  assert.equal(result.devices[2].ip, '192.168.1.51')
  assert.equal(result.devices[2].mac, '')
})

test('a CSV exactly as ResultsTable writes it imports, BOM and CRLF included', () => {
  // ResultsTable.downloadCsv prefixes a UTF-8 BOM (so Excel reads it as UTF-8)
  // and joins with CRLF. Without this case a change to headerKey could drop the
  // BOM handling and every scanner export would silently lose its IP column.
  const exported =
    '﻿' +
    [
      'IP,Status,Hostname,MAC,Vendor,Latency,Responded Via,Open Ports',
      '192.168.1.1,Up,router.lan,AA:BB:CC:DD:EE:01,Ubiquiti,2 ms,icmp,"80, 443"',
      '192.168.1.50,Up,,AA:BB:CC:DD:EE:02,Hewlett Packard,9 ms,arp,',
    ].join('\r\n') +
    '\r\n'

  const result = readImport(exported)
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.devices.length, 2)
  assert.equal(result.devices[0].ip, '192.168.1.1')
  assert.equal(result.devices[0].name, 'router.lan')
  assert.equal(result.devices[0].vendor, 'Ubiquiti')
  assert.equal(result.devices[1].mac, 'AA:BB:CC:DD:EE:02')
})

test('readCsvDevices matches headers whatever their case and spacing', () => {
  const csv = 'Device Name,ip_address,MAC Address,Location\nSwitch,10.0.0.2,AABBCCDDEE03,Branch'
  const result = readCsvDevices(csv)
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.devices[0].name, 'Switch')
  assert.equal(result.devices[0].ip, '10.0.0.2')
  assert.equal(result.devices[0].mac, 'AA:BB:CC:DD:EE:03')
  assert.equal(result.devices[0].site, 'Branch')
})

test('readCsvDevices refuses a CSV with no column it can use', () => {
  const result = readCsvDevices('Colour,Weight\nred,10')
  assert.equal(result.ok, false)
  if (result.ok) return
  assert.equal(result.error, NO_CSV_COLUMN_MESSAGE)
})

test('readCsvDevices handles a semicolon delimiter and an empty file', () => {
  const result = readCsvDevices('name;ip\nSwitch;10.0.0.2')
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.devices[0].ip, '10.0.0.2')

  const empty = readCsvDevices('')
  assert.equal(empty.ok, false)
})

test('readJsonDevices reads a CHIT inventory export and a Wake-on-LAN list', () => {
  const inventory = readJsonDevices(exportDoc([device()], NOW))
  assert.equal(inventory.ok, true)
  if (inventory.ok) assert.equal(inventory.devices.length, 1)

  const wol = readJsonDevices({
    version: 1,
    kind: 'chit/wol-devices',
    devices: [
      {
        id: 'wol-1',
        name: 'Reception PC',
        mac: 'AA:BB:CC:DD:EE:04',
        ip: '192.168.1.42',
        site: 'Head Office',
        broadcast: '',
        port: 9,
        notes: 'Wired NIC',
      },
    ],
  })
  assert.equal(wol.ok, true)
  if (!wol.ok) return
  assert.equal(wol.devices[0].name, 'Reception PC')
  assert.equal(wol.devices[0].mac, 'AA:BB:CC:DD:EE:04')
  assert.equal(wol.devices[0].notes, 'Wired NIC')
})

test('readJsonDevices refuses what is not a device list', () => {
  for (const raw of [null, 'text', {}, { devices: 'no' }, [1, 2]]) {
    const result = readJsonDevices(raw)
    assert.equal(result.ok, false)
    if (!result.ok) assert.equal(result.error, NOT_A_LIST_MESSAGE)
  }
})

test('readJsonDevices refuses a file from a newer CHIT', () => {
  const result = readJsonDevices({ version: 2, devices: [] })
  assert.equal(result.ok, false)
  if (result.ok) return
  assert.equal(result.error, NEWER_FILE_MESSAGE)
})

test('readImport decides JSON or CSV from the contents, not the name', () => {
  const json = readImport(JSON.stringify(exportDoc([device()], NOW)))
  assert.equal(json.ok, true)

  const csv = readImport('name,ip\nSwitch,10.0.0.2')
  assert.equal(csv.ok, true)
  if (csv.ok) assert.equal(csv.devices[0].name, 'Switch')

  const broken = readImport('{ not json')
  assert.equal(broken.ok, false)
  if (!broken.ok) assert.equal(broken.error, NOT_A_LIST_MESSAGE)

  const empty = readImport('   ')
  assert.equal(empty.ok, false)
})

test('readImport refuses a file over 5 MB', () => {
  const result = readImport('x'.repeat(5 * 1024 * 1024 + 1))
  assert.equal(result.ok, false)
  if (result.ok) return
  assert.equal(result.error, TOO_BIG_MESSAGE)
})

test('filterDevices searches every field a tech would type', () => {
  const list = [
    device({ id: '1', name: 'Reception printer', notes: 'Toner 26X' }),
    device({ id: '2', name: 'Switch', site: 'Branch', ip: '10.0.0.2', mac: '', vendor: 'Cisco', kind: 'Switch' }),
  ]
  assert.equal(filterDevices(list, 'toner', '').length, 1)
  assert.equal(filterDevices(list, 'CISCO', '').length, 1)
  assert.equal(filterDevices(list, '10.0.0.2', '').length, 1)
  assert.equal(filterDevices(list, 'aa:bb', '').length, 1)
  assert.equal(filterDevices(list, '', '').length, 2)
  assert.equal(filterDevices(list, '', 'Branch').length, 1)
  assert.equal(filterDevices(list, 'switch', 'Head Office').length, 0)
})

test('siteNames lists each site once, counting numbers properly', () => {
  const list = [
    device({ id: '1', site: 'Site 10' }),
    device({ id: '2', site: 'Site 2' }),
    device({ id: '3', site: 'Site 2' }),
    device({ id: '4', site: '' }),
  ]
  assert.deepEqual(siteNames(list), ['Site 2', 'Site 10'])
})

test('sortDevices groups the sites and orders names numerically', () => {
  const list = [
    device({ id: '1', site: 'Head Office', name: 'PC 10' }),
    device({ id: '2', site: 'Branch', name: 'PC 2' }),
    device({ id: '3', site: 'Head Office', name: 'PC 2' }),
  ]
  assert.deepEqual(
    sortDevices(list).map((d) => `${d.site}/${d.name}`),
    ['Branch/PC 2', 'Head Office/PC 2', 'Head Office/PC 10'],
  )
})

test('exportDoc names the format and keeps the devices as they are', () => {
  const doc = exportDoc([device()], NOW)
  assert.equal(doc.kind, 'chit/device-inventory')
  assert.equal(doc.version, 1)
  assert.equal(doc.exportedAt, NOW)
  assert.equal(doc.devices.length, 1)
})

test('exportFileName and csvName slug the site', () => {
  assert.equal(exportFileName('Head Office', NOW), 'chit-inventory-head-office-2026-07-26.json')
  assert.equal(exportFileName('', NOW), 'chit-inventory-all-sites-2026-07-26.json')
  assert.equal(csvName('Head Office'), 'inventory-head-office')
  assert.equal(csvName(''), 'inventory-all-sites')
})
