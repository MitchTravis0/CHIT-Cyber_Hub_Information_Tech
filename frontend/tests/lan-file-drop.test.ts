import test from 'node:test'
import assert from 'node:assert/strict'
import {
  baseName,
  directionLabel,
  fileListLine,
  statusLabel,
  summaryLine,
  toPicked,
  transferId,
  transferTime,
  validPort,
} from '../src/tools/lan-file-drop/session.ts'
import type { Transfer } from '../src/tools/lan-file-drop/api.ts'
import { linkForAddress } from '../src/tools/lan-throughput/reading.ts'

test('the shared link follows the address picker, keeping the /d/ path and token', () => {
  // The page rewrites the host of the link the backend built rather than
  // rebuilding it, so the scheme, port, path segment and token are always Go's.
  const built = 'http://10.2.136.81:8722/d/k3f9x2qp'
  assert.equal(linkForAddress(built, '192.168.1.44'), 'http://192.168.1.44:8722/d/k3f9x2qp')
  assert.equal(linkForAddress(built, 'fd00::1'), 'http://[fd00::1]:8722/d/k3f9x2qp')
  // Before the address list arrives there is nothing to pick, and the link the
  // backend built must show unchanged rather than blanking.
  assert.equal(linkForAddress(built, ''), built)
})

test('baseName handles both separators', () => {
  assert.equal(baseName('/home/me/driver.exe'), 'driver.exe')
  assert.equal(baseName('C:\\Users\\me\\driver.exe'), 'driver.exe')
  assert.equal(baseName('plain.txt'), 'plain.txt')
  assert.equal(baseName(''), '')
})

test('toPicked keeps the path and names the file', () => {
  assert.deepEqual(toPicked(['/a/b/one.txt', 'C:\\x\\two.bin']), [
    { path: '/a/b/one.txt', name: 'one.txt' },
    { path: 'C:\\x\\two.bin', name: 'two.bin' },
  ])
  assert.deepEqual(toPicked([]), [])
})

test('fileListLine reads as a sentence', () => {
  const three = toPicked(['/a/1', '/a/2', '/a/3'])
  assert.equal(fileListLine(three, 225_182_515), '3 files, 215 MB')
  assert.equal(fileListLine(toPicked(['/a/1']), 12_288), '1 file, 12 KB')
  assert.equal(fileListLine([], 0), 'No files chosen yet.')
})

test('summaryLine reads as a sentence', () => {
  assert.equal(
    summaryLine({ downloads: 2, uploads: 1, bytesOut: 225_182_515, bytesIn: 3_250_586 }, 252_000),
    'Shared for 4 m 12 s: 2 files sent (215 MB), 1 received (3.1 MB)',
  )
})

test('summaryLine uses the singular and copes with nothing having happened', () => {
  assert.equal(
    summaryLine({ downloads: 1, uploads: 0, bytesOut: 1024, bytesIn: 0 }, 5000),
    'Shared for 5.0 s: 1 file sent (1 KB), 0 received (0 B)',
  )
  assert.equal(
    summaryLine({ downloads: 0, uploads: 0, bytesOut: 0, bytesIn: 0 }, 1000),
    'Shared for 1.0 s: 0 files sent (0 B), 0 received (0 B)',
  )
})

test('validPort accepts the whole allowed range', () => {
  for (const [text, port] of [
    ['8722', 8722],
    ['1024', 1024],
    ['65535', 65535],
    ['  8722  ', 8722],
  ] as Array<[string, number]>) {
    const result = validPort(text)
    assert.equal(result.ok, true, `${text} should be accepted`)
    if (result.ok) assert.equal(result.port, port)
  }
})

test('validPort refuses a port that needs administrator rights', () => {
  const result = validPort('80')
  assert.equal(result.ok, false)
  if (!result.ok) {
    assert.equal(
      result.error,
      'The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.',
    )
  }
})

test('validPort refuses everything that is not a port', () => {
  for (const text of ['', '   ', 'abc', '70000', '0', '1023', '-1', '80.5', '8722x']) {
    const result = validPort(text)
    assert.equal(result.ok, false, `${text} should be refused`)
    if (!result.ok) assert.notEqual(result.error, '', 'a refusal must say why')
  }
})

function transfer(over: Partial<Transfer> = {}): Transfer {
  return {
    time: '2026-07-27T14:02:03Z',
    direction: 'download',
    name: 'driver.exe',
    bytes: 1024,
    peer: '192.168.1.50',
    status: 'ok',
    message: '',
    ...over,
  }
}

test('transferId tells two identical downloads apart by time', () => {
  const first = transfer({ time: '2026-07-27T14:02:03Z' })
  const second = transfer({ time: '2026-07-27T14:05:11Z' })
  assert.notEqual(transferId(first), transferId(second))
  assert.equal(transferId(first), transferId(transfer({ time: '2026-07-27T14:02:03Z' })))
})

test('transferId separates a download from an upload of the same file', () => {
  assert.notEqual(
    transferId(transfer({ direction: 'download' })),
    transferId(transfer({ direction: 'upload' })),
  )
})

test('transferTime survives a timestamp it cannot read', () => {
  assert.equal(transferTime(transfer({ time: 'not a date' })), '')
  assert.notEqual(transferTime(transfer()), '')
})

test('directionLabel and statusLabel are words, not colours', () => {
  assert.equal(directionLabel('download'), 'Sent')
  assert.equal(directionLabel('upload'), 'Received')
  assert.equal(statusLabel('ok'), 'Done')
  assert.equal(statusLabel('failed'), 'Failed')
})
