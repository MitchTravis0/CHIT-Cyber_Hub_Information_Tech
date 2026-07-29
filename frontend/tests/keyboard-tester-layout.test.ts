import test from 'node:test'
import assert from 'node:assert/strict'
import {
  LAYOUT,
  OS_COMBINATIONS,
  allCodes,
  swallowedKeys,
} from '../src/tools/keyboard-tester/layout.ts'

test('the layout has exactly 104 keys', () => {
  // The standard ANSI board. Written as a literal so a key cannot go missing
  // without a failure.
  assert.equal(allCodes().length, 104)
})

test('every code is unique', () => {
  const codes = allCodes()
  // A duplicate would make one of the two keys impossible to ever mark seen.
  assert.equal(new Set(codes).size, codes.length)
})

test('the four commonly-wrong code names are right', () => {
  const codes = new Set(allCodes())
  // Confirmed against the WebKit engine's own string table, which is the engine
  // the Linux build actually runs on.
  for (const right of ['Backquote', 'Quote', 'ContextMenu', 'NumpadDecimal']) {
    assert.ok(codes.has(right), `${right} is missing`)
  }
  for (const wrong of ['Grave', 'Apostrophe', 'Menu', 'NumpadPeriod']) {
    assert.ok(!codes.has(wrong), `${wrong} is not a real KeyboardEvent.code`)
  }
})

test('all 26 letters are present', () => {
  const codes = new Set(allCodes())
  // Generated from the alphabet rather than pasted, so a missing letter cannot
  // hide inside a hand-written list.
  for (const letter of 'ABCDEFGHIJKLMNOPQRSTUVWXYZ') {
    assert.ok(codes.has('Key' + letter), `Key${letter} is missing`)
  }
})

test('all 10 digits are present', () => {
  const codes = new Set(allCodes())
  for (let i = 0; i <= 9; i++) {
    assert.ok(codes.has('Digit' + i), `Digit${i} is missing`)
  }
})

test('all 12 function keys are present', () => {
  const codes = new Set(allCodes())
  for (let i = 1; i <= 12; i++) {
    assert.ok(codes.has('F' + i), `F${i} is missing`)
  }
})

test('all 17 keypad keys are present', () => {
  const codes = new Set(allCodes())
  const keypad = [
    'NumLock', 'NumpadDivide', 'NumpadMultiply', 'NumpadSubtract', 'NumpadAdd',
    'NumpadEnter', 'NumpadDecimal',
    'Numpad0', 'Numpad1', 'Numpad2', 'Numpad3', 'Numpad4',
    'Numpad5', 'Numpad6', 'Numpad7', 'Numpad8', 'Numpad9',
  ]
  assert.equal(keypad.length, 17)
  for (const code of keypad) {
    assert.ok(codes.has(code), `${code} is missing`)
  }
})

test('the modifiers are present on both sides', () => {
  const codes = new Set(allCodes())
  for (const code of [
    'ShiftLeft', 'ShiftRight', 'ControlLeft', 'ControlRight',
    'AltLeft', 'AltRight', 'MetaLeft', 'MetaRight',
  ]) {
    assert.ok(codes.has(code), `${code} is missing`)
  }
})

test('every key has a positive width and a name', () => {
  for (const block of LAYOUT) {
    for (const row of block.rows) {
      for (const key of row.keys) {
        assert.ok(key.width > 0, `${key.code} has width ${key.width}`)
        assert.notEqual(key.label, '', `${key.code} has no label`)
        assert.notEqual(key.code, '', 'a key has no code')
      }
    }
  }
})

test('each of the five main typing rows sums to the same width', () => {
  const main = LAYOUT.find((block) => block.name === 'main')
  assert.ok(main !== undefined)
  // Rows 1 to 5 are the number row through the bottom row. 15 units is written
  // as a literal, so a mistyped width cannot silently make a row jut out.
  for (const row of main.rows.slice(1)) {
    const total = row.keys.reduce((sum, key) => sum + key.width, 0)
    assert.equal(
      Math.round(total * 100) / 100,
      15,
      `a row sums to ${total}: ${row.keys.map((k) => k.code).join(',')}`,
    )
  }
})

test('every swallowed key carries a reason', () => {
  const swallowed = swallowedKeys()
  assert.ok(swallowed.length > 0, 'no key is marked as swallowed at all')
  for (const key of swallowed) {
    assert.ok(
      key.swallowed !== undefined && key.swallowed.length > 20,
      `${key.code} is dashed with no explanation`,
    )
  }
})

test('the swallowed set is exactly the keys the OS is known to keep', () => {
  const codes = swallowedKeys()
    .map((key) => key.code)
    .sort()
  assert.deepEqual(codes, ['F11', 'MetaLeft', 'MetaRight', 'PrintScreen'])
})

test('the OS combinations panel names every combination with a reason', () => {
  assert.ok(OS_COMBINATIONS.length > 0)
  for (const entry of OS_COMBINATIONS) {
    assert.notEqual(entry.combination, '')
    assert.ok(entry.reason.length > 20, `${entry.combination} has no real explanation`)
  }
})

test('the layout has the four blocks the page draws', () => {
  assert.deepEqual(
    LAYOUT.map((block) => block.name),
    ['main', 'navigation', 'arrows', 'keypad'],
  )
})
