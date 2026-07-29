// The bar lengths, the flagged jump and the row tones are all derived in the
// frontend from the streamed hops, so this is the arithmetic a tech reads off
// the path view.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Hop } from '../src/tools/traceroute/api.ts'
import {
  barWidth,
  biggestJump,
  hopLabel,
  hopTone,
  mergeHops,
} from '../src/tools/traceroute/hops.ts'

function hop(number: number, avgMs: number, extra: Partial<Hop> = {}): Hop {
  const answered = avgMs > 0
  return {
    number,
    ip: `10.0.0.${number}`,
    hostname: '',
    timesMs: answered ? [avgMs] : [],
    lost: answered ? 0 : 3,
    bestMs: avgMs,
    avgMs,
    worstMs: avgMs,
    alsoSeen: [],
    note: '',
    final: false,
    ...extra,
  }
}

function silent(number: number): Hop {
  return hop(number, 0)
}

test('mergeHops keeps the last emit per hop and sorts ascending', () => {
  const merged = mergeHops([hop(3, 30), hop(1, 10), hop(2, 20), hop(1, 11)])
  assert.deepEqual(
    merged.map((row) => row.number),
    [1, 2, 3],
  )
  assert.equal(merged[0].avgMs, 11)
  assert.equal(merged.length, 3)
})

test('barWidth is 100 for the slowest hop and proportional for the rest', () => {
  const hops = [hop(1, 10), hop(2, 40), hop(3, 20)]
  assert.equal(barWidth(hops[1], hops), 100)
  assert.equal(barWidth(hops[0], hops), 25)
  assert.equal(barWidth(hops[2], hops), 50)
})

test('barWidth is 0 for a hop with no times', () => {
  const hops = [hop(1, 10), silent(2)]
  assert.equal(barWidth(hops[1], hops), 0)
})

test('barWidth is 0 when no hop answered', () => {
  const hops = [silent(1), silent(2)]
  assert.equal(barWidth(hops[0], hops), 0)
})

test('biggestJump ignores silent hops between two answers', () => {
  const hops = [hop(1, 10), silent(2), hop(3, 90)]
  assert.equal(biggestJump(hops), 3)
})

test('biggestJump returns -1 with fewer than two answering hops', () => {
  assert.equal(biggestJump([]), -1)
  assert.equal(biggestJump([hop(1, 10)]), -1)
  assert.equal(biggestJump([hop(1, 10), silent(2)]), -1)
})

test('biggestJump breaks ties on the earlier hop', () => {
  const hops = [hop(1, 10), hop(2, 20), hop(3, 30)]
  assert.equal(biggestJump(hops), 2)
})

test('hopTone marks a fully lost hop danger and a partly lost hop warn', () => {
  assert.equal(hopTone(hop(1, 0, { timesMs: [], lost: 3 })), 'danger')
  assert.equal(hopTone(hop(2, 11, { timesMs: [10, 12], lost: 1 })), 'warn')
  assert.equal(hopTone(hop(3, 11, { timesMs: [10, 12], lost: 0 })), undefined)
})

test('hopLabel prefers the note, then No reply, then empty', () => {
  assert.equal(
    hopLabel(hop(1, 20, { note: 'the router said it cannot reach that host' })),
    'the router said it cannot reach that host',
  )
  assert.equal(hopLabel(silent(2)), 'No reply')
  assert.equal(hopLabel(hop(3, 20)), '')
})
