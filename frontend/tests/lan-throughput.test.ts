import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import {
  curlFor,
  linkForAddress,
  localTime,
  mbpsText,
  pullId,
  secondsText,
  speedReading,
  statusLabel,
  summaryLine,
  validPort,
} from '../src/tools/lan-throughput/reading.ts'

const goldenPath = fileURLToPath(new URL('../../testdata/lanspeed-readings.json', import.meta.url))
const golden = JSON.parse(readFileSync(goldenPath, 'utf8')) as {
  cases: { mbps: number; reading: string }[]
}

test('speedReading matches the golden file the Go side reads', () => {
  assert.ok(golden.cases.length >= 17, 'the golden file must cover both sides of every threshold')
  for (const c of golden.cases) {
    assert.equal(speedReading(c.mbps), c.reading, `at ${c.mbps} Mbps`)
  }
})

test('mbpsText keeps one decimal place and never renders a negative', () => {
  assert.equal(mbpsText(742.14), '742.1 Mbps')
  assert.equal(mbpsText(94), '94.0 Mbps')
  assert.equal(mbpsText(0), '0 Mbps')
  assert.equal(mbpsText(-5), '0 Mbps')
  assert.equal(mbpsText(Number.NaN), '0 Mbps')
})

test('secondsText keeps one decimal place', () => {
  assert.equal(secondsText(2.34), '2.3 s')
  assert.equal(secondsText(0), '0 s')
  assert.equal(secondsText(-1), '0 s')
})

test('summaryLine reports the count, the best figure and the total', () => {
  assert.equal(
    summaryLine(2, 742.1, 400 * 1024 * 1024, 184000),
    '2 pulls, best 742.1 Mbps, 400 MB sent in 3 m 4 s',
  )
  assert.equal(
    summaryLine(1, 94.2, 200 * 1024 * 1024, 18000),
    '1 pull, best 94.2 Mbps, 200 MB sent in 18.0 s',
  )
  assert.equal(summaryLine(0, 0, 0, 5000), 'No pulls in 5.0 s')
})

test('curlFor appends the download path', () => {
  assert.equal(
    curlFor('http://10.40.21.153:8740/t/abc'),
    'curl -o /dev/null http://10.40.21.153:8740/t/abc/dl',
  )
  assert.equal(
    curlFor('http://[fe80::1]:8740/t/abc'),
    'curl -o /dev/null http://[fe80::1]:8740/t/abc/dl',
  )
})

test('linkForAddress swaps only the host', () => {
  const url = 'http://10.40.21.153:8740/t/a1b2c3d4e5f6a7b8'
  assert.equal(linkForAddress(url, '192.168.1.44'), 'http://192.168.1.44:8740/t/a1b2c3d4e5f6a7b8')
  // The port, the /t/ segment and the token must survive untouched, which is
  // what makes the QR code and the link on screen the same address.
  assert.equal(linkForAddress(url, '10.40.21.153'), url)
})

test('linkForAddress brackets an IPv6 address and replaces a bracketed one', () => {
  const v4 = 'http://10.40.21.153:8740/t/abc'
  assert.equal(linkForAddress(v4, 'fe80::1'), 'http://[fe80::1]:8740/t/abc')
  assert.equal(
    linkForAddress('http://[fe80::1]:8740/t/abc', '10.0.0.7'),
    'http://10.0.0.7:8740/t/abc',
  )
  assert.equal(
    linkForAddress('http://[fe80::1]:8740/t/abc', 'fe80::2'),
    'http://[fe80::2]:8740/t/abc',
  )
})

test('linkForAddress leaves anything it does not understand alone', () => {
  const url = 'http://10.40.21.153:8740/t/abc'
  assert.equal(linkForAddress('', '10.0.0.7'), '')
  assert.equal(linkForAddress(url, ''), url)
  assert.equal(linkForAddress('10.40.21.153:8740/t/abc', '10.0.0.7'), '10.40.21.153:8740/t/abc')
  // No port to keep, so there is nothing safe to rebuild.
  assert.equal(linkForAddress('http://10.40.21.153/t/abc', '10.0.0.7'), 'http://10.40.21.153/t/abc')
  assert.equal(linkForAddress('http://[fe80::1]/t/abc', '10.0.0.7'), 'http://[fe80::1]/t/abc')
})

test('linkForAddress reads the host and not the path after it', () => {
  // The token is hex by construction, so this pins the anchoring rather than a
  // reachable bug: only the authority may be rewritten.
  assert.equal(
    linkForAddress('http://10.40.21.153:8740/t/aa:bb/cc', '10.0.0.7'),
    'http://10.0.0.7:8740/t/aa:bb/cc',
  )
})

test('statusLabel names both outcomes', () => {
  assert.equal(statusLabel('ok'), 'Complete')
  assert.equal(statusLabel('stopped'), 'Stopped part way')
  assert.equal(statusLabel('anything else'), 'Stopped part way')
})

test('localTime renders a wall-clock time and rejects garbage', () => {
  const at = new Date(2026, 6, 28, 9, 5, 3)
  assert.equal(localTime(at.toISOString()), '09:05:03')
  assert.equal(localTime('not a time'), '')
})

test('pullId distinguishes two pulls from the same machine', () => {
  const base = {
    time: '2026-07-28T10:00:00Z',
    peer: '10.0.0.7',
    bytes: 1,
    seconds: 1,
    mbps: 1,
    status: 'ok',
    reading: '',
  }
  assert.notEqual(pullId(base), pullId({ ...base, bytes: 2 }))
  assert.equal(pullId(base), pullId({ ...base }))
})

test('validPort accepts the range and rejects outside it', () => {
  assert.deepEqual(validPort('8740'), { ok: true, port: 8740 })
  assert.deepEqual(validPort(''), { ok: true, port: 0 })
  const low = validPort('1023')
  assert.equal(low.ok, false)
  assert.match(low.ok === false ? low.error : '', /administrator rights/)
  assert.equal(validPort('65536').ok, false)
  assert.equal(validPort('abc').ok, false)
})
