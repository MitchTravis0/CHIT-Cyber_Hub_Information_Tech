import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buttonName,
  coverageLine,
  describeKey,
  locationName,
  logLine,
  logText,
  modifierList,
  stuckModifiers,
  stuckSentence,
  wheelLabel,
  type KeyboardEventLike,
  type LogEntry,
} from '../src/tools/keyboard-tester/events.ts'
import { allCodes } from '../src/tools/keyboard-tester/layout.ts'

function keyEvent(over: Partial<KeyboardEventLike> = {}): KeyboardEventLike {
  return {
    code: 'KeyE',
    key: 'e',
    keyCode: 69,
    location: 0,
    repeat: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    metaKey: false,
    ...over,
  }
}

test('describeKey reports what the panel shows', () => {
  assert.deepEqual(describeKey(keyEvent()), {
    code: 'KeyE',
    key: 'e',
    keyCode: 69,
    location: 'standard',
    repeat: false,
    modifiers: [],
  })
})

test('describeKey carries the modifiers and the repeat flag', () => {
  const shifted = describeKey(keyEvent({ key: 'E', shiftKey: true }))
  assert.deepEqual(shifted.modifiers, ['Shift'])
  assert.equal(shifted.key, 'E')

  const repeated = describeKey(keyEvent({ repeat: true }))
  assert.equal(repeated.repeat, true)
})

test('describeKey reports a numpad key as numpad', () => {
  const pad = describeKey(keyEvent({ code: 'Numpad7', key: '7', keyCode: 103, location: 3 }))
  assert.equal(pad.location, 'numpad')
  assert.equal(pad.code, 'Numpad7')
})

test('describeKey shows the code even when the character does not match it', () => {
  // A German layout produces "y" from the key whose code is KeyZ. That is a
  // layout problem, not a broken key, and the panel is what tells them apart.
  const german = describeKey(keyEvent({ code: 'KeyZ', key: 'y', keyCode: 90 }))
  assert.equal(german.code, 'KeyZ')
  assert.equal(german.key, 'y')
})

test('locationName covers every value and falls back safely', () => {
  assert.equal(locationName(0), 'standard')
  assert.equal(locationName(1), 'left')
  assert.equal(locationName(2), 'right')
  assert.equal(locationName(3), 'numpad')
  assert.equal(locationName(9), 'standard')
  assert.equal(locationName(-1), 'standard')
})

test('modifierList uses a fixed order', () => {
  assert.deepEqual(
    modifierList({ ctrlKey: true, altKey: true, shiftKey: true, metaKey: true }),
    ['Ctrl', 'Alt', 'Shift', 'Meta'],
  )
  assert.deepEqual(modifierList({ ctrlKey: false, altKey: false, shiftKey: false, metaKey: false }), [])
  assert.deepEqual(
    modifierList({ ctrlKey: false, altKey: true, shiftKey: false, metaKey: true }),
    ['Alt', 'Meta'],
  )
})

const CODES = allCodes()

test('coverageLine counts down to zero', () => {
  assert.equal(coverageLine(new Set(), CODES, 104), '0 of 104 keys seen. 104 to go.')
  assert.equal(
    coverageLine(new Set(CODES.slice(0, 82)), CODES, 104),
    '82 of 104 keys seen. 22 to go.',
  )
  assert.equal(
    coverageLine(new Set(CODES.slice(0, 103)), CODES, 104),
    '103 of 104 keys seen. 1 to go.',
  )
})

test('coverageLine says so when every key has reported', () => {
  assert.equal(
    coverageLine(new Set(CODES), CODES, 104),
    'Every key on this keyboard reported. Nothing is stuck.',
  )
})

test('a key that is not on the drawn board does not push the count over the total', () => {
  // An ISO keyboard has IntlBackslash, which the ANSI picture has no cell for.
  const seen = new Set([...CODES.slice(0, 10), 'IntlBackslash', 'Fn', 'Lang1'])
  assert.equal(coverageLine(seen, CODES, 104), '10 of 104 keys seen. 94 to go.')
})

function entry(over: Partial<LogEntry> = {}): LogEntry {
  return { kind: 'keydown', code: 'KeyE', detail: '"e"', atMs: 1000, ...over }
}

test('logLine shows the gap since the event before it', () => {
  assert.equal(logLine(entry(), null), '+0 ms  keydown  KeyE  "e"')
  assert.equal(logLine(entry({ atMs: 1002 }), 1000), '+2 ms  keydown  KeyE  "e"')
  assert.equal(
    logLine({ kind: 'mousedown', code: 'Left', detail: '', atMs: 1015 }, 1000),
    '+15 ms  mousedown  Left',
  )
  assert.equal(
    logLine({ kind: 'wheel', code: 'wheel', detail: 'y -120', atMs: 1003 }, 1000),
    '+3 ms  wheel  wheel  y -120',
  )
})

test('logLine never shows a negative gap', () => {
  assert.equal(logLine(entry({ atMs: 900 }), 1000), '+0 ms  keydown  KeyE  "e"')
})

test('logText joins newest first, matching what is on screen', () => {
  // The log is newest first, so each entry's gap is measured against the one
  // below it.
  const entries = [
    entry({ code: 'KeyC', atMs: 1010 }),
    entry({ code: 'KeyB', atMs: 1004 }),
    entry({ code: 'KeyA', atMs: 1000 }),
  ]
  assert.equal(
    logText(entries),
    ['+6 ms  keydown  KeyC  "e"', '+4 ms  keydown  KeyB  "e"', '+0 ms  keydown  KeyA  "e"'].join('\n'),
  )
})

test('logText of nothing is nothing', () => {
  assert.equal(logText([]), '')
})

test('buttonName covers the five buttons and falls back readably', () => {
  assert.equal(buttonName(0), 'Left')
  assert.equal(buttonName(1), 'Middle')
  assert.equal(buttonName(2), 'Right')
  assert.equal(buttonName(3), 'Back')
  assert.equal(buttonName(4), 'Forward')
  assert.equal(buttonName(7), 'Button 8')
})

test('wheelLabel names the unit, because the number means nothing without it', () => {
  assert.equal(wheelLabel(0, -120, 0), 'x 0, y -120 (pixels)')
  assert.equal(wheelLabel(0, -3, 1), 'x 0, y -3 (lines)')
  assert.equal(wheelLabel(0, -1, 2), 'x 0, y -1 (pages)')
  assert.equal(wheelLabel(0, -1, 9), 'x 0, y -1 (units)')
  assert.equal(wheelLabel(12.7, -3.2, 0), 'x 13, y -3 (pixels)')
})

test('stuckModifiers fires only when nothing is physically down', () => {
  assert.deepEqual(stuckModifiers(new Set(['Shift']), false), ['Shift'])
  assert.deepEqual(stuckModifiers(new Set(['Shift']), true), [])
  assert.deepEqual(stuckModifiers(new Set(), false), [])
})

test('stuckModifiers reports several in the fixed order', () => {
  assert.deepEqual(stuckModifiers(new Set(['Shift', 'Ctrl']), false), ['Ctrl', 'Shift'])
  assert.deepEqual(
    stuckModifiers(new Set(['Meta', 'Alt', 'Shift', 'Ctrl']), false),
    ['Ctrl', 'Alt', 'Shift', 'Meta'],
  )
})

test('stuckSentence names the key and what to do about it', () => {
  assert.equal(
    stuckSentence(['Shift']),
    'Shift is reporting as held down. If nothing is pressed, check for a stuck key or turn Sticky Keys off in accessibility settings.',
  )
  assert.ok(stuckSentence(['Ctrl', 'Shift']).startsWith('Ctrl and Shift are reporting'))
  assert.ok(stuckSentence(['Ctrl', 'Alt', 'Shift']).startsWith('Ctrl, Alt and Shift are reporting'))
  assert.equal(stuckSentence([]), '')
})
