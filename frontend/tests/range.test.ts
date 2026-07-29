// Holds the frontend range parser to the same golden file as the Go one
// (internal/netscan/iprange_test.go, TestParseRangeSharedCases). The grid is
// laid out from this copy while the scan comes from Go, so a difference between
// them either misaligns the grid or starts a scan the backend then refuses.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import {
  ipToNumber,
  lastOctet,
  numberToIP,
  parseRange,
  MAX_ADDRESSES,
} from '../src/tools/ip-range-scanner/range.ts'

interface GoldenCase {
  input: string
  ok: boolean
  start?: string
  end?: string
  count?: number
  text?: string
  error?: string
}

const goldenPath = fileURLToPath(new URL('../../testdata/iprange-cases.json', import.meta.url))
const golden = JSON.parse(readFileSync(goldenPath, 'utf8')) as { cases: GoldenCase[] }

test('the golden file is not truncated', () => {
  assert.ok(golden.cases.length >= 50, `only ${golden.cases.length} cases`)
})

for (const c of golden.cases) {
  test(`parseRange ${JSON.stringify(c.input)}`, () => {
    const got = parseRange(c.input)
    if (!c.ok) {
      assert.equal(got.ok, false, `expected an error, got ${JSON.stringify(got)}`)
      if (!got.ok) assert.equal(got.error, c.error)
      return
    }
    assert.equal(got.ok, true, got.ok ? '' : got.error)
    if (!got.ok) return
    assert.equal(numberToIP(got.range.start), c.start)
    assert.equal(numberToIP(got.range.end), c.end)
    assert.equal(got.range.count, c.count)
    assert.equal(got.range.text, c.text)
  })
}

test('a range covers exactly the addresses between its ends', () => {
  const got = parseRange('192.168.1.254-192.168.2.2')
  assert.equal(got.ok, true)
  if (!got.ok) return
  const addresses = []
  for (let i = 0; i < got.range.count; i++) addresses.push(numberToIP(got.range.start + i))
  assert.deepEqual(addresses, [
    '192.168.1.254',
    '192.168.1.255',
    '192.168.2.0',
    '192.168.2.1',
    '192.168.2.2',
  ])
})

test('the last address of the space does not wrap', () => {
  const got = parseRange('255.255.255.250-255.255.255.255')
  assert.equal(got.ok, true)
  if (!got.ok) return
  assert.equal(got.range.count, 6)
  assert.equal(numberToIP(got.range.start + got.range.count - 1), '255.255.255.255')
})

test('the cap is inclusive', () => {
  const ok = parseRange('10.0.0.0-10.0.255.255')
  assert.equal(ok.ok, true)
  if (ok.ok) assert.equal(ok.range.count, MAX_ADDRESSES)
  assert.equal(parseRange('10.0.0.0-10.1.0.0').ok, false)
})

test('addresses convert both ways', () => {
  for (const ip of ['0.0.0.0', '10.0.0.1', '192.168.1.254', '255.255.255.255']) {
    const value = ipToNumber(ip)
    assert.notEqual(value, null)
    assert.equal(numberToIP(value as number), ip)
  }
  assert.equal(ipToNumber('1.2.3'), null)
  assert.equal(ipToNumber('1.2.3.4.5'), null)
  assert.equal(ipToNumber('1.2.3.256'), null)
  assert.equal(ipToNumber('1.2.3.04'), null)
  assert.equal(ipToNumber(''), null)
})

test('lastOctet is the number painted on a grid cell', () => {
  assert.equal(lastOctet(ipToNumber('192.168.1.0') as number), 0)
  assert.equal(lastOctet(ipToNumber('192.168.1.77') as number), 77)
  assert.equal(lastOctet(ipToNumber('10.0.255.255') as number), 255)
})
