import test from 'node:test'
import assert from 'node:assert/strict'
import {
  changedText,
  collapse,
  CONTEXT_LINES,
  diffTexts,
  MAX_EDITS,
  MAX_LINES,
  MAX_RENDERED_ROWS,
  normalizeLine,
  splitLines,
  type DiffRow,
} from '../src/tools/text-diff/diff.ts'

const PLAIN = { ignoreWhitespace: false, ignoreCase: false }

function lines(...items: string[]): string {
  return items.join('\n')
}

function shape(rows: DiffRow[]): string[] {
  return rows.map((row) => `${row.kind} ${row.left ?? '-'}/${row.right ?? '-'} ${row.text}`)
}

test('splitLines handles every line ending', () => {
  assert.deepEqual(splitLines(''), [])
  assert.deepEqual(splitLines('a'), ['a'])
  assert.deepEqual(splitLines('a\nb'), ['a', 'b'])
  assert.deepEqual(splitLines('a\r\nb'), ['a', 'b'])
  assert.deepEqual(splitLines('a\rb'), ['a', 'b'])
  assert.deepEqual(splitLines('a\n'), ['a'])
  assert.deepEqual(splitLines('a\n\n'), ['a', ''])
  assert.deepEqual(splitLines('\n'), [''])
})

test('normalizeLine applies the rules in order', () => {
  assert.equal(normalizeLine('  a\tb  ', PLAIN), '  a\tb  ')
  assert.equal(normalizeLine('  a   b  ', { ignoreWhitespace: true, ignoreCase: false }), 'a b')
  assert.equal(normalizeLine('\ta\tb', { ignoreWhitespace: true, ignoreCase: false }), 'a b')
  assert.equal(normalizeLine('AbC', { ignoreWhitespace: false, ignoreCase: true }), 'abc')
  assert.equal(normalizeLine('  Ab   Cd ', { ignoreWhitespace: true, ignoreCase: true }), 'ab cd')
})

test('an unchanged text reports identical', () => {
  const text = lines('a', 'b', 'c')
  const result = diffTexts(text, text, PLAIN)
  assert.equal(result.identical, true)
  assert.equal(result.added, 0)
  assert.equal(result.removed, 0)
  assert.equal(result.same, 3)
  assert.equal(result.differsOnlyByRules, false)
  assert.deepEqual(shape(result.rows), ['same 1/1 a', 'same 2/2 b', 'same 3/3 c'])
})

test('differsOnlyByRules is set only when a rule did the work', () => {
  const withRule = diffTexts('A', 'a', { ignoreWhitespace: false, ignoreCase: true })
  assert.equal(withRule.identical, true)
  assert.equal(withRule.differsOnlyByRules, true)

  assert.equal(diffTexts('A', 'a', PLAIN).identical, false)
  assert.equal(diffTexts('a\nb', 'a\nb', { ignoreWhitespace: true, ignoreCase: true }).differsOnlyByRules, false)
  // A trailing newline on one side alone is not a rule doing work.
  assert.equal(diffTexts('a\n', 'a', PLAIN).differsOnlyByRules, false)
  assert.equal(diffTexts('a\r\nb', 'a\nb', PLAIN).differsOnlyByRules, false)
})

test('one inserted line at the top does not shift everything', () => {
  const result = diffTexts(lines('a', 'b', 'c', 'd', 'e'), lines('x', 'a', 'b', 'c', 'd', 'e'), PLAIN)
  assert.equal(result.added, 1)
  assert.equal(result.removed, 0)
  assert.deepEqual(shape(result.rows), [
    'added -/1 x',
    'same 1/2 a',
    'same 2/3 b',
    'same 3/4 c',
    'same 4/5 d',
    'same 5/6 e',
  ])
})

test('one deleted line in the middle', () => {
  const result = diffTexts(lines('a', 'b', 'c', 'd', 'e'), lines('a', 'b', 'd', 'e'), PLAIN)
  assert.equal(result.added, 0)
  assert.equal(result.removed, 1)
  assert.deepEqual(shape(result.rows), [
    'same 1/1 a',
    'same 2/2 b',
    'removed 3/- c',
    'same 4/3 d',
    'same 5/4 e',
  ])
})

test('a changed line is one removed plus one added', () => {
  const result = diffTexts(lines('a', 'b', 'c'), lines('a', 'x', 'c'), PLAIN)
  assert.deepEqual(shape(result.rows), [
    'same 1/1 a',
    'removed 2/- b',
    'added -/2 x',
    'same 3/3 c',
  ])
})

test('an empty side is all additions or all removals', () => {
  const added = diffTexts('', lines('a', 'b'), PLAIN)
  assert.equal(added.added, 2)
  assert.equal(added.removed, 0)
  assert.equal(added.same, 0)

  const removed = diffTexts(lines('a', 'b'), '', PLAIN)
  assert.equal(removed.removed, 2)
  assert.equal(removed.added, 0)

  const nothing = diffTexts('', '', PLAIN)
  assert.deepEqual(nothing.rows, [])
  assert.equal(nothing.identical, true)
})

test('the diff is minimal on a shuffled block', () => {
  // diff --minimal agrees: 2 changed lines each way, not 4 each way.
  const result = diffTexts(lines('a', 'b', 'c', 'd', 'e', 'f'), lines('a', 'd', 'e', 'b', 'c', 'f'), PLAIN)
  assert.equal(result.added + result.removed, 4)
  assert.equal(result.same, 4)
})

test('line numbers are 1-based and gapless per side', () => {
  const left = lines('a', 'b', 'c', 'd', 'e', 'f', 'g')
  const right = lines('a', 'x', 'c', 'd', 'y', 'z', 'g', 'h')
  const result = diffTexts(left, right, PLAIN)
  const lefts = result.rows.map((row) => row.left).filter((n): n is number => n !== null)
  const rights = result.rows.map((row) => row.right).filter((n): n is number => n !== null)
  assert.deepEqual(lefts, [1, 2, 3, 4, 5, 6, 7])
  assert.deepEqual(rights, [1, 2, 3, 4, 5, 6, 7, 8])
})

test('ignore rules change the outcome', () => {
  assert.equal(diffTexts('  server 1', '\tserver 1', PLAIN).added, 1)
  assert.equal(diffTexts('  server 1', '\tserver 1', PLAIN).removed, 1)
  assert.equal(
    diffTexts('  server 1', '\tserver 1', { ignoreWhitespace: true, ignoreCase: false }).identical,
    true,
  )
  assert.equal(diffTexts('Server', 'server', PLAIN).identical, false)
  assert.equal(
    diffTexts('Server', 'server', { ignoreWhitespace: false, ignoreCase: true }).identical,
    true,
  )
})

test('a long identical run is trimmed as prefix and suffix', () => {
  const before = Array.from({ length: 2500 }, (_, i) => `line ${i}`)
  const after = before.slice()
  after[1250] = 'changed'
  const result = diffTexts(before.join('\n'), after.join('\n'), PLAIN)
  assert.equal(result.added, 1)
  assert.equal(result.removed, 1)
  assert.equal(result.same, 2499)
})

test('truncation cuts at exactly 10,000 lines', () => {
  assert.equal(MAX_LINES, 10000)
  const long = Array.from({ length: 10001 }, (_, i) => `line ${i}`).join('\n')
  const cut = diffTexts(long, long, PLAIN)
  assert.equal(cut.truncated, true)
  assert.equal(cut.same, 10000)

  const exact = Array.from({ length: 10000 }, (_, i) => `line ${i}`).join('\n')
  const whole = diffTexts(exact, exact, PLAIN)
  assert.equal(whole.truncated, false)
  assert.equal(whole.same, 10000)
})

test('too many changes falls back honestly', () => {
  assert.equal(MAX_EDITS, 3000)
  const left = Array.from({ length: 3001 }, (_, i) => `left ${i}`).join('\n')
  const right = Array.from({ length: 3001 }, (_, i) => `right ${i}`).join('\n')
  const blunt = diffTexts(left, right, PLAIN)
  assert.equal(blunt.tooDifferent, true)
  assert.equal(blunt.removed, 3001)
  assert.equal(blunt.added, 3001)
  assert.equal(blunt.same, 0)

  const smallLeft = Array.from({ length: 1500 }, (_, i) => `left ${i}`).join('\n')
  const smallRight = Array.from({ length: 1500 }, (_, i) => `right ${i}`).join('\n')
  const fine = diffTexts(smallLeft, smallRight, PLAIN)
  assert.equal(fine.tooDifferent, false)
  assert.equal(fine.added, 1500)
  assert.equal(fine.removed, 1500)
})

function sameRows(count: number, from: number): DiffRow[] {
  return Array.from({ length: count }, (_, i) => ({
    kind: 'same' as const,
    left: from + i,
    right: from + i,
    text: `line ${from + i}`,
  }))
}

const ADDED: DiffRow = { kind: 'added', left: null, right: 99, text: 'new' }

test('collapse keeps three lines of context', () => {
  assert.equal(CONTEXT_LINES, 3)
  const rows = [...sameRows(20, 1), ADDED, ...sameRows(20, 21)]
  const blocks = collapse(rows, 3)
  assert.deepEqual(
    blocks.map((block) => `${block.kind}:${block.kind === 'gap' ? block.skipped : block.rows.length}`),
    ['gap:17', 'rows:7', 'gap:17'],
  )

  // A run between two changes keeps three either side and gaps the other 14.
  const middle = collapse([ADDED, ...sameRows(20, 1), ADDED], 3)
  assert.deepEqual(
    middle.map((block) => `${block.kind}:${block.kind === 'gap' ? block.skipped : block.rows.length}`),
    ['rows:4', 'gap:14', 'rows:4'],
  )
})

test('collapse does not fold a short run', () => {
  const six = collapse([ADDED, ...sameRows(6, 1), ADDED], 3)
  assert.deepEqual(six.map((block) => block.kind), ['rows'])
  assert.equal(six[0].rows.length, 8)

  const seven = collapse([ADDED, ...sameRows(7, 1), ADDED], 3)
  assert.deepEqual(
    seven.map((block) => `${block.kind}:${block.kind === 'gap' ? block.skipped : block.rows.length}`),
    ['rows:4', 'gap:1', 'rows:4'],
  )
})

test('collapse folds an all-same list into one gap', () => {
  const blocks = collapse(sameRows(50, 1), 3)
  assert.equal(blocks.length, 1)
  assert.equal(blocks[0].kind, 'gap')
  assert.equal(blocks[0].skipped, 50)
  assert.deepEqual(collapse([], 3), [])
})

test('collapse never puts two gaps together', () => {
  const rows: DiffRow[] = []
  for (let i = 0; i < 6; i++) {
    rows.push(...sameRows(30, i * 31 + 1))
    rows.push({ kind: 'added', left: null, right: i, text: `change ${i}` })
  }
  const blocks = collapse(rows, 3)
  for (let i = 1; i < blocks.length; i++) {
    assert.ok(!(blocks[i].kind === 'gap' && blocks[i - 1].kind === 'gap'), 'two gaps in a row')
  }
  const kept = blocks.reduce((total, block) => total + block.rows.length, 0)
  const skipped = blocks.reduce((total, block) => total + block.skipped, 0)
  assert.equal(kept + skipped, rows.length)
})

test('changedText is patch shaped', () => {
  const result = diffTexts(lines('a', 'b', 'c'), lines('a', 'x', 'c'), PLAIN)
  assert.equal(changedText(result.rows), '-b\n+x')
  assert.equal(changedText(diffTexts('a', 'a', PLAIN).rows), '')
  assert.ok(!changedText(result.rows).includes('a'))
})

test('changedText is not capped', () => {
  assert.equal(MAX_RENDERED_ROWS, 2000)
  const left = Array.from({ length: 1250 }, (_, i) => `left ${i}`).join('\n')
  const right = Array.from({ length: 1250 }, (_, i) => `right ${i}`).join('\n')
  const result = diffTexts(left, right, PLAIN)
  assert.equal(changedText(result.rows).split('\n').length, 2500)
})

test('the row text keeps the original line', () => {
  const result = diffTexts('  Server One', '  Server Two', {
    ignoreWhitespace: true,
    ignoreCase: true,
  })
  assert.equal(result.rows[0].text, '  Server One')
  assert.equal(result.rows[1].text, '  Server Two')
})
