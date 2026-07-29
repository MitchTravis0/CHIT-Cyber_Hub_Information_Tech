import test from 'node:test'
import assert from 'node:assert/strict'
import {
  csvBase,
  mergeGroups,
  modifiedLabel,
  summaryLine,
  toRows,
  totals,
} from '../src/tools/duplicate-finder/groups.ts'
import type { Group } from '../src/tools/duplicate-finder/api.ts'

function group(hash: string, bytes: number, paths: string[]): Group {
  return {
    hash,
    bytes,
    count: paths.length,
    waste: bytes * (paths.length - 1),
    files: paths.map((path) => ({
      path,
      name: path.split('/').pop() ?? path,
      modified: '2026-07-27T10:00:00Z',
    })),
  }
}

const BIG = group('aaa', 2_000_000_000, ['/a/big1.mkv', '/b/big2.mkv', '/c/big3.mkv'])
const MID = group('bbb', 1_000_000, ['/a/mid1.pdf', '/b/mid2.pdf'])
const SMALL = group('ccc', 1_000, ['/a/s1.txt', '/b/s2.txt'])

test('mergeGroups dedupes by hash and orders by waste', () => {
  const merged = mergeGroups([SMALL, BIG, MID, BIG])
  assert.deepEqual(
    merged.map((g) => g.hash),
    ['aaa', 'bbb', 'ccc'],
  )
  assert.equal(merged.length, 3, 'the same group arriving twice must appear once')
})

test('mergeGroups breaks a waste tie by file size', () => {
  const a = group('a', 100, ['/x', '/y']) // waste 100
  const b = group('b', 50, ['/p', '/q', '/r']) // waste 100 too
  const merged = mergeGroups([a, b])
  assert.deepEqual(
    merged.map((g) => g.hash),
    ['a', 'b'],
    'with equal waste, the bigger file comes first',
  )
})

test('mergeGroups on nothing is an empty list', () => {
  assert.deepEqual(mergeGroups([]), [])
})

test('totals counts extra copies, not files', () => {
  const t = totals([BIG, MID, SMALL])
  assert.equal(t.groups, 3)
  // 2 extras from the group of 3, 1 from each group of 2.
  assert.equal(t.copies, 4)
  assert.equal(t.waste, 2_000_000_000 * 2 + 1_000_000 + 1_000)
})

test('totals of nothing is all zeros', () => {
  assert.deepEqual(totals([]), { groups: 0, copies: 0, waste: 0 })
})

test('toRows flattens to one row per file, numbered from one', () => {
  const rows = toRows(mergeGroups([SMALL, BIG]))
  assert.equal(rows.length, 5)
  assert.deepEqual(
    rows.map((r) => r.group),
    [1, 1, 1, 2, 2],
  )
  assert.equal(rows[0].path, '/a/big1.mkv')
  assert.equal(rows[0].count, 3, 'the copy count is carried onto every row of the group')
  assert.equal(rows[0].name, 'big1.mkv')
  assert.equal(rows[4].group, 2)
})

test('toRows tolerates a null files array from a Go nil slice', () => {
  const rows = toRows([{ hash: 'x', bytes: 1, count: 0, waste: 0, files: null }])
  assert.deepEqual(rows, [])
})

test('summaryLine reads as a sentence', () => {
  assert.equal(
    summaryLine({ groups: 41, copies: 96, waste: 30_400_000_000 }, 12_043, 104_000, false),
    '41 groups of identical files, 96 copies you could delete, 28 GB wasted. Looked at 12,043 files in 1 m 44 s.',
  )
})

test('summaryLine keeps a decimal where formatBytes gives one', () => {
  assert.equal(
    summaryLine({ groups: 2, copies: 2, waste: 9_000_000_000 }, 10, 1000, false),
    '2 groups of identical files, 2 copies you could delete, 8.4 GB wasted. Looked at 10 files in 1.0 s.',
  )
})

test('summaryLine uses the singular forms', () => {
  assert.equal(
    summaryLine({ groups: 1, copies: 1, waste: 1024 }, 1, 500, false),
    '1 group of identical files, 1 copy you could delete, 1 KB wasted. Looked at 1 file in 500 ms.',
  )
})

test('summaryLine says when a scan was stopped early', () => {
  const line = summaryLine({ groups: 2, copies: 2, waste: 2048 }, 900, 3000, true)
  assert.ok(line.endsWith('(stopped early).'), line)
})

test('summaryLine says so plainly when nothing was found', () => {
  assert.equal(
    summaryLine({ groups: 0, copies: 0, waste: 0 }, 500, 2000, false),
    'No identical files found. Looked at 500 files in 2.0 s.',
  )
})

test('csvBase makes a safe file name', () => {
  assert.equal(csvBase('/home/me/My Photos'), 'duplicates-My-Photos')
  assert.equal(csvBase('C:\\Users\\jsmith'), 'duplicates-jsmith')
  assert.equal(csvBase('/'), 'duplicates-root')
})

test('modifiedLabel returns nothing for an unusable timestamp', () => {
  assert.equal(modifiedLabel({ path: '/a', name: 'a', modified: 'not a date' }), '')
  assert.notEqual(modifiedLabel({ path: '/a', name: 'a', modified: '2026-07-27T10:00:00Z' }), '')
})
