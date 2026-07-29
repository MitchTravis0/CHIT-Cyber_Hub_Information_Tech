import test from 'node:test'
import assert from 'node:assert/strict'
import {
  NOT_AVAILABLE,
  NOT_REPORTED,
  diskLine,
  fieldText,
  formatUptime,
  reportText,
} from '../src/tools/system-info/report.ts'
import type { Report } from '../src/tools/system-info/api.ts'

test('formatUptime shows the two largest non-zero units', () => {
  const cases: Array<[number, string]> = [
    [0, 'less than a minute'],
    [59, 'less than a minute'],
    [60, '1 minute'],
    [120, '2 minutes'],
    [3599, '59 minutes'],
    [3600, '1 hour'],
    [5400, '1 hour 30 minutes'],
    [7200, '2 hours'],
    [86400, '1 day'],
    [90000, '1 day 1 hour'],
    [1036800, '12 days'],
    [1040400, '12 days 1 hour'],
  ]
  for (const [seconds, want] of cases) {
    assert.equal(formatUptime(seconds), want, `formatUptime(${seconds})`)
  }
})

test('formatUptime never shows seconds above one minute', () => {
  assert.equal(formatUptime(3661), '1 hour 1 minute')
  assert.ok(!formatUptime(3661).includes('second'))
})

test('fieldText tells the two empty cases apart', () => {
  assert.equal(fieldText('Arch Linux', 'osName', []), 'Arch Linux')
  assert.equal(fieldText('', 'serial', ['serial']), NOT_AVAILABLE)
  assert.equal(fieldText('', 'serial', []), NOT_REPORTED)
  assert.equal(fieldText('', 'serial', ['model']), NOT_REPORTED)
})

test('fieldText never returns an empty string', () => {
  for (const unsupported of [[], ['serial']]) {
    assert.notEqual(fieldText('', 'serial', unsupported), '')
  }
})

test('diskLine pads the mount and the filesystem to fixed widths', () => {
  const line = diskLine({
    mount: '/',
    fs: 'ext4',
    total: 511503761408,
    used: 480839495680,
    free: 30664265728,
    usedPct: 94,
  })
  assert.equal(
    line,
    '/            ext4     476 GB total, 448 GB used, 29 GB free, 94% full',
  )
  // 12 characters of mount, a space, 8 of filesystem, a space.
  assert.equal(line.slice(0, 12), '/           ')
  assert.equal(line.slice(13, 21), 'ext4    ')
})

test('diskLine truncates a mount longer than the column', () => {
  const line = diskLine({
    mount: '/mnt/a-very-long-mount-point',
    fs: 'btrfs',
    total: 1024,
    used: 512,
    free: 512,
    usedPct: 50,
  })
  assert.ok(line.startsWith('/mnt/a-very- btrfs   '), line)
})

const FULL: Report = {
  hostname: 'dev-box',
  user: 'mtrav',
  os: 'linux',
  osName: 'Arch Linux',
  osVersion: '6.12.4-arch1-1',
  arch: 'amd64',
  manufacturer: 'LENOVO',
  model: '20XW00ABUK',
  serial: '',
  cpuModel: '11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz',
  cpuCores: 8,
  memoryTotal: 16703008768,
  memoryFree: 4002480128,
  uptimeS: 1036800,
  bootTime: '2026-07-15T08:02:11Z',
  appVersion: '0.1.0',
  disks: [
    { mount: '/', fs: 'ext4', total: 511503761408, used: 480839495680, free: 30664265728, usedPct: 94 },
    { mount: '/boot', fs: 'vfat', total: 1073741824, used: 322122547, free: 751619277, usedPct: 30 },
  ],
  unsupported: ['serial'],
}

test('reportText builds the whole snapshot in one fixed layout', () => {
  assert.equal(
    reportText(FULL),
    [
      'CHIT system snapshot',
      '',
      'Computer name: dev-box',
      'Signed in as: mtrav',
      'Operating system: Arch Linux',
      'Version: 6.12.4-arch1-1',
      'Architecture: amd64',
      'Up for: 12 days',
      'Started: 2026-07-15T08:02:11Z',
      '',
      'Manufacturer: LENOVO',
      'Model: 20XW00ABUK',
      'Serial number: not available on this OS',
      'Processor: 11th Gen Intel(R) Core(TM) i7-1185G7 @ 3.00GHz',
      'Processor cores: 8',
      'Memory fitted: 16 GB',
      'Memory free: 3.7 GB',
      '',
      'Drives',
      '/            ext4     476 GB total, 448 GB used, 29 GB free, 94% full',
      '/boot        vfat     1 GB total, 307 MB used, 717 MB free, 30% full',
      '',
      'Read with CHIT 0.1.0 on linux/amd64',
    ].join('\n'),
  )
})

test('reportText still prints the Drives heading when there are none', () => {
  const text = reportText({ ...FULL, disks: [] })
  assert.ok(text.includes('Drives\nnone reported'), text)
})

test('reportText tolerates null slices from a Go nil', () => {
  const text = reportText({ ...FULL, disks: null, unsupported: null })
  assert.ok(text.includes('none reported'))
  // With no unsupported list, an empty serial is "not reported", not "not available".
  assert.ok(text.includes('Serial number: not reported'), text)
})

test('reportText marks an unsupported uptime rather than printing zero', () => {
  const text = reportText({ ...FULL, uptimeS: 0, unsupported: ['uptime'] })
  assert.ok(text.includes('Up for: not available on this OS'), text)
})
