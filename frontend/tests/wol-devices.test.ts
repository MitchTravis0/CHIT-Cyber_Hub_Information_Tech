import test from 'node:test'
import assert from 'node:assert/strict'
import {
  canonicalMac,
  docWarning,
  ensureIds,
  exportDoc,
  exportFileName,
  filterDevices,
  mergeDevices,
  migrateDoc,
  newDeviceId,
  sortDevices,
  validateDevice,
  IPV4_MESSAGE,
  MAC_MESSAGE,
  NAME_MESSAGE,
  NEWER_VERSION_MESSAGE,
  NOT_A_LIST_MESSAGE,
  type WolDevice,
} from '../src/tools/wake-on-lan/devices.ts'

const DRAFT = {
  name: 'Reception PC',
  mac: 'AA:BB:CC:DD:EE:FF',
  ip: '',
  site: '',
  broadcast: '',
  port: '9',
  notes: '',
}

function device(over: Partial<WolDevice>): WolDevice {
  return {
    id: 'wol-00000001',
    name: '',
    mac: 'AA:BB:CC:DD:EE:FF',
    ip: '',
    site: '',
    broadcast: '',
    port: 9,
    notes: '',
    addedAt: '2026-07-01T00:00:00.000Z',
    lastWokenAt: '',
    ...over,
  }
}

function idMaker(): () => string {
  let n = 0
  return () => `wol-test${++n}`
}

test('canonicalMac accepts every form a tech will paste', () => {
  for (const form of ['aabbccddeeff', 'aa-bb-cc-dd-ee-ff', 'AABB.CCDD.EEFF', 'AA:BB:CC:DD:EE:FF']) {
    assert.equal(canonicalMac(form), 'AA:BB:CC:DD:EE:FF')
  }
  assert.equal(canonicalMac('  aa:bb:cc:dd:ee:ff  '), 'AA:BB:CC:DD:EE:FF')
})

test('canonicalMac rejects anything that is not 48 bits', () => {
  for (const form of ['AA:BB:CC:DD:EE', 'hello', '', 'AA:BB:CC:DD:EE:FF:11', 'GG:BB:CC:DD:EE:FF']) {
    assert.equal(canonicalMac(form), null)
  }
})

test('validateDevice accepts a filled in device and canonicalises the MAC', () => {
  const result = validateDevice({ ...DRAFT, mac: 'aabbccddeeff', ip: ' 192.168.1.42 ', port: '7' })
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.fields.mac, 'AA:BB:CC:DD:EE:FF')
  assert.equal(result.fields.ip, '192.168.1.42')
  assert.equal(result.fields.port, 7)
})

test('validateDevice reports one message per broken rule', () => {
  const cases: Array<{ name: string; draft: typeof DRAFT; field: string; message: string }> = [
    { name: 'empty name', draft: { ...DRAFT, name: '   ' }, field: 'name', message: NAME_MESSAGE },
    {
      name: '65 character name',
      draft: { ...DRAFT, name: 'x'.repeat(65) },
      field: 'name',
      message: 'Keep the name to 64 characters or fewer.',
    },
    { name: 'bad mac', draft: { ...DRAFT, mac: 'AA:BB:CC:DD:EE' }, field: 'mac', message: MAC_MESSAGE },
    { name: 'bad ip', draft: { ...DRAFT, ip: '192.168.1.999' }, field: 'ip', message: IPV4_MESSAGE },
    {
      name: 'bad broadcast',
      draft: { ...DRAFT, broadcast: 'everyone' },
      field: 'broadcast',
      message: IPV4_MESSAGE,
    },
    {
      name: '65 character site',
      draft: { ...DRAFT, site: 'y'.repeat(65) },
      field: 'site',
      message: 'Keep the site name to 64 characters or fewer.',
    },
    {
      name: 'port 0',
      draft: { ...DRAFT, port: '0' },
      field: 'port',
      message: 'Ports run from 1 to 65535. Wake-on-LAN normally uses 9.',
    },
    {
      name: 'port 65536',
      draft: { ...DRAFT, port: '65536' },
      field: 'port',
      message: 'Ports run from 1 to 65535. Wake-on-LAN normally uses 9.',
    },
    {
      name: 'port not a number',
      draft: { ...DRAFT, port: 'nine' },
      field: 'port',
      message: 'Ports run from 1 to 65535. Wake-on-LAN normally uses 9.',
    },
    {
      name: '501 character notes',
      draft: { ...DRAFT, notes: 'n'.repeat(501) },
      field: 'notes',
      message: 'Keep the notes to 500 characters or fewer.',
    },
  ]

  for (const item of cases) {
    const result = validateDevice(item.draft)
    assert.equal(result.ok, false, item.name)
    if (result.ok) continue
    assert.equal((result.errors as Record<string, string>)[item.field], item.message, item.name)
  }
})

test('validateDevice takes the boundary values', () => {
  const ok = validateDevice({
    ...DRAFT,
    name: 'x'.repeat(64),
    site: 'y'.repeat(64),
    notes: 'n'.repeat(500),
    port: '65535',
  })
  assert.equal(ok.ok, true)
  assert.equal(validateDevice({ ...DRAFT, port: '1' }).ok, true)
})

test('migrateDoc turns anything unreadable into an empty list', () => {
  for (const raw of [null, undefined, 'text', 42, {}, { version: '1' }, { version: 1 }]) {
    assert.deepEqual(migrateDoc(raw), { version: 1, devices: [] })
  }
  assert.deepEqual(migrateDoc({ version: 1, devices: 'x' }), { version: 1, devices: [] })
  assert.deepEqual(migrateDoc({ version: 2, devices: [{ mac: 'AABBCCDDEEFF' }] }), {
    version: 1,
    devices: [],
  })
})

test('migrateDoc keeps good entries, drops entries with no MAC, fills the gaps', () => {
  const doc = migrateDoc({
    version: 1,
    devices: [
      { id: 'wol-1', name: 'Reception PC', mac: 'aa-bb-cc-dd-ee-ff' },
      { name: 'No MAC here' },
      'not an object',
      { name: 'Bad MAC', mac: 'nope' },
    ],
  })
  assert.equal(doc.version, 1)
  assert.equal(doc.devices.length, 1)
  assert.deepEqual(doc.devices[0], {
    id: 'wol-1',
    name: 'Reception PC',
    mac: 'AA:BB:CC:DD:EE:FF',
    ip: '',
    site: '',
    broadcast: '',
    port: 9,
    notes: '',
    addedAt: '',
    lastWokenAt: '',
  })
})

test('docWarning only complains about a newer document', () => {
  assert.equal(docWarning({ version: 1, devices: [] }), '')
  assert.equal(docWarning(null), '')
  assert.equal(docWarning({ version: 2, devices: [] }), NEWER_VERSION_MESSAGE)
})

test('ensureIds fills only the missing ids', () => {
  const got = ensureIds([device({ id: '' }), device({ id: 'wol-keepme', mac: '11:22:33:44:55:66' })], idMaker())
  assert.equal(got[0].id, 'wol-test1')
  assert.equal(got[1].id, 'wol-keepme')
})

test('newDeviceId looks like wol- plus eight hex characters', () => {
  assert.match(newDeviceId(), /^wol-[0-9a-f]{8}$/)
  assert.notEqual(newDeviceId(), newDeviceId())
})

test('mergeDevices adds every new device with an injected id', () => {
  const report = mergeDevices(
    [],
    {
      version: 1,
      devices: [
        { name: 'Reception PC', mac: 'AA:BB:CC:DD:EE:FF' },
        { name: 'Server', mac: '11:22:33:44:55:66' },
      ],
    },
    idMaker(),
    '2026-07-25T10:04:00.000Z',
  )
  assert.equal(report.error, '')
  assert.equal(report.added, 2)
  assert.equal(report.updated, 0)
  assert.equal(report.skipped, 0)
  assert.deepEqual(
    report.devices.map((d) => d.id),
    ['wol-test1', 'wol-test2'],
  )
  assert.equal(report.devices[0].addedAt, '2026-07-25T10:04:00.000Z')
  assert.equal(report.devices[0].lastWokenAt, '')
})

test('mergeDevices fills blanks only and never overwrites', () => {
  const current = [device({ name: 'Reception PC', ip: '', site: 'Head Office' })]
  const report = mergeDevices(
    current,
    {
      version: 1,
      devices: [
        { name: 'Front desk', mac: 'aa:bb:cc:dd:ee:ff', ip: '192.168.1.42', site: 'Branch' },
      ],
    },
    idMaker(),
    '2026-07-25T10:04:00.000Z',
  )
  assert.equal(report.updated, 1)
  assert.equal(report.added, 0)
  assert.equal(report.skipped, 0)
  assert.equal(report.devices[0].name, 'Reception PC')
  assert.equal(report.devices[0].site, 'Head Office')
  assert.equal(report.devices[0].ip, '192.168.1.42')
})

test('mergeDevices changes nothing the second time', () => {
  const file = {
    version: 1,
    devices: [
      { name: 'Reception PC', mac: 'AA:BB:CC:DD:EE:FF', site: 'Head Office' },
      { name: 'Server', mac: '11:22:33:44:55:66', site: 'Head Office' },
    ],
  }
  const first = mergeDevices([], file, idMaker(), '2026-07-25T10:04:00.000Z')
  assert.equal(first.added, 2)

  const second = mergeDevices(first.devices, file, idMaker(), '2026-07-26T10:04:00.000Z')
  assert.equal(second.added, 0)
  assert.equal(second.updated, 0)
  assert.equal(second.skipped, 2)
  assert.deepEqual(second.devices, first.devices)
})

test('mergeDevices skips entries it cannot use', () => {
  const report = mergeDevices(
    [],
    {
      version: 1,
      devices: [{ name: 'Good', mac: 'AA:BB:CC:DD:EE:FF' }, { name: 'No MAC' }, 7],
    },
    idMaker(),
    '2026-07-25T10:04:00.000Z',
  )
  assert.equal(report.added, 1)
  assert.equal(report.skipped, 2)
})

test('mergeDevices rejects a file that is not a device list', () => {
  const current = [device({ name: 'Reception PC' })]
  for (const raw of [null, 'text', {}, { version: 1 }, { devices: 'nope' }]) {
    const report = mergeDevices(current, raw, idMaker(), '2026-07-25T10:04:00.000Z')
    assert.equal(report.error, NOT_A_LIST_MESSAGE)
    assert.equal(report.added, 0)
    assert.equal(report.updated, 0)
    assert.equal(report.skipped, 0)
    assert.deepEqual(report.devices, current)
  }
})

test('mergeDevices sorts by site then name, counting numbers properly', () => {
  const report = mergeDevices(
    [],
    {
      version: 1,
      devices: [
        { name: 'PC 10', mac: '11:22:33:44:55:01', site: 'Branch' },
        { name: 'PC 2', mac: '11:22:33:44:55:02', site: 'Branch' },
        { name: 'Reception PC', mac: '11:22:33:44:55:03', site: 'Head Office' },
        { name: 'Anywhere', mac: '11:22:33:44:55:04', site: '' },
      ],
    },
    idMaker(),
    '2026-07-25T10:04:00.000Z',
  )
  assert.deepEqual(
    report.devices.map((d) => d.name),
    ['Anywhere', 'PC 2', 'PC 10', 'Reception PC'],
  )
})

test('sortDevices puts the sites together', () => {
  const got = sortDevices([
    device({ name: 'B', mac: '11:22:33:44:55:01', site: 'Two' }),
    device({ name: 'A', mac: '11:22:33:44:55:02', site: 'Two' }),
    device({ name: 'C', mac: '11:22:33:44:55:03', site: 'One' }),
  ])
  assert.deepEqual(
    got.map((d) => d.name),
    ['C', 'A', 'B'],
  )
})

test('exportDoc names the format and keeps the devices as they are', () => {
  const devices = [device({ name: 'Reception PC' })]
  const doc = exportDoc(devices, '2026-07-25T10:04:00.000Z')
  assert.equal(doc.version, 1)
  assert.equal(doc.kind, 'chit/wol-devices')
  assert.deepEqual(doc.devices, devices)
  assert.equal(Number.isNaN(new Date(doc.exportedAt).getTime()), false)
})

test('exportFileName falls back to "devices" without a site', () => {
  assert.equal(
    exportFileName('Head Office', '2026-07-25T10:04:00.000Z'),
    'chit-wol-head-office-2026-07-25.json',
  )
  assert.equal(exportFileName('', '2026-07-25T10:04:00.000Z'), 'chit-wol-devices-2026-07-25.json')
  assert.equal(
    exportFileName('  ***  ', '2026-07-25T10:04:00.000Z'),
    'chit-wol-devices-2026-07-25.json',
  )
})

test('filterDevices matches name, MAC, IP and site, and honours the site picker', () => {
  const list = [
    device({ id: 'a', name: 'Reception PC', mac: 'AA:BB:CC:DD:EE:FF', ip: '192.168.1.42', site: 'Head Office' }),
    device({ id: 'b', name: 'Server', mac: '11:22:33:44:55:66', ip: '', site: 'Branch' }),
  ]
  assert.deepEqual(filterDevices(list, '', '').map((d) => d.id), ['a', 'b'])
  assert.deepEqual(filterDevices(list, 'reception', '').map((d) => d.id), ['a'])
  assert.deepEqual(filterDevices(list, '11:22', '').map((d) => d.id), ['b'])
  assert.deepEqual(filterDevices(list, '192.168.1.42', '').map((d) => d.id), ['a'])
  assert.deepEqual(filterDevices(list, '', 'Branch').map((d) => d.id), ['b'])
  assert.deepEqual(filterDevices(list, 'server', 'Head Office').map((d) => d.id), [])
})
