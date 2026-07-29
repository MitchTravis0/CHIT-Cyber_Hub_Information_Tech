// The speed test graph and its labels are drawn in the frontend from the
// samples the backend streams, so this is the arithmetic behind the picture a
// tech reads.
import test from 'node:test'
import assert from 'node:assert/strict'
import { formatMbps, niceMax, polylinePoints, toMBps } from '../src/tools/speed-test/chart.ts'

test('niceMax rounds a peak up to a friendly axis top', () => {
  const cases: Array<[number, number]> = [
    [0, 10],
    [7, 10],
    [10, 10],
    [12, 20],
    [43, 50],
    [96, 100],
    [187, 200],
    [940, 1000],
  ]
  for (const [peak, want] of cases) {
    assert.equal(niceMax(peak), want, `niceMax(${peak})`)
  }
})

test('polylinePoints returns nothing for an empty series', () => {
  assert.equal(polylinePoints([], 600, 120, 100), '')
})

test('polylinePoints puts a single value at x = 0', () => {
  assert.equal(polylinePoints([50], 600, 120, 100), '0.00,60.00')
})

test('polylinePoints spreads three values across the width', () => {
  const points = polylinePoints([0, 50, 100], 600, 120, 100)
  const xs = points.split(' ').map((pair) => pair.split(',')[0])
  assert.deepEqual(xs, ['0.00', '300.00', '600.00'])
})

test('polylinePoints clamps a value above the axis top to the ceiling', () => {
  assert.equal(polylinePoints([250], 600, 120, 100), '0.00,0.00')
})

test('polylinePoints clamps a negative value to the floor', () => {
  assert.equal(polylinePoints([-5], 600, 120, 100), '0.00,120.00')
})

test('formatMbps keeps two decimals below 10, one below 100 and none above', () => {
  assert.equal(formatMbps(0.5), '0.50')
  assert.equal(formatMbps(8.25), '8.25')
  assert.equal(formatMbps(94.7), '94.7')
  assert.equal(formatMbps(943.2), '943')
})

test('toMBps turns megabits into the megabytes a browser shows', () => {
  assert.equal(toMBps(100), 12.5)
  assert.equal(toMBps(0), 0)
})
