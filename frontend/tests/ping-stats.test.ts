// The Ping Monitor's cards, drop log and clock column are all derived from the
// streamed samples, so this is the arithmetic a tech puts in a ticket.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Sample } from '../src/tools/ping-monitor/api.ts'
import {
  dropRows,
  formatClock,
  seriesFor,
  targetStats,
} from '../src/tools/ping-monitor/stats.ts'

function reply(target: string, round: number, latencyMs: number, ip = '10.0.0.1'): Sample {
  return { round, target, ip, ok: true, latencyMs, via: 'icmp', at: 0, reason: '' }
}

function loss(target: string, round: number, ip = '10.0.0.1'): Sample {
  return {
    round,
    target,
    ip,
    ok: false,
    latencyMs: 0,
    via: '',
    at: 0,
    reason: 'No reply within 1000 ms.',
  }
}

test('targetStats counts sent, received and lost per target', () => {
  const samples: Sample[] = []
  for (let round = 1; round <= 5; round++) {
    samples.push(round === 2 || round === 4 ? loss('a', round) : reply('a', round, 10))
    samples.push(reply('b', round, 20, '10.0.0.2'))
  }

  const [a, b] = targetStats(samples)
  assert.equal(a.target, 'a')
  assert.equal(a.sent, 5)
  assert.equal(a.received, 3)
  assert.equal(a.lost, 2)
  assert.equal(a.lossPct, 40)
  assert.equal(b.sent, 5)
  assert.equal(b.received, 5)
  assert.equal(b.lost, 0)
  assert.equal(b.lossPct, 0)
  assert.equal(b.ip, '10.0.0.2')
  assert.equal(b.up, true)
})

test('targetStats reports 0 stats before anything answers', () => {
  const [only] = targetStats([loss('a', 1, ''), loss('a', 2, '')])
  assert.equal(only.minMs, 0)
  assert.equal(only.avgMs, 0)
  assert.equal(only.maxMs, 0)
  assert.equal(only.lastMs, -1)
  assert.equal(only.jitterMs, 0)
  assert.equal(only.up, false)
  assert.equal(only.lossPct, 100)
  assert.equal(only.ip, '')
})

test('targetStats computes min, avg and max over replies only', () => {
  const [only] = targetStats([
    reply('a', 1, 10),
    loss('a', 2),
    reply('a', 3, 30),
    reply('a', 4, 20),
  ])
  assert.equal(only.minMs, 10)
  assert.equal(only.avgMs, 20)
  assert.equal(only.maxMs, 30)
  assert.equal(only.lastMs, 20)
})

test('targetStats rounds loss percent to one decimal', () => {
  const [only] = targetStats([reply('a', 1, 1), reply('a', 2, 1), loss('a', 3)])
  assert.equal(only.lossPct, 33.3)
})

test('targetStats measures jitter across a gap', () => {
  const [only] = targetStats([
    reply('a', 1, 10),
    loss('a', 2),
    reply('a', 3, 20),
    reply('a', 4, 30),
  ])
  assert.equal(only.jitterMs, 10)
})

test('targetStats finds the longest outage', () => {
  const [only] = targetStats([
    reply('a', 1, 5),
    loss('a', 2),
    loss('a', 3),
    reply('a', 4, 5),
    loss('a', 5),
  ])
  assert.equal(only.longestOutage, 2)
  assert.equal(only.up, false)
  assert.equal(only.lastMs, -1)
})

test('targetStats keeps the order the targets first appeared', () => {
  const stats = targetStats([reply('b', 1, 1), reply('a', 1, 1), reply('a', 2, 1)])
  assert.deepEqual(
    stats.map((entry) => entry.target),
    ['b', 'a'],
  )
})

test('seriesFor returns only that target, oldest first, capped to the window', () => {
  const samples: Sample[] = []
  for (let round = 1; round <= 400; round++) {
    samples.push(reply('a', round, round))
    samples.push(reply('b', round, 1))
  }

  const series = seriesFor(samples, 'a', 300)
  assert.equal(series.length, 300)
  assert.equal(series[0].round, 101)
  assert.equal(series[299].round, 400)
  assert.ok(series.every((sample) => sample.target === 'a'))
})

test('seriesFor returns everything when there is less than a windowful', () => {
  const series = seriesFor([reply('a', 1, 1), reply('b', 1, 1)], 'a', 300)
  assert.equal(series.length, 1)
  assert.equal(series[0].target, 'a')
})

test('dropRows returns only failures, most recent last, capped', () => {
  const samples: Sample[] = []
  for (let round = 1; round <= 600; round++) {
    samples.push(loss('a', round))
    samples.push(reply('b', round, 1))
  }

  const drops = dropRows(samples, 500)
  assert.equal(drops.length, 500)
  assert.equal(drops[0].round, 101)
  assert.equal(drops[499].round, 600)
  assert.ok(drops.every((sample) => sample.ok === false))
})

test('dropRows is empty while everything answers', () => {
  assert.deepEqual(dropRows([reply('a', 1, 1), reply('a', 2, 1)], 500), [])
})

test('formatClock pads to HH:MM:SS', () => {
  assert.equal(formatClock(new Date(2024, 0, 2, 3, 4, 5).getTime()), '03:04:05')
  assert.equal(formatClock(new Date(2024, 0, 2, 23, 59, 9).getTime()), '23:59:09')
  assert.equal(formatClock(new Date(2024, 0, 2, 0, 0, 0).getTime()), '00:00:00')
})
