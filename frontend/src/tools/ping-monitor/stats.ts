// Everything the cards, the chart and the drop log read is derived here from
// the streamed samples, so the numbers a tech puts in a ticket are testable
// without a DOM.
import type { Sample } from './api'

export interface TargetStats {
  target: string
  ip: string
  sent: number
  received: number
  lost: number
  /** 0 to 100, rounded to one decimal. */
  lossPct: number
  minMs: number
  avgMs: number
  maxMs: number
  /** -1 when the most recent sample was a loss. */
  lastMs: number
  jitterMs: number
  /** The longest run of consecutive losses seen so far. */
  longestOutage: number
  /** The most recent sample answered. */
  up: boolean
}

interface Accumulator extends TargetStats {
  total: number
  jitterTotal: number
  jitterCount: number
  previousMs: number
  outage: number
}

function round1(value: number): number {
  return Math.round(value * 10) / 10
}

/** One entry per target, in the order the targets first appear in the stream. */
export function targetStats(samples: Sample[]): TargetStats[] {
  const order: string[] = []
  const byTarget = new Map<string, Accumulator>()

  for (const sample of samples) {
    let acc = byTarget.get(sample.target)
    if (acc === undefined) {
      acc = {
        target: sample.target,
        ip: sample.ip,
        sent: 0,
        received: 0,
        lost: 0,
        lossPct: 0,
        minMs: 0,
        avgMs: 0,
        maxMs: 0,
        lastMs: -1,
        jitterMs: 0,
        longestOutage: 0,
        up: false,
        total: 0,
        jitterTotal: 0,
        jitterCount: 0,
        previousMs: -1,
        outage: 0,
      }
      byTarget.set(sample.target, acc)
      order.push(sample.target)
    }

    if (sample.ip !== '') acc.ip = sample.ip
    acc.sent++

    if (!sample.ok) {
      acc.lost++
      acc.outage++
      if (acc.outage > acc.longestOutage) acc.longestOutage = acc.outage
      acc.lastMs = -1
      acc.up = false
      continue
    }

    acc.outage = 0
    acc.received++
    acc.total += sample.latencyMs
    acc.lastMs = sample.latencyMs
    acc.up = true
    if (acc.received === 1 || sample.latencyMs < acc.minMs) acc.minMs = sample.latencyMs
    if (sample.latencyMs > acc.maxMs) acc.maxMs = sample.latencyMs
    if (acc.previousMs >= 0) {
      acc.jitterTotal += Math.abs(sample.latencyMs - acc.previousMs)
      acc.jitterCount++
    }
    acc.previousMs = sample.latencyMs
  }

  return order.map((target) => {
    const acc = byTarget.get(target) as Accumulator
    return {
      target: acc.target,
      ip: acc.ip,
      sent: acc.sent,
      received: acc.received,
      lost: acc.lost,
      lossPct: acc.sent === 0 ? 0 : round1((acc.lost / acc.sent) * 100),
      minMs: acc.received === 0 ? 0 : round1(acc.minMs),
      avgMs: acc.received === 0 ? 0 : round1(acc.total / acc.received),
      maxMs: acc.received === 0 ? 0 : round1(acc.maxMs),
      lastMs: acc.lastMs,
      jitterMs: acc.jitterCount === 0 ? 0 : round1(acc.jitterTotal / acc.jitterCount),
      longestOutage: acc.longestOutage,
      up: acc.up,
    }
  })
}

/** The last `window` samples for one target, oldest first. */
export function seriesFor(samples: Sample[], target: string, window: number): Sample[] {
  const mine = samples.filter((sample) => sample.target === target)
  return mine.length > window ? mine.slice(mine.length - window) : mine
}

/** The most recent `limit` failed samples, oldest first. */
export function dropRows(samples: Sample[], limit: number): Sample[] {
  const drops = samples.filter((sample) => !sample.ok)
  return drops.length > limit ? drops.slice(drops.length - limit) : drops
}

/** Local clock time as HH:MM:SS, zero padded. */
export function formatClock(at: number): string {
  const when = new Date(at)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${pad(when.getHours())}:${pad(when.getMinutes())}:${pad(when.getSeconds())}`
}
