import test from 'node:test'
import assert from 'node:assert/strict'
import {
  CSS_PX_PER_INCH,
  MM_PER_INCH,
  clampToBox,
  formatInches,
  formatMm,
  lockAxis,
  measure,
  scalingLabel,
} from '../src/tools/screen-ruler/ruler.ts'

test('measure gets the geometry right', () => {
  // A 3-4-5 triangle, scaled up so the diagonal is exact.
  const m = measure({ x: 0, y: 0 }, { x: 300, y: 400 }, 1)
  assert.equal(m.width, 300)
  assert.equal(m.height, 400)
  assert.equal(m.diagonal, 500)
})

test('measure treats a backwards drag the same as a forwards one', () => {
  const forwards = measure({ x: 0, y: 0 }, { x: 300, y: 400 }, 1)
  const backwards = measure({ x: 300, y: 400 }, { x: 0, y: 0 }, 1)
  assert.deepEqual(backwards, forwards)
})

test('measure of a click with no drag is all zeros, not nothing', () => {
  const m = measure({ x: 50, y: 50 }, { x: 50, y: 50 }, 2)
  assert.equal(m.width, 0)
  assert.equal(m.height, 0)
  assert.equal(m.diagonal, 0)
  assert.equal(m.deviceWidth, 0)
  assert.equal(m.mmWidth, 0)
})

test('measure multiplies by the device pixel ratio', () => {
  assert.equal(measure({ x: 0, y: 0 }, { x: 100, y: 50 }, 1).deviceWidth, 100)
  assert.equal(measure({ x: 0, y: 0 }, { x: 100, y: 50 }, 2).deviceWidth, 200)
  assert.equal(measure({ x: 0, y: 0 }, { x: 100, y: 50 }, 2).deviceHeight, 100)
  assert.equal(measure({ x: 0, y: 0 }, { x: 100, y: 0 }, 1.5).deviceWidth, 150)
  // 10 * 1.25 is 12.5, which rounds to 13. The literal pins the rounding rule.
  assert.equal(measure({ x: 0, y: 0 }, { x: 10, y: 0 }, 1.25).deviceWidth, 13)
})

test('measure falls back to a ratio of 1 for a nonsense dpr', () => {
  for (const dpr of [0, -1, Number.NaN, Number.POSITIVE_INFINITY]) {
    assert.equal(measure({ x: 0, y: 0 }, { x: 100, y: 0 }, dpr).deviceWidth, 100, `dpr ${dpr}`)
  }
})

test('the physical constants are the published ones', () => {
  // Written as literals so neither can drift from the readout copy, which says
  // "96 pixels per inch" in as many words.
  assert.equal(CSS_PX_PER_INCH, 96)
  assert.equal(MM_PER_INCH, 25.4)
})

test('measure converts to millimetres at the CSS standard 96 dpi', () => {
  const inch = measure({ x: 0, y: 0 }, { x: 96, y: 96 }, 1)
  assert.equal(inch.mmWidth, 25.4)
  assert.equal(inch.inWidth, 1)

  const two = measure({ x: 0, y: 0 }, { x: 192, y: 0 }, 1)
  assert.equal(two.mmWidth, 50.8)
  assert.equal(two.inWidth, 2)
})

test('formatMm and formatInches always show their unit', () => {
  assert.equal(formatMm(96), '25.4 mm')
  assert.equal(formatMm(0), '0.0 mm')
  assert.equal(formatMm(1), '0.3 mm')
  assert.equal(formatInches(96), '1.00 in')
  assert.equal(formatInches(0), '0.00 in')
})

test('lockAxis flattens the shorter side', () => {
  // Mostly horizontal: the y is flattened onto the start.
  assert.deepEqual(lockAxis({ x: 0, y: 0 }, { x: 100, y: 10 }), { x: 100, y: 0 })
  // Mostly vertical: the x is flattened.
  assert.deepEqual(lockAxis({ x: 0, y: 0 }, { x: 10, y: 100 }), { x: 0, y: 100 })
})

test('lockAxis picks horizontal on an exact diagonal, which is the tie-break', () => {
  assert.deepEqual(lockAxis({ x: 0, y: 0 }, { x: 50, y: 50 }), { x: 50, y: 0 })
})

test('lockAxis works from any starting point', () => {
  assert.deepEqual(lockAxis({ x: 20, y: 30 }, { x: 25, y: 90 }), { x: 20, y: 90 })
})

test('scalingLabel is the number that settles a "everything is huge" call', () => {
  assert.equal(scalingLabel(1), '100% (no scaling)')
  assert.equal(scalingLabel(1.25), '125%')
  assert.equal(scalingLabel(1.5), '150%')
  assert.equal(scalingLabel(2), '200%')
  assert.equal(scalingLabel(3), '300%')
})

test('scalingLabel says so rather than showing a nonsense percentage', () => {
  for (const dpr of [0, -1, Number.NaN]) {
    assert.equal(scalingLabel(dpr), 'not reported', `dpr ${dpr}`)
  }
})

test('clampToBox keeps a drag inside the measuring area', () => {
  assert.deepEqual(clampToBox({ x: 50, y: 50 }, 100, 100), { x: 50, y: 50 })
  assert.deepEqual(clampToBox({ x: -10, y: 500 }, 100, 100), { x: 0, y: 100 })
  assert.deepEqual(clampToBox({ x: 200, y: -5 }, 100, 100), { x: 100, y: 0 })
})
