import test from 'node:test'
import assert from 'node:assert/strict'
import {
  crumbLabel,
  crumbs,
  csvBase,
  sharePct,
  squarify,
  type Rect,
} from '../src/tools/disk-visualizer/layout.ts'
import { largestFrom } from '../src/tools/disk-visualizer/api.ts'

const SPACE: Rect = { x: 0, y: 0, w: 100, h: 62 }

function sizesOf(n: number): number[] {
  // A deliberately lumpy distribution: one giant, a middle, and a long tail.
  const out: number[] = []
  for (let i = 0; i < n; i++) out.push(Math.round(1000 / (i + 1)))
  return out
}

function layout(sizes: number[], rect: Rect = SPACE) {
  return squarify(sizes, (s) => s, rect)
}

function area(r: Rect): number {
  return r.w * r.h
}

function overlaps(a: Rect, b: Rect): boolean {
  const eps = 1e-9
  return (
    a.x + a.w > b.x + eps &&
    b.x + b.w > a.x + eps &&
    a.y + a.h > b.y + eps &&
    b.y + b.h > a.y + eps
  )
}

test('squarify covers the whole rectangle', () => {
  for (const n of [1, 2, 7, 40]) {
    const tiles = layout(sizesOf(n))
    const covered = tiles.reduce((sum, t) => sum + area(t.rect), 0)
    assert.ok(
      Math.abs(covered - area(SPACE)) < 0.5,
      `${n} items covered ${covered}, want ${area(SPACE)}`,
    )
  }
})

/**
 * The property the whole treemap rests on, and the one the coverage test cannot
 * see: a tile's share of the area must equal its item's share of the total.
 *
 * Coverage alone is not enough because the last row is stretched to fill
 * whatever is left, so a wrong scale hides inside it: the rectangle stays
 * exactly covered while every tile is the wrong size. Three separate mutations
 * survived the coverage test and died here.
 */
test('every tile area is proportional to its item size', () => {
  for (const sizes of [sizesOf(7), sizesOf(20), [500, 300, 200], [1000, 1, 1, 1]]) {
    const tiles = layout(sizes)
    const totalSize = sizes.reduce((a, b) => a + b, 0)
    const totalArea = area(SPACE)
    tiles.forEach((tile, i) => {
      const wantShare = sizes[i] / totalSize
      const gotShare = area(tile.rect) / totalArea
      assert.ok(
        Math.abs(gotShare - wantShare) < 0.005,
        `item ${i} of ${sizes.length} is ${(gotShare * 100).toFixed(2)}% of the area, ` +
          `want ${(wantShare * 100).toFixed(2)}%`,
      )
    })
  }
})

test('two items of equal size get equal areas', () => {
  const tiles = layout([100, 100, 100, 100])
  const areas = tiles.map((t) => area(t.rect))
  for (const a of areas) {
    assert.ok(Math.abs(a - areas[0]) < 0.5, `equal sizes gave areas ${areas.join(', ')}`)
  }
})

test('an item twice the size gets twice the area', () => {
  const tiles = layout([200, 100])
  assert.ok(
    Math.abs(area(tiles[0].rect) / area(tiles[1].rect) - 2) < 0.02,
    `ratio was ${area(tiles[0].rect) / area(tiles[1].rect)}, want 2`,
  )
})

test('squarify produces no overlapping tiles', () => {
  const tiles = layout(sizesOf(12))
  for (let i = 0; i < tiles.length; i++) {
    for (let j = i + 1; j < tiles.length; j++) {
      assert.ok(
        !overlaps(tiles[i].rect, tiles[j].rect),
        `tile ${i} ${JSON.stringify(tiles[i].rect)} overlaps ${j} ${JSON.stringify(tiles[j].rect)}`,
      )
    }
  }
})

test('every tile lies inside the parent rectangle', () => {
  const rect: Rect = { x: 5, y: 7, w: 100, h: 62 }
  for (const t of layout(sizesOf(20), rect)) {
    assert.ok(t.rect.x >= rect.x - 1e-9, `x ${t.rect.x} < ${rect.x}`)
    assert.ok(t.rect.y >= rect.y - 1e-9, `y ${t.rect.y} < ${rect.y}`)
    assert.ok(t.rect.x + t.rect.w <= rect.x + rect.w + 1e-9, 'right edge escapes')
    assert.ok(t.rect.y + t.rect.h <= rect.y + rect.h + 1e-9, 'bottom edge escapes')
  }
})

test('squarify keeps the input order so a caller can zip tiles back to items', () => {
  const items = ['a', 'b', 'c', 'd']
  const tiles = squarify(items, (s) => ({ a: 400, b: 300, c: 200, d: 100 })[s] ?? 0, SPACE)
  assert.deepEqual(
    tiles.map((t) => t.item),
    items,
  )
})

test('squarify gives bigger items bigger tiles', () => {
  const tiles = layout([800, 100, 100])
  assert.ok(
    area(tiles[0].rect) > area(tiles[1].rect),
    'the 800 tile is not larger than the 100 tile',
  )
})

test('squarify handles the edge cases without dividing by zero', () => {
  assert.deepEqual(squarify([], (n: number) => n, SPACE), [])

  const one = layout([500])
  assert.equal(one.length, 1)
  assert.ok(Math.abs(area(one[0].rect) - area(SPACE)) < 0.5)

  const zeros = layout([0, 0, 0])
  assert.equal(zeros.length, 3)
  for (const t of zeros) assert.equal(area(t.rect), 0)

  const mixed = layout([100, 0, 50])
  assert.equal(area(mixed[1].rect), 0, 'a zero-size item must get a zero-area tile')
  const covered = mixed.reduce((sum, t) => sum + area(t.rect), 0)
  assert.ok(Math.abs(covered - area(SPACE)) < 0.5, 'a zero item must not lose coverage')
})

test('squarify tolerates a rectangle with no area', () => {
  const tiles = squarify([1, 2], (n) => n, { x: 0, y: 0, w: 0, h: 10 })
  assert.equal(tiles.length, 2)
  for (const t of tiles) assert.equal(area(t.rect), 0)
})

test('sharePct rounds to one decimal', () => {
  assert.equal(sharePct(50, 200), 25)
  assert.equal(sharePct(1, 3), 33.3)
  assert.equal(sharePct(2, 3), 66.7)
  assert.equal(sharePct(5, 0), 0)
  assert.equal(sharePct(0, 100), 0)
  assert.equal(sharePct(100, 100), 100)
})

test('crumbs walks a unix path from the root down', () => {
  assert.deepEqual(crumbs('/home/me/x'), ['/', '/home', '/home/me', '/home/me/x'])
  assert.deepEqual(crumbs('/'), ['/'])
  assert.deepEqual(crumbs('/home'), ['/', '/home'])
  assert.deepEqual(crumbs('/home/me/'), ['/', '/home', '/home/me'])
  assert.deepEqual(crumbs(''), [])
})

test('crumbs walks a windows path from the drive down', () => {
  assert.deepEqual(crumbs('C:\\Users\\me'), ['C:\\', 'C:\\Users', 'C:\\Users\\me'])
  assert.deepEqual(crumbs('C:\\'), ['C:\\'])
  assert.deepEqual(crumbs('D:/Data/logs'), ['D:\\', 'D:\\Data', 'D:\\Data\\logs'])
})

test('crumbLabel names the last part, or the root itself', () => {
  assert.equal(crumbLabel('/home/me/x'), 'x')
  assert.equal(crumbLabel('/'), '/')
  assert.equal(crumbLabel('C:\\'), 'C:')
  assert.equal(crumbLabel('C:\\Users\\me'), 'me')
})

test('csvBase makes a safe file name from the scanned folder', () => {
  assert.equal(csvBase('/home/me/My Files'), 'disk-My-Files')
  assert.equal(csvBase('/'), 'disk-root')
  assert.equal(csvBase('C:\\Users\\jsmith'), 'disk-jsmith')
  assert.equal(csvBase('/var/log'), 'disk-log')
})

test('largestFrom reads the summary defensively', () => {
  assert.deepEqual(largestFrom(undefined), [])
  assert.deepEqual(largestFrom({}), [])
  assert.deepEqual(largestFrom({ largest: 'nonsense' }), [])
  assert.deepEqual(largestFrom({ largest: [] }), [])

  assert.deepEqual(
    largestFrom({
      largest: [
        { path: '/a/big.iso', name: 'big.iso', bytes: 900 },
        { path: '/a/no-name', bytes: 5 },
        { path: '/a/no-bytes', name: 'x' },
        null,
        'junk',
      ],
    }),
    [
      { path: '/a/big.iso', name: 'big.iso', bytes: 900 },
      { path: '/a/no-name', name: '/a/no-name', bytes: 5 },
    ],
  )
})
