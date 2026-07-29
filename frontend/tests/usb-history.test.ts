import test from 'node:test'
import assert from 'node:assert/strict'
import {
  countLine,
  filterDevices,
  firstSeenLabel,
  kindLabel,
  sortDevices,
  vidPid,
} from '../src/tools/usb-history/devices.ts'
import type { Device } from '../src/tools/usb-history/api.ts'

function device(over: Partial<Device> = {}): Device {
  return {
    name: 'Thing',
    manufacturer: 'Maker',
    vendorId: '1d6b',
    productId: '0002',
    serial: '',
    kind: 'other',
    connected: true,
    firstSeen: '',
    source: '/sys/bus/usb',
    ...over,
  }
}

test('vidPid joins both ids, and never shows half of one', () => {
  assert.equal(vidPid(device()), '1d6b:0002')
  assert.equal(vidPid(device({ vendorId: '', productId: '' })), '')
  assert.equal(vidPid(device({ vendorId: '1d6b', productId: '' })), '')
  assert.equal(vidPid(device({ vendorId: '', productId: '0002' })), '')
})

test('vidPid never produces a stray colon', () => {
  for (const d of [
    device({ vendorId: '', productId: '0002' }),
    device({ vendorId: '1d6b', productId: '' }),
    device({ vendorId: '', productId: '' }),
  ]) {
    assert.ok(!vidPid(d).includes(':'), `got ${vidPid(d)}`)
  }
})

test('kindLabel covers every kind and falls back safely', () => {
  assert.equal(kindLabel('storage'), 'Storage')
  assert.equal(kindLabel('input'), 'Keyboard or mouse')
  assert.equal(kindLabel('audio'), 'Audio')
  assert.equal(kindLabel('video'), 'Camera')
  assert.equal(kindLabel('network'), 'Network')
  assert.equal(kindLabel('printer'), 'Printer')
  assert.equal(kindLabel('hub'), 'Hub')
  assert.equal(kindLabel('other'), 'Other')
  assert.equal(kindLabel('something-new'), 'Other')
  assert.equal(kindLabel(''), 'Other')
})

test('countLine says what the list means on an OS with a history', () => {
  const devices = [
    ...Array.from({ length: 4 }, () => device({ connected: true })),
    ...Array.from({ length: 7 }, () => device({ connected: false })),
  ]
  assert.equal(countLine(devices, true), '4 devices connected now, 7 seen before.')
})

test('countLine leaves the history out on an OS that keeps none', () => {
  const devices = Array.from({ length: 4 }, () => device({ connected: true }))
  assert.equal(countLine(devices, false), '4 devices connected now.')
})

test('countLine uses the singular', () => {
  assert.equal(
    countLine([device({ connected: true }), device({ connected: false })], true),
    '1 device connected now, 1 seen before.',
  )
})

test('countLine says so plainly when nothing is plugged in', () => {
  assert.equal(countLine([], false), 'Nothing is connected now.')
  assert.equal(
    countLine([device({ connected: false }), device({ connected: false })], true),
    'Nothing is connected now, 2 seen before.',
  )
})

const FIXTURE: Device[] = [
  device({ name: 'Stick', kind: 'storage', connected: true }),
  device({ name: 'Old Stick', kind: 'storage', connected: false }),
  device({ name: 'Keyboard', kind: 'input', connected: true }),
  device({ name: 'Hub', kind: 'hub', connected: true }),
]

test('filterDevices keeps exactly the right rows', () => {
  assert.equal(filterDevices(FIXTURE, 'all').length, 4)
  assert.equal(filterDevices(FIXTURE, 'connected').length, 3)
  assert.equal(filterDevices(FIXTURE, 'storage').length, 2)
  assert.equal(filterDevices(FIXTURE, 'seen').length, 1)
})

test('filterDevices on an empty list is empty, not a crash', () => {
  assert.deepEqual(filterDevices([], 'storage'), [])
})

test('sortDevices puts connected first, then storage, then numbers as numbers', () => {
  const devices = [
    device({ name: 'Disk 10', kind: 'storage', connected: true }),
    device({ name: 'Old Stick', kind: 'storage', connected: false }),
    device({ name: 'Keyboard', kind: 'input', connected: true }),
    device({ name: 'Disk 2', kind: 'storage', connected: true }),
  ]
  assert.deepEqual(
    sortDevices(devices).map((d) => d.name),
    ['Disk 2', 'Disk 10', 'Keyboard', 'Old Stick'],
  )
})

test('sortDevices does not change the array it was given', () => {
  const devices = [device({ name: 'B', connected: false }), device({ name: 'A', connected: true })]
  sortDevices(devices)
  assert.equal(devices[0].name, 'B', 'the original array was reordered in place')
})

test('firstSeenLabel is empty when there is nothing to show', () => {
  assert.equal(firstSeenLabel(device({ firstSeen: '' })), '')
  assert.equal(firstSeenLabel(device({ firstSeen: 'not a date' })), '')
  assert.notEqual(firstSeenLabel(device({ firstSeen: '2024-09-05T08:53:20Z' })), '')
})
