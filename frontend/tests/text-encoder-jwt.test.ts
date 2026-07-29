// Every token here is built from literal blocks inside the test, and the clock
// is injected, so nothing depends on a real identity provider or on when the
// suite happens to run.
import test from 'node:test'
import assert from 'node:assert/strict'
import { decodeJwt, humanGap } from '../src/tools/text-encoder/jwt.ts'

const NOW = new Date('2026-07-26T12:00:00Z')
const NOW_SECONDS = NOW.getTime() / 1000

const HOUR = 3600
const DAY = 86400

/** Base64URL without padding, the way a real token carries its blocks. */
function blockOf(value: string): string {
  const bytes = new TextEncoder().encode(value)
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  let out = ''
  for (let i = 0; i < bytes.length; i += 3) {
    const remaining = bytes.length - i
    const block =
      (bytes[i] << 16) | ((remaining > 1 ? bytes[i + 1] : 0) << 8) | (remaining > 2 ? bytes[i + 2] : 0)
    out += alphabet[(block >> 18) & 63] + alphabet[(block >> 12) & 63]
    if (remaining > 1) out += alphabet[(block >> 6) & 63]
    if (remaining > 2) out += alphabet[block & 63]
  }
  return out
}

function tokenOf(payload: object, signature = 'c2ln'): string {
  return `${blockOf('{"alg":"HS256","typ":"JWT"}')}.${blockOf(JSON.stringify(payload))}.${signature}`
}

test('decodeJwt reads a normal token', () => {
  const result = decodeJwt(tokenOf({ sub: '1234', name: 'Ana' }), NOW)
  assert.equal(result.ok, true)
  assert.equal(result.error, null)
  assert.equal(result.header, '{\n  "alg": "HS256",\n  "typ": "JWT"\n}')
  assert.equal(result.payload, '{\n  "sub": "1234",\n  "name": "Ana"\n}')
  assert.equal(result.signature, 'c2ln')
})

test('decodeJwt strips a Bearer prefix', () => {
  const token = tokenOf({ sub: '1234' })
  const plain = decodeJwt(token, NOW)
  assert.equal(plain.ok, true)
  for (const value of [`Bearer ${token}`, `bearer ${token}`, `  ${token}  `]) {
    assert.deepEqual(decodeJwt(value, NOW), plain, value)
  }
})

test('decodeJwt rejects the wrong shape', () => {
  const cases: Array<[string, number]> = [
    ['eyJhbGciOiJIUzI1NiJ9', 1],
    ['eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0', 2],
    ['eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.c2ln.extra', 4],
  ]
  for (const [value, count] of cases) {
    const result = decodeJwt(value, NOW)
    assert.equal(result.ok, false)
    assert.equal(
      result.error,
      `That does not look like a token. A JWT is three blocks separated by full stops. This has ${count}. Copy the whole value, including the signature on the end.`,
      value,
    )
  }
})

test('decodeJwt rejects a broken header or payload', () => {
  const goodHeader = blockOf('{"alg":"HS256"}')
  const goodPayload = blockOf('{"sub":"1234"}')

  assert.equal(
    decodeJwt(`not+base64.${goodPayload}.c2ln`, NOW).error,
    'The first block of the token is not valid Base64, so the header could not be read. The value was probably cut short when it was copied.',
  )
  assert.equal(
    decodeJwt(`${blockOf('not json')}.${goodPayload}.c2ln`, NOW).error,
    "The token's header decoded, but it is not JSON. This may not be a JWT.",
  )
  assert.equal(
    decodeJwt(`${goodHeader}.not+base64.c2ln`, NOW).error,
    'The second block of the token is not valid Base64, so the payload could not be read. The value was probably cut short when it was copied.',
  )
  assert.equal(
    decodeJwt(`${goodHeader}.${blockOf('[1,2,3')}.c2ln`, NOW).error,
    "The token's payload decoded, but it is not JSON. This may not be a JWT, or it may have been cut short when it was copied.",
  )
})

test('decodeJwt reports an expired token', () => {
  const fourHours = decodeJwt(tokenOf({ exp: NOW_SECONDS - 4 * HOUR }), NOW)
  assert.equal(fourHours.state, 'expired')
  assert.equal(
    fourHours.verdict,
    'This token expired 4 hours ago. Anything checking it properly will reject it.',
  )

  const threeDays = decodeJwt(tokenOf({ exp: NOW_SECONDS - 3 * DAY }), NOW)
  assert.equal(threeDays.state, 'expired')
  assert.equal(
    threeDays.verdict,
    'This token expired 3 days ago. Anything checking it properly will reject it.',
  )
})

test('decodeJwt reports a live token', () => {
  const halfHour = decodeJwt(tokenOf({ exp: NOW_SECONDS + 30 * 60 }), NOW)
  assert.equal(halfHour.state, 'valid')
  assert.equal(halfHour.verdict, 'This token is still valid for another 30 minutes.')

  assert.equal(decodeJwt(tokenOf({ exp: NOW_SECONDS }), NOW).state, 'valid')
})

test('decodeJwt handles a token with no expiry', () => {
  const sentence = 'This token has no expiry time in it, so it does not expire on its own.'
  for (const payload of [{ sub: '1234' }, { sub: '1234', exp: '12345' }]) {
    const result = decodeJwt(tokenOf(payload), NOW)
    assert.equal(result.state, 'none')
    assert.equal(result.verdict, sentence)
    assert.deepEqual(result.times, [])
  }
})

test('decodeJwt lists the time claims in a fixed order', () => {
  const scrambled = decodeJwt(
    tokenOf({ iat: NOW_SECONDS - HOUR, nbf: NOW_SECONDS - HOUR, exp: NOW_SECONDS + HOUR }),
    NOW,
  )
  assert.deepEqual(
    scrambled.times.map((time) => [time.claim, time.label]),
    [
      ['exp', 'Expires (exp)'],
      ['iat', 'Issued at (iat)'],
      ['nbf', 'Not valid before (nbf)'],
    ],
  )
  assert.equal(scrambled.times[0].seconds, NOW_SECONDS + HOUR)

  const onlyIat = decodeJwt(tokenOf({ iat: NOW_SECONDS - HOUR }), NOW)
  assert.equal(onlyIat.times.length, 1)
  assert.equal(onlyIat.times[0].claim, 'iat')
})

test('decodeJwt flags a token that is not usable yet', () => {
  const later = decodeJwt(tokenOf({ nbf: NOW_SECONDS + 2 * HOUR }), NOW)
  assert.equal(later.notYet, 'This token is not valid yet. It starts working in 2 hours from now.')

  assert.equal(decodeJwt(tokenOf({ nbf: NOW_SECONDS - HOUR }), NOW).notYet, null)
})

test('decodeJwt handles an unsigned token', () => {
  const result = decodeJwt(tokenOf({ sub: '1234' }, ''), NOW)
  assert.equal(result.ok, true)
  assert.equal(result.signature, '')
})

test('decodeJwt returns the empty state for empty input', () => {
  for (const value of ['', '   ']) {
    const result = decodeJwt(value, NOW)
    assert.equal(result.ok, false)
    assert.equal(result.error, null)
    assert.equal(result.header, '')
    assert.equal(result.payload, '')
  }
})

test('humanGap words every range', () => {
  const cases: Array<[number, string]> = [
    [0, 'less than a minute'],
    [999, 'less than a minute'],
    [59999, 'less than a minute'],
    [60000, '1 minute'],
    [90000, '2 minutes'],
    [3599999, '60 minutes'],
    [3600000, '1 hour'],
    [5400000, '2 hours'],
    [86399999, '24 hours'],
    [86400000, '1 day'],
    [259200000, '3 days'],
  ]
  for (const [ms, expected] of cases) {
    assert.equal(humanGap(ms), expected, String(ms))
  }
})
