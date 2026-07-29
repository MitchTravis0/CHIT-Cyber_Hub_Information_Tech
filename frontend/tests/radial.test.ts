import test from 'node:test'
import assert from 'node:assert/strict'
import { arcPath, layoutRing, polarToXY } from '../src/shell/radial.ts'

// 0 degrees points straight up, angles run clockwise, which is how the ring is
// read on screen. Everything below is checked against that convention.

test('polarToXY puts 0 degrees at the top and runs clockwise', () => {
  assert.deepEqual(polarToXY(240, 240, 100, 0), { x: 240, y: 140 })
  assert.deepEqual(polarToXY(240, 240, 100, 90), { x: 340, y: 240 })
  assert.deepEqual(polarToXY(240, 240, 100, 180), { x: 240, y: 340 })
  assert.deepEqual(polarToXY(240, 240, 100, 270), { x: 140, y: 240 })
})

test('polarToXY rounds to two decimals and wraps past a full turn', () => {
  assert.deepEqual(polarToXY(0, 0, 10, 45), { x: 7.07, y: -7.07 })
  assert.deepEqual(polarToXY(240, 240, 100, 360), { x: 240, y: 140 })
  assert.deepEqual(polarToXY(240, 240, 100, -90), { x: 140, y: 240 })
})

test('polarToXY at radius zero is the centre whatever the angle', () => {
  assert.deepEqual(polarToXY(240, 240, 0, 137), { x: 240, y: 240 })
})

test('layoutRing splits the circle evenly and leaves a gap between segments', () => {
  const ring = layoutRing(4)
  assert.equal(ring.length, 4)
  // 360 / 4 = 90 per slot, with a 2 degree gap taken off the end of each.
  assert.deepEqual(ring[0], { startDeg: 0, endDeg: 88, midDeg: 44 })
  assert.deepEqual(ring[1], { startDeg: 90, endDeg: 178, midDeg: 134 })
  assert.deepEqual(ring[2], { startDeg: 180, endDeg: 268, midDeg: 224 })
  assert.deepEqual(ring[3], { startDeg: 270, endDeg: 358, midDeg: 314 })
})

test('layoutRing honours a start offset and a custom gap', () => {
  const ring = layoutRing(2, { startDeg: -90, gapDeg: 10 })
  assert.deepEqual(ring[0], { startDeg: -90, endDeg: 80, midDeg: -5 })
  assert.deepEqual(ring[1], { startDeg: 90, endDeg: 260, midDeg: 175 })
})

test('layoutRing gives a single item a nearly closed ring, not a zero sweep', () => {
  const ring = layoutRing(1)
  assert.equal(ring.length, 1)
  assert.equal(ring[0].startDeg, 0)
  assert.equal(ring[0].endDeg, 359.99)
  assert.equal(ring[0].midDeg, 180)
})

test('layoutRing returns nothing for an empty ring', () => {
  assert.deepEqual(layoutRing(0), [])
})

test('layoutRing covers the seven categories that ship today', () => {
  const ring = layoutRing(7)
  assert.equal(ring.length, 7)
  // 360 / 7 is not a whole number, so the slots must still tile without a hole.
  assert.equal(ring[0].startDeg, 0)
  assert.equal(ring[6].startDeg, 308.57)
  // Every sweep is wide enough to be an easy click target.
  for (const segment of ring) {
    assert.ok(segment.endDeg - segment.startDeg > 30, `sweep too narrow: ${JSON.stringify(segment)}`)
  }
})

test('arcPath draws a donut segment as move, arc, line, arc, close', () => {
  // A quarter turn starting at the top: outer edge from 12 o'clock to 3 o'clock.
  const d = arcPath(240, 240, 120, 200, 0, 90)
  assert.equal(d, 'M 240 40 A 200 200 0 0 1 440 240 L 360 240 A 120 120 0 0 0 240 120 Z')
})

test('arcPath sets the large-arc flag once the sweep passes half a turn', () => {
  const small = arcPath(0, 0, 50, 100, 0, 180)
  const large = arcPath(0, 0, 50, 100, 0, 181)
  // The flag is the fourth number in the "A rx ry rot large sweep x y" run.
  assert.ok(small.includes('A 100 100 0 0 1'), small)
  assert.ok(large.includes('A 100 100 0 1 1'), large)
})

test('arcPath never emits NaN, for any segment of any ring size', () => {
  for (let count = 1; count <= 10; count++) {
    for (const segment of layoutRing(count)) {
      const d = arcPath(240, 240, 120, 200, segment.startDeg, segment.endDeg)
      assert.ok(!d.includes('NaN'), `count ${count} produced ${d}`)
    }
  }
})
