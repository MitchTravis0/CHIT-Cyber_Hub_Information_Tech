import test from 'node:test'
import assert from 'node:assert/strict'
import {
  fileFacts,
  gutterLabel,
  levelClass,
  levelTag,
  splitMatch,
  visibleText,
  windowLabel,
} from '../src/tools/log-viewer/lines.ts'
import type { Chunk, Line } from '../src/tools/log-viewer/api.ts'

function line(over: Partial<Line> = {}): Line {
  return { number: 0, offset: 0, text: '', level: '', truncated: false, ...over }
}

test('levelTag is a three-letter word, so colour is never the only signal', () => {
  assert.equal(levelTag('error'), 'ERR')
  assert.equal(levelTag('warn'), 'WRN')
  assert.equal(levelTag('info'), 'INF')
  assert.equal(levelTag(''), '')
  assert.equal(levelTag('something-else'), '')
})

test('levelClass uses theme tokens only', () => {
  for (const level of ['error', 'warn', 'info', '']) {
    const cls = levelClass(level)
    assert.ok(cls.startsWith('text-'), cls)
    assert.ok(!/#[0-9a-f]{3,6}/i.test(cls), `raw hex colour in ${cls}`)
    assert.ok(!/gray|green|red|amber/.test(cls), `non-token colour in ${cls}`)
  }
})

test('gutterLabel shows a line number, or a byte offset when there is none', () => {
  assert.equal(gutterLabel(line({ number: 1234, offset: 99 })), '1234')
  assert.equal(gutterLabel(line({ number: 0, offset: 398124032 })), '@398124032')
  assert.equal(gutterLabel(line({ number: 1, offset: 0 })), '1')
})

test('splitMatch cuts a line into before, match and after', () => {
  assert.deepEqual(splitMatch('target here', 0, 6), ['', 'target', ' here'])
  assert.deepEqual(splitMatch('abc target def', 4, 6), ['abc ', 'target', ' def'])
  assert.deepEqual(splitMatch('abc target', 4, 6), ['abc ', 'target', ''])
})

test('splitMatch uses the same byte offsets Go counted', () => {
  // "café " is 6 bytes: c a f + two bytes for é + a space. Go reports col 6.
  assert.deepEqual(splitMatch('café target', 6, 6), ['café ', 'target', ''])
  // The fire emoji is 4 bytes, plus a space, so Go reports col 5.
  assert.deepEqual(splitMatch('🔥 target', 5, 6), ['🔥 ', 'target', ''])
})

test('splitMatch clamps an offset that cannot be right', () => {
  assert.deepEqual(splitMatch('short', 99, 6), ['short', '', ''])
  assert.deepEqual(splitMatch('short', -5, 2), ['', 'sh', 'ort'])
  assert.deepEqual(splitMatch('short', 3, 99), ['sho', 'rt', ''])
})

test('visibleText copies the text and nothing else', () => {
  const lines = [
    line({ number: 1, text: 'first', level: 'error' }),
    line({ number: 2, text: 'second' }),
  ]
  assert.equal(visibleText(lines), 'first\nsecond')
  assert.ok(!visibleText(lines).includes('ERR'), 'the level tag must not be copied')
  assert.ok(!visibleText(lines).includes('1'), 'the gutter must not be copied')
  assert.equal(visibleText([]), '')
})

test('visibleText does not add a trailing newline', () => {
  assert.equal(visibleText([line({ text: 'only' })]), 'only')
})

function chunk(over: Partial<Chunk> = {}): Chunk {
  return { lines: [], start: 0, end: 0, bytes: 0, atStart: true, atEnd: true, shrank: false, ...over }
}

test('windowLabel says which part of the file is on screen', () => {
  assert.equal(
    windowLabel(
      chunk({
        lines: Array.from({ length: 500 }, () => line()),
        start: 398124032,
        end: 398168904,
        bytes: 398458880,
      }),
    ),
    'Showing 500 lines, bytes 398,124,032 to 398,168,904 of 398,458,880',
  )
})

test('windowLabel uses the singular for one line', () => {
  assert.equal(
    windowLabel(chunk({ lines: [line()], start: 0, end: 6, bytes: 6 })),
    'Showing 1 line, bytes 0 to 6 of 6',
  )
})

test('windowLabel says so when there is nothing to show', () => {
  assert.equal(windowLabel(chunk()), 'No lines to show.')
  assert.equal(windowLabel(chunk({ lines: null })), 'No lines to show.')
})

test('fileFacts reads as a sentence', () => {
  const facts = fileFacts('setupact.log', 398458880, '2026-07-27T14:02:00Z', true)
  assert.ok(facts.startsWith('setupact.log, 380 MB, changed '), facts)
  assert.ok(facts.endsWith(', Windows line endings'), facts)
})

test('fileFacts leaves the line-ending note off a unix file', () => {
  const facts = fileFacts('syslog', 1024, '2026-07-27T14:02:00Z', false)
  assert.ok(!facts.includes('Windows'), facts)
  assert.ok(facts.startsWith('syslog, 1 KB, changed '), facts)
})

test('fileFacts survives a timestamp it cannot read', () => {
  assert.equal(fileFacts('a.log', 0, 'not a date', false), 'a.log, 0 B')
})
