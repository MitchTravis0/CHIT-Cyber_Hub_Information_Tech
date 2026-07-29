// The phonetic tables are the whole tool, so the expected words are written out
// here as literals rather than derived from the source. A "fix" that turns Alfa
// into Alpha or Nine into Niner has to fail here before it reaches a phone call.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  convert,
  elementFor,
  LETTERS,
  MARKS,
  MAX_CHARACTERS,
  NUMBERS,
} from '../src/tools/phonetic-alphabet/phonetic.ts'

const EXPECTED_MARKS: Record<string, string> = {
  ' ': 'Space',
  '!': 'Exclamation mark',
  '"': 'Quote',
  '#': 'Hash',
  $: 'Dollar',
  '%': 'Percent',
  '&': 'Ampersand',
  "'": 'Apostrophe',
  '(': 'Open bracket',
  ')': 'Close bracket',
  '*': 'Star',
  '+': 'Plus',
  ',': 'Comma',
  '-': 'Dash',
  '.': 'Dot',
  '/': 'Slash',
  ':': 'Colon',
  ';': 'Semicolon',
  '<': 'Less than',
  '=': 'Equals',
  '>': 'Greater than',
  '?': 'Question mark',
  '@': 'At',
  '[': 'Open square bracket',
  '\\': 'Backslash',
  ']': 'Close square bracket',
  '^': 'Caret',
  _: 'Underscore',
  '`': 'Backtick',
  '{': 'Open curly bracket',
  '|': 'Pipe',
  '}': 'Close curly bracket',
  '~': 'Tilde',
}

test('the letter table is complete and spelled the ICAO way', () => {
  const keys = Object.keys(LETTERS)
  assert.equal(keys.length, 26)
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('')
  assert.deepEqual(keys.slice().sort(), alphabet)

  assert.equal(LETTERS.A, 'Alfa')
  assert.notEqual(LETTERS.A, 'Alpha')
  assert.equal(LETTERS.J, 'Juliett')
  assert.notEqual(LETTERS.J, 'Juliet')
  assert.equal(LETTERS.X, 'X-ray')
  assert.equal(LETTERS.W, 'Whiskey')

  for (const letter of alphabet) {
    assert.match(LETTERS[letter], /^[A-Z][a-z-]+$/, letter)
  }
})

test('the digit table is complete and plain English', () => {
  const words = ['Zero', 'One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight', 'Nine']
  assert.equal(Object.keys(NUMBERS).length, 10)
  words.forEach((word, digit) => {
    assert.equal(NUMBERS[String(digit)], word, String(digit))
  })
})

test('every keyboard symbol has a word', () => {
  assert.equal(Object.keys(EXPECTED_MARKS).length, 33)
  assert.deepEqual(MARKS, EXPECTED_MARKS)
  for (const [character, word] of Object.entries(EXPECTED_MARKS)) {
    assert.equal(typeof MARKS[character], 'string', character)
    assert.ok(word.length > 0, character)
  }
})

test('elementFor marks capitals when asked', () => {
  assert.equal(elementFor('A', true), 'Capital Alfa')
  assert.equal(elementFor('a', true), 'alfa')
  assert.equal(elementFor('X', true), 'Capital X-ray')
  assert.equal(elementFor('x', true), 'x-ray')
  assert.equal(elementFor('A', false), 'Alfa')
  assert.equal(elementFor('a', false), 'Alfa')
})

test('elementFor never marks case on digits or symbols', () => {
  for (const [character, word] of [
    ['7', 'Seven'],
    ['-', 'Dash'],
    [' ', 'Space'],
  ]) {
    assert.equal(elementFor(character, true), word, character)
    assert.equal(elementFor(character, false), word, character)
  }
})

test('elementFor names an unmapped character', () => {
  assert.equal(elementFor('😀', true), 'Unknown character "😀"')
  assert.equal(elementFor('é', true), 'Unknown character "é"')
  assert.equal(elementFor('’', true), 'Unknown character "’"')
})

test('convert returns nothing for empty input', () => {
  for (const input of ['', '   ', '\t\n ']) {
    assert.deepEqual(
      convert(input, { showCapitals: true }),
      { groups: [], text: '', unknown: [], truncated: false },
      JSON.stringify(input),
    )
  }
})

test('convert handles a single character', () => {
  const lower = convert('a', { showCapitals: true })
  assert.equal(lower.groups.length, 1)
  assert.equal(lower.groups[0].source, 'a')
  assert.deepEqual(lower.groups[0].elements, ['alfa'])
  assert.equal(lower.groups[0].line, 'a  =  alfa')
  assert.equal(lower.text, 'a  =  alfa')

  const upper = convert('A', { showCapitals: true })
  assert.equal(upper.groups.length, 1)
  assert.equal(upper.groups[0].line, 'A  =  Capital Alfa')
})

test('convert renders every digit', () => {
  const result = convert('0123456789', { showCapitals: true })
  assert.equal(result.groups.length, 1)
  assert.deepEqual(result.groups[0].elements, [
    'Zero',
    'One',
    'Two',
    'Three',
    'Four',
    'Five',
    'Six',
    'Seven',
    'Eight',
    'Nine',
  ])
})

test('convert renders every symbol', () => {
  for (const [character, word] of Object.entries(EXPECTED_MARKS)) {
    // A space is a group separator, so it can never be a group of its own.
    if (character === ' ') continue
    const result = convert(character, { showCapitals: true })
    assert.equal(result.groups.length, 1, character)
    assert.deepEqual(result.groups[0].elements, [word], character)
    assert.equal(result.unknown.length, 0, character)
  }
})

const EXAMPLE = '4KD7-9F2X mini  3'

test('convert numbers and joins multiple groups', () => {
  const result = convert(EXAMPLE, { showCapitals: true })
  assert.equal(
    result.text,
    [
      '1. 4KD7-9F2X  =  Four - Capital Kilo - Capital Delta - Seven - Dash - Nine - Capital Foxtrot - Two - Capital X-ray - Space',
      '2. mini  =  mike - india - november - india - Space',
      '3. 3  =  Three',
    ].join('\n'),
  )
  assert.deepEqual(
    result.groups.map((group) => group.source),
    ['4KD7-9F2X', 'mini', '3'],
  )
  assert.deepEqual(result.unknown, [])
  assert.equal(result.truncated, false)
})

test('convert drops case marks when the toggle is off', () => {
  const result = convert(EXAMPLE, { showCapitals: false })
  assert.equal(
    result.text,
    [
      '1. 4KD7-9F2X  =  Four - Kilo - Delta - Seven - Dash - Nine - Foxtrot - Two - X-ray - Space',
      '2. mini  =  Mike - India - November - India - Space',
      '3. 3  =  Three',
    ].join('\n'),
  )
})

test('convert trims the ends and collapses inner whitespace', () => {
  const spaced = convert('  ab  cd  ', { showCapitals: true })
  assert.deepEqual(
    spaced.groups.map((group) => group.source),
    ['ab', 'cd'],
  )
  assert.deepEqual(spaced.groups[0].elements, ['alfa', 'bravo', 'Space'])
  assert.deepEqual(spaced.groups[1].elements, ['charlie', 'delta'])

  for (const input of ['ab\tcd', 'ab\ncd']) {
    const got = convert(input, { showCapitals: true })
    assert.deepEqual(got.groups, spaced.groups, JSON.stringify(input))
    assert.equal(got.text, spaced.text, JSON.stringify(input))
  }
})

test('convert lists unknown characters once each in order', () => {
  const result = convert('a😀b😀é', { showCapitals: true })
  assert.deepEqual(result.unknown, ['😀', 'é'])
  assert.equal(result.groups.length, 1)
  assert.equal(result.groups[0].elements.length, 5)
  assert.deepEqual(result.groups[0].elements, [
    'alfa',
    'Unknown character "😀"',
    'bravo',
    'Unknown character "😀"',
    'Unknown character "é"',
  ])
})

test('convert counts an emoji as one character', () => {
  const result = convert('😀', { showCapitals: true })
  assert.equal(result.groups.length, 1)
  assert.equal(result.groups[0].elements.length, 1)
  assert.deepEqual(result.groups[0].elements, ['Unknown character "😀"'])
})

test('convert truncates a long paste', () => {
  // The spec fixes the cap at 500, and every other assertion here is written
  // against the constant, so this line is what stops the cap moving silently.
  assert.equal(MAX_CHARACTERS, 500)

  const long = convert('a'.repeat(600), { showCapitals: true })
  assert.equal(long.truncated, true)
  assert.equal(
    long.groups.reduce((total, group) => total + group.elements.length, 0),
    MAX_CHARACTERS,
  )

  const exact = convert('a'.repeat(MAX_CHARACTERS), { showCapitals: true })
  assert.equal(exact.truncated, false)
  assert.equal(
    exact.groups.reduce((total, group) => total + group.elements.length, 0),
    MAX_CHARACTERS,
  )
})
