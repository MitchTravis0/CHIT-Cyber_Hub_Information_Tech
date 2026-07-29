export type Conversion = 'base64' | 'base64url' | 'url' | 'hex' | 'html' | 'jwt'
export type Direction = 'encode' | 'decode'

export interface ConversionResult {
  ok: boolean
  /** The converted text. '' when ok is false. */
  text: string
  /** The sentence to show. null when ok is true. */
  error: string | null
  /** Something worth saying about a successful conversion. Usually null. */
  note: string | null
}

export interface BytesResult {
  ok: boolean
  bytes: Uint8Array
  error: string | null
}

const BINARY_MESSAGE =
  'That decoded, but the result is not readable text. It looks like binary data, such as a file, an image or something encrypted.'

const UNKNOWN_ENTITY_NOTE =
  'Some entities were left as they are. This tool knows &amp;, &lt;, &gt;, &quot;, &apos; and &nbsp;, plus numbered ones like &#169; and &#x00e9;.'

const STANDARD_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
const URL_SAFE_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'

function ok(text: string, note: string | null = null): ConversionResult {
  return { ok: true, text, error: null, note }
}

function fail(error: string): ConversionResult {
  return { ok: false, text: '', error, note: null }
}

/** UTF-8. Never fails: every JavaScript string has a UTF-8 encoding. */
export function textToBytes(text: string): Uint8Array {
  return new TextEncoder().encode(text)
}

/** UTF-8, strict. Fails when the bytes are not text. */
export function bytesToText(bytes: Uint8Array): ConversionResult {
  try {
    // fatal, so bytes that are not text are reported instead of coming back as
    // a string full of replacement characters that looks like a bad decode.
    return ok(new TextDecoder('utf-8', { fatal: true }).decode(bytes))
  } catch {
    return fail(BINARY_MESSAGE)
  }
}

export function bytesToBase64(bytes: Uint8Array, urlSafe: boolean): string {
  const alphabet = urlSafe ? URL_SAFE_ALPHABET : STANDARD_ALPHABET
  let out = ''
  for (let i = 0; i < bytes.length; i += 3) {
    const remaining = bytes.length - i
    const block = (bytes[i] << 16) | ((remaining > 1 ? bytes[i + 1] : 0) << 8) | (remaining > 2 ? bytes[i + 2] : 0)
    out += alphabet[(block >> 18) & 63]
    out += alphabet[(block >> 12) & 63]
    if (remaining > 1) out += alphabet[(block >> 6) & 63]
    else if (!urlSafe) out += '='
    if (remaining > 2) out += alphabet[block & 63]
    else if (!urlSafe) out += '='
  }
  return out
}

export function base64ToBytes(text: string, urlSafe: boolean): BytesResult {
  const alphabet = urlSafe ? URL_SAFE_ALPHABET : STANDARD_ALPHABET
  // Base64 out of an email or a PEM file arrives wrapped, and a line break is
  // not the user's mistake.
  const stripped = text.replace(/[ \t\r\n]/g, '')
  if (stripped === '') return { ok: true, bytes: new Uint8Array(0), error: null }

  for (const c of stripped) {
    if (c === '=' || alphabet.includes(c)) continue
    if (urlSafe) {
      if (c === '+' || c === '/') {
        return badBase64(
          `That is not valid Base64 (URL-safe): it contains "${c}". The URL-safe form uses - and _ where plain Base64 uses + and /, so pick Base64 instead.`,
        )
      }
      return badBase64(
        `That is not valid Base64 (URL-safe): it contains "${c}", which the URL-safe alphabet never uses. Check you copied the value and nothing around it.`,
      )
    }
    if (c === '-' || c === '_') {
      return badBase64(
        `That is not valid Base64: it contains "${c}", which plain Base64 never uses. Values with - and _ in them are the URL-safe kind, so pick Base64 (URL-safe) instead.`,
      )
    }
    return badBase64(
      `That is not valid Base64: it contains "${c}", which Base64 never uses. Check you copied the value and nothing around it.`,
    )
  }

  const firstPad = stripped.indexOf('=')
  const padCount = stripped.length - stripped.replace(/=/g, '').length
  if (firstPad !== -1 && (padCount > 2 || firstPad !== stripped.length - padCount)) {
    return badBase64(
      'That is not valid Base64: the = signs must only be at the very end. Check the value was copied in one piece.',
    )
  }

  const body = stripped.slice(0, stripped.length - padCount)
  if (urlSafe) {
    if (body.length % 4 === 1) {
      return badBase64(
        `That is not valid Base64 (URL-safe): it is ${body.length} characters long, which no Base64 value ever is. It was probably cut short when it was copied.`,
      )
    }
  } else if (stripped.length % 4 !== 0) {
    return badBase64(
      `That is not valid Base64: it is ${stripped.length} characters long, and Base64 comes in blocks of 4 padded with = on the end. Either it was cut short when it was copied, or it is the URL-safe kind, which leaves the = off.`,
    )
  }

  const bytes = new Uint8Array(Math.floor((body.length * 3) / 4))
  let buffer = 0
  let bits = 0
  let out = 0
  for (const c of body) {
    buffer = (buffer << 6) | alphabet.indexOf(c)
    bits += 6
    if (bits >= 8) {
      bits -= 8
      bytes[out++] = (buffer >> bits) & 0xff
    }
  }
  return { ok: true, bytes, error: null }
}

function badBase64(error: string): BytesResult {
  return { ok: false, bytes: new Uint8Array(0), error }
}

export function encodeBase64(text: string): ConversionResult {
  return ok(bytesToBase64(textToBytes(text), false))
}

export function decodeBase64(text: string): ConversionResult {
  const result = base64ToBytes(text, false)
  if (result.error !== null) return fail(result.error)
  return bytesToText(result.bytes)
}

export function encodeBase64Url(text: string): ConversionResult {
  return ok(bytesToBase64(textToBytes(text), true))
}

export function decodeBase64Url(text: string): ConversionResult {
  const result = base64ToBytes(text, true)
  if (result.error !== null) return fail(result.error)
  return bytesToText(result.bytes)
}

export function encodeUrl(text: string): ConversionResult {
  return ok(encodeURIComponent(text))
}

export function decodeUrl(text: string): ConversionResult {
  for (let i = 0; i < text.length; i++) {
    if (text[i] !== '%') continue
    // Checked by hand rather than by catching URIError, because the built-in
    // error has no wording a tech can act on.
    if (!/^[0-9a-fA-F]{2}$/.test(text.slice(i + 1, i + 3))) {
      return fail(
        'That is not valid URL encoding: a % has to be followed by two hex digits, like %20 for a space. Check the value was copied in full.',
      )
    }
  }
  try {
    return ok(decodeURIComponent(text))
  } catch {
    return fail(
      'Those % codes do not spell out readable text. The value may have been cut short, or it may not be URL encoded at all.',
    )
  }
}

export function encodeHex(text: string): ConversionResult {
  let out = ''
  for (const byte of textToBytes(text)) out += byte.toString(16).padStart(2, '0')
  return ok(out)
}

export function decodeHex(text: string): ConversionResult {
  const stripped = text.replace(/[ \t\r\n:-]/g, '')
  for (const c of stripped) {
    if (!/[0-9a-fA-F]/.test(c)) {
      return fail(
        `That is not valid Hex: it contains "${c}". Hex only uses the digits 0 to 9 and the letters A to F.`,
      )
    }
  }
  if (stripped.length % 2 !== 0) {
    return fail(
      `That is not valid Hex: it has ${stripped.length} digits, and hex uses two digits for every byte, so the total has to be an even number. One digit was probably lost when it was copied.`,
    )
  }
  const bytes = new Uint8Array(stripped.length / 2)
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(stripped.slice(i * 2, i * 2 + 2), 16)
  }
  return bytesToText(bytes)
}

export function encodeHtml(text: string): ConversionResult {
  return ok(
    text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;'),
  )
}

const NAMED_ENTITIES: Record<string, string> = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: '\u00a0',
}

export function decodeHtml(text: string): ConversionResult {
  let left = false
  const out = text.replace(/&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]*);/g, (whole, body: string) => {
    if (body[0] !== '#') {
      const named = NAMED_ENTITIES[body]
      if (named !== undefined) return named
      left = true
      return whole
    }
    const hex = body[1] === 'x' || body[1] === 'X'
    const code = parseInt(hex ? body.slice(2) : body.slice(1), hex ? 16 : 10)
    // Surrogates and anything past the last code point are not characters, and
    // String.fromCodePoint would throw on them.
    if (code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff)) {
      left = true
      return whole
    }
    return String.fromCodePoint(code)
  })
  return ok(out, left ? UNKNOWN_ENTITY_NOTE : null)
}

/** Dispatches to the ten functions above. 'jwt' is decode only and lives in
 *  jwt.ts, so convert() is never called with it. */
export function convert(
  text: string,
  conversion: Conversion,
  direction: Direction,
): ConversionResult {
  const encoding = direction === 'encode'
  switch (conversion) {
    case 'base64':
      return encoding ? encodeBase64(text) : decodeBase64(text)
    case 'base64url':
      return encoding ? encodeBase64Url(text) : decodeBase64Url(text)
    case 'url':
      return encoding ? encodeUrl(text) : decodeUrl(text)
    case 'hex':
      return encoding ? encodeHex(text) : decodeHex(text)
    default:
      return encoding ? encodeHtml(text) : decodeHtml(text)
  }
}
