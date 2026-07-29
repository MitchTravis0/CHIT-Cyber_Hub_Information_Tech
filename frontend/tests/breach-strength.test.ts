import test from 'node:test'
import assert from 'node:assert/strict'
import {
  bandFor,
  charsetSize,
  commonExact,
  commonInside,
  keyboardRuns,
  repeatRuns,
  score,
  sequenceRuns,
  yearIn,
} from '../src/tools/breach-checker/strength.ts'
import { COMMON_PASSWORDS } from '../src/tools/breach-checker/common-passwords.ts'

const MANDATORY = [
  'password',
  'password1',
  'passw0rd',
  'p@ssw0rd',
  '123456',
  '1234567',
  '12345678',
  '123456789',
  'qwerty',
  'qwertyuiop',
  'abc123',
  '111111',
  'letmein',
  'monkey',
  'dragon',
  'iloveyou',
  'admin',
  'welcome',
  'login',
  'sunshine',
  'master',
  'football',
  'princess',
  'trustno1',
]

test('charsetSize adds one pool per kind of character used', () => {
  const cases: Array<[string, number]> = [
    ['', 0],
    ['abc', 26],
    ['ABC', 26],
    ['abcABC', 52],
    ['abc123', 36],
    ['abc!', 59],
    ['a b', 59],
    ['abc123!X', 95],
    ['café', 126],
    ['🔒', 100],
  ]
  for (const [input, want] of cases) {
    assert.equal(charsetSize(input), want, `charsetSize(${JSON.stringify(input)})`)
  }
})

test('sequenceRuns finds maximal runs of four or more stepping characters', () => {
  assert.deepEqual(sequenceRuns('abcd'), ['abcd'])
  assert.deepEqual(sequenceRuns('1234'), ['1234'])
  assert.deepEqual(sequenceRuns('dcba'), ['dcba'])
  assert.deepEqual(sequenceRuns('abc'), [])
  assert.deepEqual(sequenceRuns('abcde'), ['abcde'])
  assert.deepEqual(sequenceRuns('abcd1234'), ['abcd', '1234'])
  assert.deepEqual(sequenceRuns('aXbXc'), [])
})

test('repeatRuns finds maximal runs of three or more identical characters', () => {
  assert.deepEqual(repeatRuns('aaa'), ['aaa'])
  assert.deepEqual(repeatRuns('aa'), [])
  assert.deepEqual(repeatRuns('aaaa'), ['aaaa'])
  assert.deepEqual(repeatRuns('aaabbb'), ['aaa', 'bbb'])
  assert.deepEqual(repeatRuns('!!!!'), ['!!!!'])
})

test('keyboardRuns finds row runs forwards and backwards, and ignores the digit row', () => {
  assert.deepEqual(keyboardRuns('qwerty'), ['qwerty'])
  assert.deepEqual(keyboardRuns('QWERTY'), ['qwerty'])
  assert.deepEqual(keyboardRuns('asdf'), ['asdf'])
  assert.deepEqual(keyboardRuns('ytrewq'), ['ytrewq'])
  assert.deepEqual(keyboardRuns('qwe'), [])
  assert.deepEqual(keyboardRuns('1234'), [])
})

test('yearIn finds the first four digits that read as a year from 1900 to 2099', () => {
  assert.equal(yearIn('summer2024'), '2024')
  assert.equal(yearIn('1999x'), '1999')
  assert.equal(yearIn('x1899'), '')
  assert.equal(yearIn('2100'), '')
  assert.equal(yearIn('20244'), '2024')
  assert.equal(yearIn('abcd'), '')
})

test('commonExact and commonInside read the common-password list', () => {
  assert.equal(commonExact('password'), true)
  assert.equal(commonExact('PASSWORD'), true)
  assert.equal(commonExact('pässword'), false)
  assert.equal(commonInside('xxpasswordxx'), 'password')
  assert.equal(commonInside('xxabcxx'), '')
})

test('score puts the named passwords in the right band', () => {
  const empty = score('')
  assert.equal(empty.bits, 0)
  assert.equal(empty.rawBits, 0)
  assert.equal(empty.charsetSize, 0)
  assert.equal(empty.length, 0)
  assert.equal(empty.band, 'Very weak')
  assert.deepEqual(empty.reasons, [])

  const single = score('a')
  assert.equal(single.bits, 4)
  assert.equal(single.band, 'Very weak')

  const literal = score('password')
  assert.equal(literal.bits, 0)
  assert.equal(literal.band, 'Very weak')
  assert.equal(literal.reasons.length, 1)
  assert.match(literal.reasons[0], /most common passwords/)

  const leet = score('P@ssw0rd')
  assert.equal(leet.bits, 0)
  assert.equal(leet.band, 'Very weak')

  const passphrase = score('correct horse battery staple')
  assert.equal(passphrase.rawBits, 164)
  assert.equal(passphrase.band, 'Very strong')

  const random = score('7Kq2vX9mTb4RzN8pLc3JwF6hYd1sGa5eUo0iVr2Q')
  assert.equal(random.rawBits, 238)
  assert.equal(random.band, 'Very strong')
})

test('score does the arithmetic exactly', () => {
  assert.equal(COMMON_PASSWORDS.includes('xy!abcd7'), false)

  const result = score('Xy!abcd7')
  assert.equal(result.length, 8)
  assert.equal(result.charsetSize, 95)
  assert.equal(result.rawBits, 52)
  assert.equal(result.bits, 44)
  assert.equal(result.band, 'Reasonable')
  assert.equal(result.tone, 'warn')
  assert.equal(result.reasons.length, 1)
  assert.equal(
    result.reasons[0],
    'It contains the run "abcd", which guessing tools try early.',
  )
})

test('score caps each penalty', () => {
  // Three sequence runs are 24 bits before the cap, 16 after it.
  const sequences = 'abcd!1234!wxyz'
  assert.equal(commonInside(sequences), '')
  assert.deepEqual(sequenceRuns(sequences), ['abcd', '1234', 'wxyz'])
  assert.deepEqual(keyboardRuns(sequences), [])
  assert.equal(score(sequences).rawBits, 85)
  assert.equal(score(sequences).bits, 85 - 16)

  // Three repeat runs are 18 bits before the cap, 12 after it.
  const repeats = 'aaabbbccc'
  assert.equal(commonInside(repeats), '')
  assert.deepEqual(repeatRuns(repeats), ['aaa', 'bbb', 'ccc'])
  assert.deepEqual(sequenceRuns(repeats), [])
  assert.equal(score(repeats).rawBits, 42)
  assert.equal(score(repeats).bits, 42 - 12)

  // Three keyboard runs are 30 bits before the cap, 20 after it.
  const keyboard = 'qwer!asdf!zxcv'
  assert.equal(commonInside(keyboard), '')
  assert.deepEqual(keyboardRuns(keyboard), ['qwer', 'asdf', 'zxcv'])
  assert.deepEqual(sequenceRuns(keyboard), [])
  assert.equal(score(keyboard).rawBits, 82)
  assert.equal(score(keyboard).bits, 82 - 20)

  // A year twice is still charged once.
  const years = 'x2024y2024z'
  assert.equal(commonInside(years), '')
  assert.equal(yearIn(years), '2024')
  assert.equal(score(years).rawBits, 56)
  assert.equal(score(years).bits, 56 - 8)
  assert.equal(score(years).reasons.length, 1)
})

test('an exact common password short-circuits the other rules', () => {
  const result = score('qwerty')
  assert.equal(result.bits, 0)
  assert.equal(result.reasons.length, 1)
  assert.match(result.reasons[0], /most common passwords/)
})

test('score is pure', () => {
  assert.deepEqual(score('Xy!abcd7'), score('Xy!abcd7'))
})

test('bandFor names every band and both sides of every boundary', () => {
  const cases: Array<[number, string, string]> = [
    [0, 'Very weak', 'danger'],
    [27, 'Very weak', 'danger'],
    [28, 'Weak', 'danger'],
    [39, 'Weak', 'danger'],
    [40, 'Reasonable', 'warn'],
    [59, 'Reasonable', 'warn'],
    [60, 'Strong', 'ok'],
    [79, 'Strong', 'ok'],
    [80, 'Very strong', 'ok'],
    [238, 'Very strong', 'ok'],
  ]
  for (const [bits, band, tone] of cases) {
    assert.equal(bandFor(bits).band, band, `bandFor(${bits}).band`)
    assert.equal(bandFor(bits).tone, tone, `bandFor(${bits}).tone`)
    assert.ok(bandFor(bits).advice.length > 0, `bandFor(${bits}).advice`)
  }
})

test('the common-password list is clean', () => {
  assert.ok(COMMON_PASSWORDS.length >= 250, `only ${COMMON_PASSWORDS.length} entries`)
  assert.ok(COMMON_PASSWORDS.length <= 1200, `${COMMON_PASSWORDS.length} entries is too many`)
  assert.equal(new Set(COMMON_PASSWORDS).size, COMMON_PASSWORDS.length)
  for (const entry of COMMON_PASSWORDS) {
    assert.equal(entry, entry.toLowerCase(), `${entry} is not lowercase`)
    assert.equal(entry, entry.trim(), `${entry} is not trimmed`)
    assert.match(entry, /^\S+$/, `${entry} contains whitespace`)
    assert.ok(entry.length >= 4, `${entry} is shorter than 4 characters`)
  }
  for (const entry of MANDATORY) {
    assert.ok(COMMON_PASSWORDS.includes(entry), `${entry} is missing from the list`)
  }
})
