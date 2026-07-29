import { base64ToBytes, bytesToText } from './codecs'

export interface JwtTime {
  claim: 'exp' | 'iat' | 'nbf'
  /** 'Expires (exp)', 'Issued at (iat)', 'Not valid before (nbf)'. */
  label: string
  seconds: number
  /** new Date(seconds * 1000).toLocaleString() + ' (local time)'. */
  absolute: string
}

export interface JwtResult {
  ok: boolean
  /** Pretty-printed JSON, two space indent. '' when ok is false. */
  header: string
  payload: string
  /** The third block verbatim. '' when the token was not signed. */
  signature: string
  /** Present claims only, always in the order exp, iat, nbf. */
  times: JwtTime[]
  /** 'expired' | 'valid' | 'none'. Drives the status dot. */
  state: 'expired' | 'valid' | 'none'
  /** One sentence about expiry. '' when ok is false. */
  verdict: string
  /** The "not valid yet" sentence, or null. */
  notYet: string | null
  error: string | null
}

const CLAIM_LABELS: Array<{ claim: 'exp' | 'iat' | 'nbf'; label: string }> = [
  { claim: 'exp', label: 'Expires (exp)' },
  { claim: 'iat', label: 'Issued at (iat)' },
  { claim: 'nbf', label: 'Not valid before (nbf)' },
]

function empty(error: string | null): JwtResult {
  return {
    ok: false,
    header: '',
    payload: '',
    signature: '',
    times: [],
    state: 'none',
    verdict: '',
    notYet: null,
    error,
  }
}

function block(part: string, badBase64: string, badJson: string): { value: unknown; error: string | null } {
  const bytes = base64ToBytes(part, true)
  if (bytes.error !== null) return { value: null, error: badBase64 }
  const text = bytesToText(bytes.bytes)
  if (!text.ok) return { value: null, error: badBase64 }
  try {
    return { value: JSON.parse(text.text), error: null }
  } catch {
    return { value: null, error: badJson }
  }
}

function numberClaim(payload: unknown, name: string): number | null {
  if (payload === null || typeof payload !== 'object') return null
  const value = (payload as Record<string, unknown>)[name]
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

/** now is injected so the tests do not fight the clock. */
export function decodeJwt(token: string, now: Date): JwtResult {
  const trimmed = token.trim()
  // A token copied out of a request header arrives with its scheme attached.
  const value = (/^bearer /i.test(trimmed) ? trimmed.slice(7) : trimmed).trim()
  if (value === '') return empty(null)

  const parts = value.split('.')
  if (parts.length !== 3) {
    return empty(
      `That does not look like a token. A JWT is three blocks separated by full stops. This has ${parts.length}. Copy the whole value, including the signature on the end.`,
    )
  }

  const header = block(
    parts[0],
    'The first block of the token is not valid Base64, so the header could not be read. The value was probably cut short when it was copied.',
    "The token's header decoded, but it is not JSON. This may not be a JWT.",
  )
  if (header.error !== null) return empty(header.error)

  const payload = block(
    parts[1],
    'The second block of the token is not valid Base64, so the payload could not be read. The value was probably cut short when it was copied.',
    "The token's payload decoded, but it is not JSON. This may not be a JWT, or it may have been cut short when it was copied.",
  )
  if (payload.error !== null) return empty(payload.error)

  const times: JwtTime[] = []
  for (const { claim, label } of CLAIM_LABELS) {
    const seconds = numberClaim(payload.value, claim)
    if (seconds === null) continue
    times.push({
      claim,
      label,
      seconds,
      absolute: `${new Date(seconds * 1000).toLocaleString()} (local time)`,
    })
  }

  const nowMs = now.getTime()
  const exp = numberClaim(payload.value, 'exp')
  let state: JwtResult['state'] = 'none'
  let verdict = 'This token has no expiry time in it, so it does not expire on its own.'
  if (exp !== null && exp * 1000 < nowMs) {
    state = 'expired'
    verdict = `This token expired ${humanGap(nowMs - exp * 1000)} ago. Anything checking it properly will reject it.`
  } else if (exp !== null) {
    state = 'valid'
    verdict = `This token is still valid for another ${humanGap(exp * 1000 - nowMs)}.`
  }

  const nbf = numberClaim(payload.value, 'nbf')
  const notYet =
    nbf !== null && nbf * 1000 > nowMs
      ? `This token is not valid yet. It starts working in ${humanGap(nbf * 1000 - nowMs)} from now.`
      : null

  return {
    ok: true,
    header: JSON.stringify(header.value, null, 2),
    payload: JSON.stringify(payload.value, null, 2),
    signature: parts[2],
    times,
    state,
    verdict,
    notYet,
    error: null,
  }
}

/** 'less than a minute', '5 minutes', '1 hour', '3 days'. */
export function humanGap(ms: number): string {
  if (ms < 60000) return 'less than a minute'
  if (ms < 3600000) return plural(Math.round(ms / 60000), 'minute')
  if (ms < 86400000) return plural(Math.round(ms / 3600000), 'hour')
  return plural(Math.round(ms / 86400000), 'day')
}

function plural(count: number, unit: string): string {
  return `${count} ${unit}${count === 1 ? '' : 's'}`
}
