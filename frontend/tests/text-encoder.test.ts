// The expected values here are literals, not values computed by the code under
// test, so a rewrite of the encoder has to agree with them rather than with
// itself. The emoji cases are the ones btoa and atob get wrong.
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  convert,
  decodeBase64,
  decodeBase64Url,
  decodeHex,
  decodeHtml,
  decodeUrl,
  encodeBase64,
  encodeBase64Url,
  encodeHex,
  encodeHtml,
  encodeUrl,
  type Conversion,
  type Direction,
} from '../src/tools/text-encoder/codecs.ts'

const BINARY =
  'That decoded, but the result is not readable text. It looks like binary data, such as a file, an image or something encrypted.'

const UNKNOWN_ENTITY_NOTE =
  'Some entities were left as they are. This tool knows &amp;, &lt;, &gt;, &quot;, &apos; and &nbsp;, plus numbered ones like &#169; and &#x00e9;.'

test('every conversion returns nothing for an empty string', () => {
  const fns = [
    encodeBase64,
    decodeBase64,
    encodeBase64Url,
    decodeBase64Url,
    encodeUrl,
    decodeUrl,
    encodeHex,
    decodeHex,
    encodeHtml,
    decodeHtml,
  ]
  for (const fn of fns) {
    assert.deepEqual(fn(''), { ok: true, text: '', error: null, note: null }, fn.name)
  }
})

test('base64 encodes and decodes plain text', () => {
  const cases: Array<[string, string]> = [
    ['Hello world', 'SGVsbG8gd29ybGQ='],
    ['Hi', 'SGk='],
    ['H', 'SA=='],
    ['   ', 'ICAg'],
  ]
  for (const [plain, encoded] of cases) {
    assert.equal(encodeBase64(plain).text, encoded)
    assert.equal(decodeBase64(encoded).text, plain)
    assert.equal(decodeBase64(encoded).ok, true)
  }
})

test('base64 survives a multi-byte emoji', () => {
  assert.equal(encodeBase64('\u{1F600}').text, '8J+YgA==')
  assert.equal(decodeBase64('8J+YgA==').text, '\u{1F600}')
  assert.equal(encodeBase64('café').text, 'Y2Fmw6k=')
  assert.equal(decodeBase64('Y2Fmw6k=').text, 'café')
})

test('base64 ignores line breaks in the input', () => {
  assert.deepEqual(decodeBase64('SGVs\r\nbG8g\nd29ybGQ='), {
    ok: true,
    text: 'Hello world',
    error: null,
    note: null,
  })
})

test('base64 rejects a bad value with a sentence a tech can act on', () => {
  const cases: Array<[string, string]> = [
    [
      'SGVsbG8',
      'That is not valid Base64: it is 7 characters long, and Base64 comes in blocks of 4 padded with = on the end. Either it was cut short when it was copied, or it is the URL-safe kind, which leaves the = off.',
    ],
    [
      'SGVsbG8h!',
      'That is not valid Base64: it contains "!", which Base64 never uses. Check you copied the value and nothing around it.',
    ],
    [
      'a-b_',
      'That is not valid Base64: it contains "-", which plain Base64 never uses. Values with - and _ in them are the URL-safe kind, so pick Base64 (URL-safe) instead.',
    ],
    [
      'SG=k=',
      'That is not valid Base64: the = signs must only be at the very end. Check the value was copied in one piece.',
    ],
    [
      'SGk===',
      'That is not valid Base64: the = signs must only be at the very end. Check the value was copied in one piece.',
    ],
  ]
  for (const [value, message] of cases) {
    assert.deepEqual(decodeBase64(value), { ok: false, text: '', error: message, note: null }, value)
  }
})

test('base64url uses the other alphabet and drops the padding', () => {
  assert.equal(encodeBase64Url('\u{1F600}').text, '8J-YgA')
  assert.equal(decodeBase64Url('8J-YgA').text, '\u{1F600}')
  assert.equal(decodeBase64Url('SGVsbG8gd29ybGQ').text, 'Hello world')
  assert.equal(decodeBase64Url('SGVsbG8gd29ybGQ=').text, 'Hello world')
  assert.equal(
    decodeBase64Url('SGVsbG8+').error,
    'That is not valid Base64 (URL-safe): it contains "+". The URL-safe form uses - and _ where plain Base64 uses + and /, so pick Base64 instead.',
  )
  assert.equal(
    decodeBase64Url('SGVsb').error,
    'That is not valid Base64 (URL-safe): it is 5 characters long, which no Base64 value ever is. It was probably cut short when it was copied.',
  )
  assert.equal(
    decodeBase64Url('SGVsbG8?').error,
    'That is not valid Base64 (URL-safe): it contains "?", which the URL-safe alphabet never uses. Check you copied the value and nothing around it.',
  )
})

test('base64 decode reports binary rather than mangling it', () => {
  for (const value of ['////', '/w==']) {
    assert.deepEqual(decodeBase64(value), { ok: false, text: '', error: BINARY, note: null }, value)
  }
})

test('hex encodes lowercase and decodes either case', () => {
  assert.equal(encodeHex('Hello').text, '48656c6c6f')
  assert.equal(decodeHex('48656C6C6F').text, 'Hello')
  assert.equal(decodeHex('48656c6c6f').text, 'Hello')
  assert.equal(encodeHex('\u{1F600}').text, 'f09f9880')
  assert.equal(decodeHex('f09f9880').text, '\u{1F600}')
})

test('hex ignores separators a tech would paste', () => {
  for (const value of ['48:65:6c:6c:6f', '48 65 6c 6c 6f', '48-65-6c-6c-6f']) {
    assert.equal(decodeHex(value).text, 'Hello', value)
  }
})

test('hex rejects a bad value', () => {
  assert.equal(
    decodeHex('48656c6c6').error,
    'That is not valid Hex: it has 9 digits, and hex uses two digits for every byte, so the total has to be an even number. One digit was probably lost when it was copied.',
  )
  assert.equal(
    decodeHex('zz').error,
    'That is not valid Hex: it contains "z". Hex only uses the digits 0 to 9 and the letters A to F.',
  )
  assert.deepEqual(decodeHex('ff'), { ok: false, text: '', error: BINARY, note: null })
})

test('url encoding keeps a plus and encodes a space', () => {
  assert.equal(encodeUrl('a b').text, 'a%20b')
  assert.equal(decodeUrl('a%20b').text, 'a b')
  assert.equal(decodeUrl('a+b').text, 'a+b')
  assert.equal(encodeUrl('a+b').text, 'a%2Bb')
})

test('url encoding survives an emoji', () => {
  assert.equal(encodeUrl('\u{1F600}').text, '%F0%9F%98%80')
  assert.equal(decodeUrl('%F0%9F%98%80').text, '\u{1F600}')
})

test('url decoding rejects a broken escape', () => {
  for (const value of ['%zz', '100%', '%2']) {
    assert.deepEqual(
      decodeUrl(value),
      {
        ok: false,
        text: '',
        error:
          'That is not valid URL encoding: a % has to be followed by two hex digits, like %20 for a space. Check the value was copied in full.',
        note: null,
      },
      value,
    )
  }
  assert.equal(
    decodeUrl('%C3').error,
    'Those % codes do not spell out readable text. The value may have been cut short, or it may not be URL encoded at all.',
  )
})

test('html entities round trip the five characters', () => {
  const plain = '<b>Bob & "Ana" said it\'s fine</b>'
  const encoded = '&lt;b&gt;Bob &amp; &quot;Ana&quot; said it&#39;s fine&lt;/b&gt;'
  assert.equal(encodeHtml(plain).text, encoded)
  assert.deepEqual(decodeHtml(encoded), { ok: true, text: plain, error: null, note: null })
})

test('html decode understands numeric entities', () => {
  const cases: Array<[string, string]> = [
    ['&#169;', '©'],
    ['&#xA9;', '©'],
    ['&#xa9;', '©'],
    ['&#233;', 'é'],
    ['&#x00e9;', 'é'],
    ['&nbsp;', '\u00a0'],
    ['&apos;', "'"],
  ]
  for (const [entity, character] of cases) {
    assert.deepEqual(
      decodeHtml(entity),
      { ok: true, text: character, error: null, note: null },
      entity,
    )
  }
})

test('html decode leaves an entity it does not know alone and says so', () => {
  for (const value of ['&frac12;', '&#1114112;', '&#xD800;']) {
    assert.deepEqual(
      decodeHtml(value),
      { ok: true, text: value, error: null, note: UNKNOWN_ENTITY_NOTE },
      value,
    )
  }
  assert.equal(decodeHtml('nothing to decode here').note, null)
})

test('convert dispatches to the right function', () => {
  const cases: Array<[Conversion, Direction, string, ReturnType<typeof encodeBase64>]> = [
    ['base64', 'encode', 'Hello world', encodeBase64('Hello world')],
    ['base64', 'decode', 'SGVsbG8gd29ybGQ=', decodeBase64('SGVsbG8gd29ybGQ=')],
    ['base64url', 'encode', '\u{1F600}', encodeBase64Url('\u{1F600}')],
    ['base64url', 'decode', '8J-YgA', decodeBase64Url('8J-YgA')],
    ['url', 'encode', 'a b', encodeUrl('a b')],
    ['url', 'decode', 'a%20b', decodeUrl('a%20b')],
    ['hex', 'encode', 'Hello', encodeHex('Hello')],
    ['hex', 'decode', '48656c6c6f', decodeHex('48656c6c6f')],
    ['html', 'encode', '<b>&</b>', encodeHtml('<b>&</b>')],
    ['html', 'decode', '&lt;b&gt;&amp;&lt;/b&gt;', decodeHtml('&lt;b&gt;&amp;&lt;/b&gt;')],
  ]
  for (const [conversion, direction, input, expected] of cases) {
    assert.deepEqual(convert(input, conversion, direction), expected, `${conversion} ${direction}`)
  }
})
