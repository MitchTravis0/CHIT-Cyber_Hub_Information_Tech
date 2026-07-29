// The timing bar is the one thing this tool works out for itself: the backend
// reports stage durations, the page turns them into widths.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Timing } from '../src/tools/uptime-ssl-checker/api.ts'
import { reusedConnection, timingSegments } from '../src/tools/uptime-ssl-checker/timing.ts'

function timing(partial: Partial<Timing>): Timing {
  return { dnsMs: 0, connectMs: 0, tlsMs: 0, ttfbMs: 0, totalMs: 0, ...partial }
}

test('splits the time to first byte into four stages', () => {
  const segments = timingSegments(timing({ dnsMs: 10, connectMs: 20, tlsMs: 30, ttfbMs: 100 }))
  assert.deepEqual(
    segments.map((s) => [s.key, s.ms, s.percent]),
    [
      ['dns', 10, 10],
      ['connect', 20, 20],
      ['tls', 30, 30],
      ['server', 40, 40],
    ],
  )
})

test('the four percentages add up to 100', () => {
  const segments = timingSegments(timing({ dnsMs: 3, connectMs: 7, tlsMs: 11, ttfbMs: 90 }))
  const total = segments.reduce((sum, s) => sum + s.percent, 0)
  assert.ok(Math.abs(total - 100) < 1e-9, `percentages summed to ${total}`)
})

test('an empty timing gives no widths instead of dividing by zero', () => {
  const segments = timingSegments(timing({}))
  assert.deepEqual(
    segments.map((s) => s.percent),
    [0, 0, 0, 0],
  )
})

test('a first byte earlier than the handshake leaves no negative server time', () => {
  const segments = timingSegments(timing({ dnsMs: 10, connectMs: 20, tlsMs: 30, ttfbMs: 5 }))
  const server = segments[3]
  assert.equal(server.key, 'server')
  assert.equal(server.ms, 0)
  assert.equal(server.percent, 0)
})

test('one stage on its own takes the whole bar', () => {
  const segments = timingSegments(timing({ dnsMs: 0, connectMs: 0, tlsMs: 0, ttfbMs: 250 }))
  assert.deepEqual(
    segments.map((s) => s.percent),
    [0, 0, 0, 100],
  )
})

test('a reused connection is the one with no lookup, connect or handshake', () => {
  assert.equal(reusedConnection(timing({ ttfbMs: 40 })), true)
  assert.equal(reusedConnection(timing({ dnsMs: 1, ttfbMs: 40 })), false)
  assert.equal(reusedConnection(timing({ connectMs: 1 })), false)
  assert.equal(reusedConnection(timing({ tlsMs: 1 })), false)
})
