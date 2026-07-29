/**
 * IPv4 and basic IPv6 subnet arithmetic, mirroring internal/subnet in Go.
 *
 * The UI recalculates on every keystroke, so the maths runs here rather than
 * over the Wails bridge. The Go package stays the source of truth for anything
 * the backend does (the IP range scanner and friends) and the two are kept in
 * step field for field: same names, same values, same wording.
 *
 * Addresses are held as bigint so IPv4 and IPv6 share one code path.
 */

export interface Prefix {
  /** The address as typed, host bits included. */
  value: bigint
  bits: number
  v6: boolean
}

export interface SubnetInfo {
  input: string
  version: number
  address: string
  cidr: string
  prefixLength: number
  netmask: string
  wildcard: string
  network: string
  broadcast: string
  firstHost: string
  lastHost: string
  hostBits: number
  totalAddresses: string
  usableHosts: string
  class: string
  scope: string
  private: boolean
  loopback: boolean
  linkLocal: boolean
  cgnat: boolean
  note: string
}

export interface SubnetRow {
  index: number
  cidr: string
  network: string
  netmask: string
  broadcast: string
  firstHost: string
  lastHost: string
  usableHosts: string
  totalAddresses: string
}

/** Listing more than this is never what a tech wants. */
export const MAX_SPLIT_SUBNETS = 4096

const V4_OCTET = /^(0|[1-9]\d{0,2})$/
const HEX_GROUP = /^[0-9a-fA-F]{1,4}$/

function bitLen(v6: boolean): number {
  return v6 ? 128 : 32
}

function parseAddr(text: string): { value: bigint; v6: boolean } | null {
  if (text.includes(':')) return parseV6(text)
  return parseV4(text)
}

function parseV4(text: string): { value: bigint; v6: boolean } | null {
  const parts = text.split('.')
  if (parts.length !== 4) return null
  let value = 0n
  for (const part of parts) {
    // Leading zeros are rejected: 010 is ambiguous (octal to some tools).
    if (!V4_OCTET.test(part)) return null
    const n = Number(part)
    if (n > 255) return null
    value = (value << 8n) | BigInt(n)
  }
  return { value, v6: false }
}

/** Splits one side of an address into 16 bit groups. A trailing dotted quad
 *  ("::ffff:192.168.1.1") counts as two groups. */
function v6Groups(text: string, allowV4: boolean): string[] | null {
  if (text === '') return []
  const parts = text.split(':')
  const groups: string[] = []
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (part.includes('.')) {
      if (!allowV4 || i !== parts.length - 1) return null
      const v4 = parseV4(part)
      if (v4 === null) return null
      groups.push(((v4.value >> 16n) & 0xffffn).toString(16), (v4.value & 0xffffn).toString(16))
      continue
    }
    if (!HEX_GROUP.test(part)) return null
    groups.push(part)
  }
  return groups
}

function parseV6(text: string): { value: bigint; v6: boolean } | null {
  const at = text.indexOf('::')
  if (at >= 0 && text.indexOf('::', at + 2) >= 0) return null

  const head = v6Groups(at < 0 ? text : text.slice(0, at), at < 0)
  const tail = at < 0 ? [] : v6Groups(text.slice(at + 2), true)
  if (head === null || tail === null) return null

  // "::" stands for at least one group of zeros, so a full address never has it.
  if (at < 0 ? head.length !== 8 : head.length + tail.length >= 8) return null

  const ordered = [
    ...head,
    ...Array<string>(8 - head.length - tail.length).fill('0'),
    ...tail,
  ]
  let value = 0n
  for (const group of ordered) value = (value << 16n) | BigInt(`0x${group}`)
  return { value, v6: true }
}

function formatAddr(value: bigint, v6: boolean): string {
  if (!v6) {
    return [24n, 16n, 8n, 0n].map((shift) => Number((value >> shift) & 0xffn)).join('.')
  }
  const groups: number[] = []
  for (let i = 7; i >= 0; i--) groups.push(Number((value >> BigInt(i * 16)) & 0xffffn))

  // RFC 5952: compress the longest run of two or more zero groups, leftmost wins.
  let bestStart = -1
  let bestLen = 0
  let start = -1
  for (let i = 0; i <= 8; i++) {
    if (i < 8 && groups[i] === 0) {
      if (start < 0) start = i
      continue
    }
    if (start >= 0) {
      const len = i - start
      if (len > bestLen) {
        bestStart = start
        bestLen = len
      }
      start = -1
    }
  }
  const text = groups.map((g) => g.toString(16))
  if (bestLen < 2) return text.join(':')
  return `${text.slice(0, bestStart).join(':')}::${text.slice(bestStart + bestLen).join(':')}`
}

function netmaskValue(bits: number, v6: boolean): bigint {
  const total = bitLen(v6)
  return ((1n << BigInt(bits)) - 1n) << BigInt(total - bits)
}

function hostMask(bits: number, v6: boolean): bigint {
  return (1n << BigInt(bitLen(v6) - bits)) - 1n
}

/** The network address: the prefix with its host bits cleared. */
export function networkValue(p: Prefix): bigint {
  return p.value & netmaskValue(p.bits, p.v6)
}

/** The highest address in the prefix. */
export function lastAddress(p: Prefix): bigint {
  return networkValue(p) | hostMask(p.bits, p.v6)
}

export function totalAddresses(p: Prefix): bigint {
  return 1n << BigInt(bitLen(p.v6) - p.bits)
}

/**
 * IPv4 networks of /30 and wider lose the network and broadcast addresses. A
 * /31 is a point-to-point link with both addresses usable (RFC 3021), a /32 is
 * a single host, and IPv6 has no broadcast address at all.
 */
export function usableHosts(p: Prefix): bigint {
  const total = totalAddresses(p)
  return !p.v6 && p.bits <= 30 ? total - 2n : total
}

/** Null when the network has no broadcast address. */
export function broadcast(p: Prefix): bigint | null {
  return p.v6 || p.bits >= 31 ? null : lastAddress(p)
}

export function firstHost(p: Prefix): bigint {
  const network = networkValue(p)
  return p.v6 || p.bits >= 31 ? network : network + 1n
}

export function lastHost(p: Prefix): bigint {
  const last = lastAddress(p)
  return p.v6 || p.bits >= 31 ? last : last - 1n
}

/** Dotted-decimal mask length, or null when the mask is not a run of 1 bits. */
function maskToBits(text: string): number | null {
  const parsed = parseV4(text)
  if (parsed === null) return null
  const value = parsed.value
  let bits = 0
  while (bits < 32 && (value & (1n << BigInt(31 - bits))) !== 0n) bits++
  return value === netmaskValue(bits, false) ? bits : null
}

/**
 * Parses the forms a tech actually types:
 * "192.168.1.10/24", "192.168.1.10 255.255.255.0", "192.168.1.10/255.255.255.0",
 * "192.168.1.10" (a single host) and "2001:db8::1/64".
 *
 * Throws an Error whose message is meant to be shown to the user.
 */
export function parse(input: string): Prefix {
  const text = input.trim()
  if (text === '') throw new Error('Enter an IP address, for example 192.168.1.10/24.')

  const at = text.search(/[/ \t]/)
  const hostText = at < 0 ? text : text.slice(0, at)
  const maskText = at < 0 ? '' : text.slice(at + 1).trim()

  const parsed = parseAddr(hostText.replace(/%.*$/, ''))
  if (parsed === null) {
    throw new Error(`"${hostText}" is not a valid IP address. Try something like 192.168.1.10/24.`)
  }
  let { value, v6 } = parsed
  // ::ffff:1.2.3.4 is an IPv4 address wearing an IPv6 costume.
  if (v6 && value >> 32n === 0xffffn) {
    value &= 0xffffffffn
    v6 = false
  }

  const maxBits = bitLen(v6)
  if (maskText === '') return { value, bits: maxBits, v6 }

  if (/[.:]/.test(maskText)) {
    if (v6) {
      throw new Error(
        `IPv6 networks use a prefix length such as /64, not a mask like ${maskText}.`,
      )
    }
    const bits = maskToBits(maskText)
    if (bits === null) {
      throw new Error(
        `${maskText} is not a valid subnet mask. A mask is a run of 1 bits followed by 0 bits, like 255.255.240.0.`,
      )
    }
    return { value, bits, v6 }
  }

  if (!/^\d+$/.test(maskText) || Number(maskText) > maxBits) {
    throw new Error(
      `"${maskText}" is not a valid prefix length. For IPv${v6 ? 6 : 4} it must be between /0 and /${maxBits}.`,
    )
  }
  return { value, bits: Number(maskText), v6 }
}

const SCOPE_RULES: Array<{
  prefix: string
  label: string
  private?: boolean
  loopback?: boolean
  linkLocal?: boolean
  cgnat?: boolean
}> = [
  { prefix: '0.0.0.0/8', label: 'This network (RFC 1122)' },
  { prefix: '10.0.0.0/8', label: 'Private (RFC 1918)', private: true },
  { prefix: '100.64.0.0/10', label: 'Carrier-grade NAT (RFC 6598)', cgnat: true },
  { prefix: '127.0.0.0/8', label: 'Loopback', loopback: true },
  { prefix: '169.254.0.0/16', label: 'Link-local (APIPA)', linkLocal: true },
  { prefix: '172.16.0.0/12', label: 'Private (RFC 1918)', private: true },
  { prefix: '192.0.2.0/24', label: 'Documentation (TEST-NET-1)' },
  { prefix: '192.168.0.0/16', label: 'Private (RFC 1918)', private: true },
  { prefix: '198.18.0.0/15', label: 'Benchmarking (RFC 2544)' },
  { prefix: '198.51.100.0/24', label: 'Documentation (TEST-NET-2)' },
  { prefix: '203.0.113.0/24', label: 'Documentation (TEST-NET-3)' },
  { prefix: '255.255.255.255/32', label: 'Limited broadcast' },
  { prefix: '224.0.0.0/4', label: 'Multicast' },
  { prefix: '240.0.0.0/4', label: 'Reserved (RFC 1112)' },
  { prefix: '::/128', label: 'Unspecified' },
  { prefix: '::1/128', label: 'Loopback', loopback: true },
  { prefix: '2001:db8::/32', label: 'Documentation' },
  { prefix: 'fc00::/7', label: 'Unique local (ULA)', private: true },
  { prefix: 'fe80::/10', label: 'Link-local', linkLocal: true },
  { prefix: 'ff00::/8', label: 'Multicast' },
]

// Most specific first: the first match wins.
const SCOPES = SCOPE_RULES.map((rule) => ({ ...rule, parsed: parse(rule.prefix) }))

function scopeOf(value: bigint, v6: boolean) {
  for (const rule of SCOPES) {
    if (rule.parsed.v6 !== v6) continue
    if ((value & netmaskValue(rule.parsed.bits, v6)) === networkValue(rule.parsed)) {
      return {
        scope: rule.label,
        private: rule.private === true,
        loopback: rule.loopback === true,
        linkLocal: rule.linkLocal === true,
        cgnat: rule.cgnat === true,
      }
    }
  }
  return {
    scope: v6 ? 'Global unicast' : 'Public',
    private: false,
    loopback: false,
    linkLocal: false,
    cgnat: false,
  }
}

/** The legacy IPv4 class letter, still quoted in vendor documentation. */
function classOf(value: bigint): string {
  const first = Number(value >> 24n)
  if (first < 128) return 'A'
  if (first < 192) return 'B'
  if (first < 224) return 'C'
  if (first < 240) return 'D (multicast)'
  return 'E (reserved)'
}

function noteFor(p: Prefix): string {
  if (p.v6) {
    return p.bits === 128
      ? 'A /128 is one address, a single host.'
      : 'IPv6 has no broadcast address, so every address in the range is usable.'
  }
  if (p.bits === 32) {
    return 'A /32 is one address, a single host route. There is no network or broadcast address.'
  }
  if (p.bits === 31) {
    return 'A /31 is a point-to-point link (RFC 3021). Both addresses are usable and there is no broadcast address.'
  }
  if (p.bits === 30) return 'A /30 gives 2 usable addresses, the classic point-to-point subnet.'
  return ''
}

export function describe(p: Prefix): SubnetInfo {
  const network = networkValue(p)
  const bcast = broadcast(p)
  const scope = scopeOf(p.value, p.v6)
  return {
    input: `${formatAddr(p.value, p.v6)}/${p.bits}`,
    version: p.v6 ? 6 : 4,
    address: formatAddr(p.value, p.v6),
    cidr: `${formatAddr(network, p.v6)}/${p.bits}`,
    prefixLength: p.bits,
    netmask: p.v6 ? '' : formatAddr(netmaskValue(p.bits, false), false),
    wildcard: p.v6 ? '' : formatAddr(hostMask(p.bits, false), false),
    network: formatAddr(network, p.v6),
    broadcast: bcast === null ? '' : formatAddr(bcast, p.v6),
    firstHost: formatAddr(firstHost(p), p.v6),
    lastHost: formatAddr(lastHost(p), p.v6),
    hostBits: bitLen(p.v6) - p.bits,
    totalAddresses: totalAddresses(p).toString(),
    usableHosts: usableHosts(p).toString(),
    class: p.v6 ? '' : classOf(p.value),
    ...scope,
    note: noteFor(p),
  }
}

/** parse + describe in one step. Throws on invalid input. */
export function calculate(input: string): SubnetInfo {
  const info = describe(parse(input))
  return { ...info, input: input.trim() }
}

/** Carves p into equal subnets of length newBits, in address order. */
export function split(p: Prefix, newBits: number): SubnetRow[] {
  const maxBits = bitLen(p.v6)
  const network = networkValue(p)
  const cidr = `${formatAddr(network, p.v6)}/${p.bits}`
  if (!Number.isInteger(newBits) || newBits < p.bits || newBits > maxBits) {
    throw new Error(
      `Cannot split ${cidr} into /${newBits}. The new prefix must be between /${p.bits} and /${maxBits}.`,
    )
  }
  const steps = newBits - p.bits
  if (steps > 12 || 1 << steps > MAX_SPLIT_SUBNETS) {
    throw new Error(
      `Splitting ${cidr} into /${newBits} subnets would list more than ${MAX_SPLIT_SUBNETS} networks. Pick a shorter new prefix.`,
    )
  }

  const count = 1 << steps
  const step = 1n << BigInt(maxBits - newBits)
  const rows: SubnetRow[] = []
  for (let i = 0; i < count; i++) {
    const sub: Prefix = { value: network + BigInt(i) * step, bits: newBits, v6: p.v6 }
    const bcast = broadcast(sub)
    rows.push({
      index: i + 1,
      cidr: `${formatAddr(sub.value, p.v6)}/${newBits}`,
      network: formatAddr(sub.value, p.v6),
      netmask: p.v6 ? '' : formatAddr(netmaskValue(newBits, false), false),
      broadcast: bcast === null ? '' : formatAddr(bcast, p.v6),
      firstHost: formatAddr(firstHost(sub), p.v6),
      lastHost: formatAddr(lastHost(sub), p.v6),
      usableHosts: usableHosts(sub).toString(),
      totalAddresses: totalAddresses(sub).toString(),
    })
  }
  return rows
}

/** Carves p into at least count equal subnets. Equal subnets only come in
 *  powers of two, so asking for 3 gives 4. */
export function splitInto(p: Prefix, count: number): SubnetRow[] {
  if (!Number.isInteger(count) || count < 1) {
    throw new Error('Enter how many subnets you need, for example 4.')
  }
  let steps = 0
  while (1 << steps < count) {
    steps++
    if (steps > 12) {
      throw new Error(
        `${count} subnets is more than the ${MAX_SPLIT_SUBNETS} this tool lists. Ask for fewer.`,
      )
    }
  }
  return split(p, p.bits + steps)
}

/** 16777214 -> "16,777,214". Works on the digit strings the counts are held in. */
export function groupDigits(digits: string): string {
  return digits.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}
