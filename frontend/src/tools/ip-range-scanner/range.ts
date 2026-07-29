// Mirror of ParseRange in internal/netscan/iprange.go. The backend stays the
// authority: this copy exists so the grid can be drawn, and a typo caught,
// before the scan is started.

export const MAX_ADDRESSES = 65536

export interface IPRange {
  /** Inclusive, as unsigned 32 bit numbers. */
  start: number
  end: number
  count: number
  /** Canonical form, e.g. "192.168.1.0/24". */
  text: string
}

export type RangeResult = { ok: true; range: IPRange } | { ok: false; error: string }

// Go's netip rejects a leading zero because 010 is octal to some tools, so
// this rejects it too. The two parsers have to agree or the grid is drawn for
// a range the backend then refuses to scan.
const OCTET = /^(0|[1-9]\d{0,2})$/

export function ipToNumber(ip: string): number | null {
  const parts = ip.split('.')
  if (parts.length !== 4) return null
  let value = 0
  for (const part of parts) {
    if (!OCTET.test(part)) return null
    const octet = Number(part)
    if (octet > 255) return null
    value = value * 256 + octet
  }
  return value
}

export function numberToIP(value: number): string {
  return [(value >>> 24) & 255, (value >>> 16) & 255, (value >>> 8) & 255, value & 255].join('.')
}

/** Last octet of an address, the number shown on a grid cell. */
export function lastOctet(value: number): number {
  return value & 255
}

export function parseRange(input: string): RangeResult {
  const text = input.trim()
  if (text === '') {
    return { ok: false, error: 'Type something to scan, for example 192.168.1.0/24 or 192.168.1.1-254.' }
  }
  if (text.includes(':')) {
    return {
      ok: false,
      error: `"${text}" looks like IPv6. The scanner works on IPv4 ranges such as 192.168.1.0/24.`,
    }
  }
  if (text.includes('/')) return parseCIDR(text)
  if (text.includes('-')) return parseStartEnd(text)
  return parseSingle(text)
}

function build(start: number, end: number, text: string): RangeResult {
  const count = end - start + 1
  if (count > MAX_ADDRESSES) {
    return {
      ok: false,
      error: `That covers ${count} addresses. Scan at most ${MAX_ADDRESSES} at a time (a /16), for example 192.168.1.0/24.`,
    }
  }
  return { ok: true, range: { start, end, count, text } }
}

function parseCIDR(text: string): RangeResult {
  // No trimming around the slash: netip.ParsePrefix takes the whole string as
  // it stands, so "192.168.1.0 /24" has to fail here as well.
  const [addrPart, bitsPart, ...rest] = text.split('/')
  const addr = ipToNumber(addrPart)
  const bits = Number(bitsPart)
  if (addr === null || rest.length > 0 || !OCTET.test(bitsPart) || bits > 32) {
    return { ok: false, error: `"${text}" is not a valid network. Use a form like 192.168.1.0/24.` }
  }

  const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0
  let first = (addr & mask) >>> 0
  let last = (first | (~mask >>> 0)) >>> 0
  // A /31 is a two-address point-to-point link and a /32 is one host, so there
  // is nothing to trim there.
  if (bits <= 30) {
    first += 1
    last -= 1
  }
  return build(first, last, `${numberToIP((addr & mask) >>> 0)}/${bits}`)
}

function parseStartEnd(text: string): RangeResult {
  const cut = text.indexOf('-')
  const rawStart = text.slice(0, cut).trim()
  const rawEnd = text.slice(cut + 1).trim()

  const start = ipToNumber(rawStart)
  if (start === null) return notAnAddress(text)
  if (rawEnd === '') {
    return { ok: false, error: `"${text}" is missing the end of the range. Try 192.168.1.1-254.` }
  }

  let end: number | null
  if (rawEnd.includes('.')) {
    end = ipToNumber(rawEnd)
    if (end === null) return notAnAddress(text)
  } else {
    if (!OCTET.test(rawEnd) || Number(rawEnd) > 255) {
      return {
        ok: false,
        error: `"${rawEnd}" is not a valid end for the range. Use a last number from 0 to 255, or a full address.`,
      }
    }
    end = ((start & 0xffffff00) >>> 0) + Number(rawEnd)
  }

  if (end < start) {
    return {
      ok: false,
      error: `${numberToIP(end)} comes before ${numberToIP(start)}, so that range is backwards. Swap the two addresses.`,
    }
  }
  return build(start, end, `${numberToIP(start)}-${numberToIP(end)}`)
}

function parseSingle(text: string): RangeResult {
  const addr = ipToNumber(text)
  if (addr === null) return notAnAddress(text)
  return { ok: true, range: { start: addr, end: addr, count: 1, text: numberToIP(addr) } }
}

function notAnAddress(text: string): RangeResult {
  return {
    ok: false,
    error: `"${text}" is not an address CHIT understands. Try 192.168.1.0/24, 192.168.1.1-254 or 192.168.1.50.`,
  }
}
