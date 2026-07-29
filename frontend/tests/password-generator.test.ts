// The password generator has no Go side, so these tests are the only thing
// guarding the randomness, the pools and the entropy figures the UI prints.
// Randomness is asserted through properties and through a scripted draw, never
// by pinning a value that cryptoFill produced.
import test from 'node:test'
import assert from 'node:assert/strict'
import { cryptoFill, randomIndex, shuffle, type RandomFill } from '../src/tools/password-generator/random.ts'
import {
  charEntropyBits,
  generatePassphrase,
  generatePassword,
  optionsError,
  parseCount,
  parseLength,
  parseWordCount,
  phraseEntropyBits,
  poolFor,
  strengthFor,
  DIGITS,
  LOOKALIKES,
  LOWER,
  SYMBOLS,
  UPPER,
  type CharOptions,
  type SeparatorValue,
} from '../src/tools/password-generator/generate.ts'
import { WORDS } from '../src/tools/password-generator/wordlist.ts'

function scripted(values: number[]): RandomFill {
  let i = 0
  return (out) => {
    out[0] = values[i++]
    return out
  }
}

const ALL_CLASSES = { lower: true, upper: true, digits: true, symbols: true }

function chars(over: Partial<CharOptions> = {}): CharOptions {
  return { length: 20, ...ALL_CLASSES, excludeLookAlikes: true, ...over }
}

const POOL_ALL_INCLUDED =
  'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&()*+-=?@[]^_{}~'
const POOL_ALL_EXCLUDED =
  'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!#$%&()*+-=?@[]^_{}~'

test('randomIndex rejects the biased tail and redraws', () => {
  // limit 10 gives a ceiling of 4294967290: both draws at or above it are
  // thrown away rather than folded back into the range.
  const fill = scripted([4294967295, 4294967290, 7])
  assert.equal(randomIndex(10, fill), 7)
})

test('randomIndex accepts the last good value', () => {
  assert.equal(randomIndex(10, scripted([4294967289])), 9)
})

test('randomIndex with a limit of 1 always returns 0', () => {
  assert.equal(randomIndex(1, scripted([0])), 0)
  assert.equal(randomIndex(1, scripted([4294967295])), 0)
})

test('randomIndex stays inside the range', () => {
  const seen = new Set<number>()
  for (let i = 0; i < 5000; i++) {
    const value = randomIndex(76)
    assert.ok(Number.isInteger(value), `not an integer: ${value}`)
    assert.ok(value >= 0 && value <= 75, `out of range: ${value}`)
    seen.add(value)
  }
  assert.equal(seen.size, 76)
})

test('shuffle is a permutation', () => {
  const source = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j']
  const sorted = [...source].sort()
  let differed = 0
  for (let run = 0; run < 200; run++) {
    const result = shuffle([...source], cryptoFill)
    assert.deepEqual([...result].sort(), sorted)
    if (result.join('') !== source.join('')) differed++
  }
  assert.ok(differed > 0, 'every shuffle returned the input order')
})

test('shuffle is deterministic for a scripted source', () => {
  const result = shuffle(['a', 'b', 'c', 'd'], scripted([0, 1, 0]))
  assert.deepEqual(result, ['c', 'd', 'b', 'a'])
})

test('shuffle draws from 0 to i, so an item can stay where it is', () => {
  // Drawing from 0 to i-1 instead is Sattolo's algorithm: still a permutation,
  // still never the input order, but every item is guaranteed to move, which
  // biases where the guaranteed one-per-class characters end up. The scripted
  // draws below ask for the identity at every step, which only the unbiased
  // bound can produce.
  assert.deepEqual(shuffle(['a', 'b', 'c'], scripted([2, 1])), ['a', 'b', 'c'])

  let stayed = 0
  for (let run = 0; run < 200; run++) {
    if (shuffle(['a', 'b', 'c', 'd'], cryptoFill)[0] === 'a') stayed++
  }
  assert.ok(stayed > 0, 'no shuffle in 200 runs left the first item in place')
})

test('poolFor builds the pool in class order', () => {
  assert.equal(poolFor(chars({ excludeLookAlikes: false })), POOL_ALL_INCLUDED)
  assert.equal(poolFor(chars({ excludeLookAlikes: false })).length, 82)
  assert.equal(poolFor(chars()), POOL_ALL_EXCLUDED)
  assert.equal(poolFor(chars()).length, 76)

  const lowerOnly = { lower: true, upper: false, digits: false, symbols: false }
  assert.equal(poolFor(chars(lowerOnly)), 'abcdefghijkmnpqrstuvwxyz')
  assert.equal(poolFor(chars({ ...lowerOnly, excludeLookAlikes: false })), LOWER)

  const digitsOnly = { lower: false, upper: false, digits: true, symbols: false }
  assert.equal(poolFor(chars(digitsOnly)), '23456789')
  assert.equal(poolFor(chars({ ...digitsOnly, excludeLookAlikes: false })), DIGITS)

  const symbolsOnly = { lower: false, upper: false, digits: false, symbols: true }
  assert.equal(poolFor(chars(symbolsOnly)), SYMBOLS)
  assert.equal(poolFor(chars({ ...symbolsOnly, excludeLookAlikes: false })), SYMBOLS)

  assert.equal(poolFor(chars({ lower: false, upper: false, digits: false, symbols: false })), '')
})

test('poolFor never contains a look-alike when the toggle is on', () => {
  const excluded = poolFor(chars())
  const included = poolFor(chars({ excludeLookAlikes: false }))
  for (const character of LOOKALIKES) {
    assert.ok(!excluded.includes(character), `${character} survived the exclusion`)
    assert.ok(included.includes(character), `${character} is missing from the full pool`)
  }
  assert.equal(UPPER.length, 26)
})

test('optionsError names the missing classes', () => {
  assert.equal(
    optionsError(chars({ lower: false, upper: false, digits: false, symbols: false })),
    'Pick at least one kind of character: lowercase, uppercase, digits or symbols.',
  )
  assert.equal(optionsError(chars({ upper: false, digits: false, symbols: false })), null)
  assert.equal(optionsError(chars({ lower: false, digits: false, symbols: false })), null)
  assert.equal(optionsError(chars({ lower: false, upper: false, symbols: false })), null)
  assert.equal(optionsError(chars({ lower: false, upper: false, digits: false })), null)
})

test('generatePassword honours the length', () => {
  for (const length of [8, 20, 128]) {
    assert.equal(generatePassword(chars({ length })).length, length)
  }
})

test('generatePassword includes every enabled class', () => {
  const pools = ['abcdefghijkmnpqrstuvwxyz', 'ABCDEFGHJKLMNPQRSTUVWXYZ', '23456789', SYMBOLS]
  for (let run = 0; run < 300; run++) {
    const password = generatePassword(chars({ length: 8 }))
    for (const pool of pools) {
      assert.ok(
        [...password].some((character) => pool.includes(character)),
        `${password} has nothing from ${pool}`,
      )
    }
  }
  for (let run = 0; run < 100; run++) {
    const password = generatePassword(
      chars({ length: 8, lower: false, upper: false, symbols: false }),
    )
    assert.match(password, /^[2-9]{8}$/)
  }
})

test('generatePassword never emits a look-alike when excluded', () => {
  for (let run = 0; run < 300; run++) {
    const password = generatePassword(chars({ length: 128 }))
    for (const character of LOOKALIKES) {
      assert.ok(!password.includes(character), `${character} appeared in ${password}`)
    }
  }
})

test('generatePassword does not leak class positions', () => {
  let firstIsLower = 0
  for (let run = 0; run < 300; run++) {
    const password = generatePassword(chars({ length: 20 }))
    if (/^[a-z]/.test(password)) firstIsLower++
  }
  assert.ok(firstIsLower < 290, `the first character was lowercase ${firstIsLower} times out of 300`)
})

test('generatePassphrase builds the phrase', () => {
  const separators: SeparatorValue[] = ['-', '.', '_', ' ']
  for (const separator of separators) {
    for (const words of [3, 5, 12]) {
      const plain = generatePassphrase({ words, separator, capitalise: false, addNumber: false })
      const parts = plain.split(separator)
      assert.equal(parts.length, words)
      for (const part of parts) assert.ok(WORDS.includes(part), `${part} is not in the wordlist`)

      const capitalised = generatePassphrase({
        words,
        separator,
        capitalise: true,
        addNumber: false,
      })
      const capParts = capitalised.split(separator)
      assert.equal(capParts.length, words)
      for (const part of capParts) {
        assert.match(part, /^[A-Z][a-z]*$/)
        assert.ok(WORDS.includes(part.toLowerCase()), `${part} is not in the wordlist`)
      }

      const numbered = generatePassphrase({ words, separator, capitalise: false, addNumber: true })
      const numParts = numbered.split(separator)
      assert.equal(numParts.length, words + 1)
      assert.match(numParts[numParts.length - 1], /^[0-9]{2}$/)
      for (const part of numParts.slice(0, words)) {
        assert.ok(WORDS.includes(part), `${part} is not in the wordlist`)
      }
    }
  }

  const scriptedPhrase = generatePassphrase(
    { words: 3, separator: '-', capitalise: false, addNumber: true },
    scripted([0, 0, 0, 7]),
  )
  assert.equal(scriptedPhrase, `${WORDS[0]}-${WORDS[0]}-${WORDS[0]}-07`)
})

test('charEntropyBits matches the published figures', () => {
  assert.equal(charEntropyBits(chars()), 125)
  assert.equal(charEntropyBits(chars({ excludeLookAlikes: false })), 127)
  assert.equal(charEntropyBits(chars({ length: 8 })), 50)
  assert.equal(
    charEntropyBits(chars({ length: 20, lower: false, upper: false, symbols: false })),
    60,
  )
  assert.equal(charEntropyBits(chars({ length: 128 })), 800)
})

test('phraseEntropyBits matches the published figures', () => {
  assert.equal(
    phraseEntropyBits({ words: 5, separator: '-', capitalise: false, addNumber: false }),
    50,
  )
  assert.equal(
    phraseEntropyBits({ words: 5, separator: '-', capitalise: false, addNumber: true }),
    57,
  )
  assert.equal(
    phraseEntropyBits({ words: 5, separator: '-', capitalise: true, addNumber: false }),
    50,
  )
  assert.equal(
    phraseEntropyBits({ words: 12, separator: '-', capitalise: false, addNumber: true }),
    127,
  )
})

test('strengthFor labels every band', () => {
  const cases: Array<[number, string, string]> = [
    [0, 'Weak', 'danger'],
    [24, 'Weak', 'danger'],
    [49, 'Weak', 'danger'],
    [50, 'Reasonable', 'warn'],
    [57, 'Reasonable', 'warn'],
    [74, 'Reasonable', 'warn'],
    [75, 'Strong', 'ok'],
    [99, 'Strong', 'ok'],
    [100, 'Very strong', 'ok'],
    [125, 'Very strong', 'ok'],
    [800, 'Very strong', 'ok'],
  ]
  for (const [bits, label, tone] of cases) {
    const strength = strengthFor(bits)
    assert.equal(strength.bits, bits)
    assert.equal(strength.label, label, `${bits} bits`)
    assert.equal(strength.tone, tone, `${bits} bits`)
    assert.ok(strength.advice.endsWith('.'))
  }
  assert.equal(strengthFor(49).label, 'Weak')
  assert.equal(strengthFor(50).label, 'Reasonable')
})

test('parseLength rejects what a tech might type', () => {
  const notWhole = 'Type how many characters you want, between 8 and 128.'
  const tooShort =
    '8 characters is the shortest this will make. Anything shorter is not worth generating.'
  const tooLong = '128 characters is the longest this will make. Very few systems accept more.'
  const cases: Array<[string, boolean, number, string | null]> = [
    ['', false, 0, notWhole],
    ['   ', false, 0, notWhole],
    ['abc', false, 0, notWhole],
    ['20.5', false, 0, notWhole],
    // '2e3' is a whole number to Number(), so the range check is what stops it.
    ['2e3', false, 0, tooLong],
    ['-1', false, 0, tooShort],
    ['0', false, 0, tooShort],
    ['7', false, 0, tooShort],
    ['8', true, 8, null],
    ['20', true, 20, null],
    [' 20 ', true, 20, null],
    ['128', true, 128, null],
    ['129', false, 0, tooLong],
  ]
  for (const [raw, ok, value, error] of cases) {
    const parsed = parseLength(raw)
    assert.equal(parsed.ok, ok, raw)
    assert.equal(parsed.value, value, raw)
    assert.equal(parsed.error, error, raw)
  }
})

test('parseWordCount rejects what a tech might type', () => {
  const notWhole = 'Type how many words you want, between 3 and 12.'
  const cases: Array<[string, boolean, number, string | null]> = [
    ['', false, 0, notWhole],
    ['x', false, 0, notWhole],
    ['2', false, 0, 'Use at least 3 words. Fewer than that is quick to guess.'],
    ['3', true, 3, null],
    ['12', true, 12, null],
    [
      '13',
      false,
      0,
      '12 words is the most this will make. That is already far more than anything needs.',
    ],
  ]
  for (const [raw, ok, value, error] of cases) {
    const parsed = parseWordCount(raw)
    assert.equal(parsed.ok, ok, raw)
    assert.equal(parsed.value, value, raw)
    assert.equal(parsed.error, error, raw)
  }
})

test('parseCount rejects what a tech might type', () => {
  const notWhole = 'Type how many passwords you want, between 1 and 20.'
  const cases: Array<[string, boolean, number, string | null]> = [
    ['', false, 0, notWhole],
    ['0', false, 0, notWhole],
    ['1', true, 1, null],
    ['20', true, 20, null],
    ['21', false, 0, '20 at a time is the most. Press Generate again if you need more.'],
    ['1.5', false, 0, notWhole],
  ]
  for (const [raw, ok, value, error] of cases) {
    const parsed = parseCount(raw)
    assert.equal(parsed.ok, ok, raw)
    assert.equal(parsed.value, value, raw)
    assert.equal(parsed.error, error, raw)
  }
})

test('the wordlist is exactly 1024 usable words', () => {
  assert.equal(WORDS.length, 1024)
  for (const word of WORDS) assert.match(word, /^[a-z]{3,8}$/)
  assert.equal(new Set(WORDS).size, 1024)
  assert.deepEqual(WORDS, [...WORDS].sort())
})

test('no word is another word with an s on the end', () => {
  const known = new Set(WORDS)
  for (const word of WORDS) {
    if (!word.endsWith('s')) continue
    assert.ok(!known.has(word.slice(0, -1)), `${word} is ${word.slice(0, -1)} with an s`)
  }
})
