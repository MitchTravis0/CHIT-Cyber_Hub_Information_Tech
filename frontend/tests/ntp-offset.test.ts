import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import {
  describeOffset,
  resultLabel,
  resultTone,
  summaryText,
} from '../src/tools/ntp-check/offset.ts'

test('describeOffset words a gap the way a tech would say it', () => {
  // Every expected string was produced by running the real function once and
  // pasting the result, not written from memory.
  assert.equal(describeOffset(0), '+0 ms')
  assert.equal(describeOffset(32), '+32 ms')
  assert.equal(describeOffset(-32), '-32 ms')
  assert.equal(describeOffset(999), '+999 ms')
  assert.equal(describeOffset(-999), '-999 ms')
  assert.equal(describeOffset(1000), '+1 s')
  assert.equal(describeOffset(-1000), '-1 s')
  assert.equal(describeOffset(7000), '+7 s')
  assert.equal(describeOffset(-7000), '-7 s')
  assert.equal(describeOffset(59000), '+59 s')
  assert.equal(describeOffset(60000), '+1 m 0 s')
  assert.equal(describeOffset(432000), '+7 m 12 s')
  assert.equal(describeOffset(-432000), '-7 m 12 s')
  assert.equal(describeOffset(3600000), '+1 h 0 m 0 s')
  assert.equal(describeOffset(3672000), '+1 h 1 m 12 s')
})

test('describeOffset always shows a sign', () => {
  for (const ms of [0, 1, -1, 999, -999, 1000, -1000, 500000, -500000]) {
    const got = describeOffset(ms)
    assert.ok(got.startsWith('+') || got.startsWith('-'), `${got} has no sign`)
  }
})

test('describeOffset matches the Go wording, case for case', () => {
  // The same golden file internal/tools/ntpcheck/golden_test.go reads. Without
  // this the Difference column and the sentence under the table are free to
  // disagree about the same number.
  const path = fileURLToPath(new URL('../../testdata/ntp-gap-cases.json', import.meta.url))
  const golden = JSON.parse(readFileSync(path, 'utf8')) as {
    cases: { ms: number; gap: string }[]
  }
  assert.ok(golden.cases.length >= 20, 'golden file shrank, so this check would prove little')

  for (const c of golden.cases) {
    assert.equal(describeOffset(c.ms), `+${c.gap}`, `+${c.ms} ms`)
    if (c.ms !== 0) {
      assert.equal(describeOffset(-c.ms), `-${c.gap}`, `-${c.ms} ms`)
    }
  }
})

test('resultLabel names every status a row can carry', () => {
  assert.equal(resultLabel('ok'), 'Fine')
  assert.equal(resultLabel('warn'), 'Drifting')
  assert.equal(resultLabel('error'), 'Too far out')
  assert.equal(resultLabel('unreachable'), 'No answer')
  assert.equal(resultLabel('something else'), 'No answer')
})

test('resultTone maps every status to a dot colour', () => {
  assert.equal(resultTone('ok'), 'ok')
  assert.equal(resultTone('warn'), 'warn')
  assert.equal(resultTone('error'), 'danger')
  assert.equal(resultTone('unreachable'), 'idle')
  assert.equal(resultTone(''), 'idle')
})

test('summaryText writes one line per server, ready for a ticket', () => {
  const got = summaryText(
    [
      {
        server: 'pool.ntp.org',
        address: '1.2.3.4:123',
        offsetMs: 32,
        delayMs: 12,
        stratum: 2,
        serverTime: '',
        localTime: '',
        status: 'ok',
        message: '',
      },
      {
        server: 'dc01',
        address: '',
        offsetMs: 0,
        delayMs: 0,
        stratum: 0,
        serverTime: '',
        localTime: '',
        status: 'unreachable',
        message: '',
      },
    ],
    '2026-07-28T09:14:22Z',
  )
  assert.equal(
    got,
    'pool.ntp.org: this computer is +32 ms (checked 2026-07-28T09:14:22Z)\n' +
      'dc01: no answer (checked 2026-07-28T09:14:22Z)',
  )
})
