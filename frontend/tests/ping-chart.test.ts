// The Ping Monitor draws its own SVG, so the axis scale and the point geometry
// are pinned here rather than trusted to a chart library.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  chartScale,
  pointX,
  pointY,
  segments,
  DASHES,
  PLOT,
  SERIES,
} from '../src/tools/ping-monitor/chart.ts'

test('chartScale handles an empty series', () => {
  assert.deepEqual(chartScale([]), { max: 10, ticks: [0, 5, 10] })
  assert.deepEqual(chartScale([0, 0]), { max: 10, ticks: [0, 5, 10] })
})

test('chartScale rounds up to the next nice value', () => {
  assert.equal(chartScale([7]).max, 10)
  assert.equal(chartScale([11]).max, 20)
  assert.equal(chartScale([26]).max, 50)
  assert.equal(chartScale([1200]).max, 1500)
  assert.equal(chartScale([5]).max, 5)
  assert.deepEqual(chartScale([11]).ticks, [0, 10, 20])
})

test('chartScale falls back to thousands above 5000', () => {
  assert.equal(chartScale([6400]).max, 7000)
  assert.equal(chartScale([5001]).max, 6000)
})

test('chartScale reads the largest value, wherever it is', () => {
  assert.equal(chartScale([3, 99, 12]).max, 100)
})

test('pointX spreads points across the plot', () => {
  assert.equal(pointX(0, 10), PLOT.left)
  assert.equal(pointX(9, 10), PLOT.width - PLOT.right)
  assert.equal(pointX(0, 1), PLOT.left)
  assert.ok(pointX(5, 10) > pointX(4, 10))
})

test('pointY puts zero on the baseline and the max at the top', () => {
  assert.equal(pointY(0, 100), PLOT.bottom)
  assert.equal(pointY(100, 100), PLOT.top)
  assert.equal(pointY(500, 100), PLOT.top)
  assert.equal(pointY(50, 100), (PLOT.bottom + PLOT.top) / 2)
})

test('segments breaks the line at every loss', () => {
  const runs = segments(
    [
      { ok: true, latencyMs: 10 },
      { ok: true, latencyMs: 20 },
      { ok: false, latencyMs: 0 },
      { ok: true, latencyMs: 30 },
    ],
    100,
  )
  assert.equal(runs.length, 2)
  assert.equal(runs[0].length, 2)
  assert.equal(runs[1].length, 1)
  assert.deepEqual(runs[0][0], [pointX(0, 4), pointY(10, 100)])
  assert.deepEqual(runs[1][0], [pointX(3, 4), pointY(30, 100)])
})

test('segments returns nothing for an all-loss series', () => {
  assert.deepEqual(
    segments(
      [
        { ok: false, latencyMs: 0 },
        { ok: false, latencyMs: 0 },
      ],
      100,
    ),
    [],
  )
  assert.deepEqual(segments([], 100), [])
})

test('every series has its own colour and dash pattern', () => {
  assert.equal(SERIES.length, 4)
  assert.equal(DASHES.length, 4)
  assert.equal(new Set(SERIES).size, 4)
  assert.equal(new Set(DASHES).size, 4)
})
