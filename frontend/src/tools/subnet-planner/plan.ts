/**
 * Carving one IPv4 network into subnets that fit a list of host counts (VLSM).
 *
 * The address maths is imported from the shipped Subnet Calculator rather than
 * copied: that module is pinned against Go's internal/subnet by
 * testdata/subnet-cases.json, and a second copy here would be free to drift
 * away from it. Nothing in that folder is edited.
 */

import {
  describe,
  groupDigits,
  networkValue,
  parse,
  totalAddresses,
  type Prefix,
} from '../subnet-calculator/subnet'

export interface Requirement {
  /** 1-based line number in the text the user typed. */
  line: number
  name: string
  /** Hosts asked for, or 0 when a prefix was given instead. */
  hosts: number
  /** Prefix asked for, or 0 when a host count was given instead. */
  prefix: number
  /** Addresses the block must hold. Always a power of two. */
  size: number
  /** The prefix length that gives `size` addresses. */
  bits: number
}

export interface ParsedRequirements {
  requirements: Requirement[]
  /** One sentence per unusable line, in line order. */
  errors: string[]
}

export interface Allocation {
  /** 1-based, in address order, which is also allocation order. */
  order: number
  name: string
  /** What the user asked for, as typed back: '40 hosts' or '/30'. */
  requested: string
  /** Hosts asked for, 0 when a prefix was asked for. Drives the row tint. */
  hosts: number
  cidr: string
  netmask: string
  network: string
  firstHost: string
  lastHost: string
  broadcast: string
  usableHosts: number
  /** Usable addresses beyond what was asked for. 0 when a prefix was asked for. */
  spare: number
}

export interface FreeBlock {
  cidr: string
  totalAddresses: number
  usableHosts: number
}

export interface Plan {
  allocations: Allocation[]
  free: FreeBlock[]
  /** The parent as a network, e.g. '10.42.0.0/22'. '' when the parent is bad. */
  parentCidr: string
  parentAddresses: number
  usedAddresses: number
  freeAddresses: number
  /** Shown above the table. '' when there is nothing to say. */
  note: string
  /** The whole plan failed. Null when it worked. */
  error: string | null
}

/** The most lines this tool plans in one go. */
export const MAX_REQUIREMENTS = 64
/** The smallest block it will hand out: a /30, the classic point-to-point. */
export const MIN_BLOCK_BITS = 30

const MAX_IPV4_HOSTS = 4294967294

const IPV6_REFUSED =
  'Subnet planning here is IPv4 only. IPv6 links get a /64 each however many devices are on them, so carving one up by host count is not how IPv6 is addressed.'

export function blockFor(hosts: number): { size: number; bits: number } {
  let bits = MIN_BLOCK_BITS
  while (2 ** (32 - bits) - 2 < hosts && bits > 0) bits--
  return { size: 2 ** (32 - bits), bits }
}

export function parseRequirements(text: string): ParsedRequirements {
  const requirements: Requirement[] = []
  const errors: string[] = []
  const lines = text.split(/\r\n|\r|\n/)

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (line === '' || line.startsWith('#')) continue
    const at = Math.max(line.lastIndexOf(':'), line.lastIndexOf(','), line.lastIndexOf('='))
    const lineNo = i + 1

    if (requirements.length >= MAX_REQUIREMENTS) {
      errors.push(
        `Only the first ${MAX_REQUIREMENTS} lines were planned. That is as many subnets as this tool carves in one go.`,
      )
      break
    }

    let name = at < 0 ? '' : line.slice(0, at).trim()
    const amount = (at < 0 ? line : line.slice(at + 1))
      .trim()
      .replace(/\s*(hosts?|users?|devices?|addresses)$/i, '')
      .trim()
    if (name === '') name = `Subnet ${requirements.length + 1}`

    const asPrefix = /^\/(\d+)$/.exec(amount)
    if (asPrefix !== null) {
      const prefix = Number(asPrefix[1])
      if (prefix > 32) {
        errors.push(`Line ${lineNo}: /${prefix} is not an IPv4 prefix. They run from /0 to /32.`)
        continue
      }
      requirements.push({
        line: lineNo,
        name,
        hosts: 0,
        prefix,
        size: 2 ** (32 - prefix),
        bits: prefix,
      })
      continue
    }

    if (!/^\d+$/.test(amount)) {
      errors.push(
        `Line ${lineNo}: "${line}" does not end in a number of hosts or a size like /30.`,
      )
      continue
    }

    const hosts = Number(amount)
    if (hosts < 1) {
      errors.push(
        `Line ${lineNo}: ${hosts} hosts is not something to plan for. Ask for at least 1 host, or write /30 for a point-to-point link.`,
      )
      continue
    }
    if (hosts > MAX_IPV4_HOSTS) {
      errors.push(
        `Line ${lineNo}: ${hosts} hosts will not fit in any IPv4 subnet. The largest, a /0, holds ${groupDigits(String(MAX_IPV4_HOSTS))}.`,
      )
      continue
    }

    const block = blockFor(hosts)
    requirements.push({ line: lineNo, name, hosts, prefix: 0, size: block.size, bits: block.bits })
  }

  return { requirements, errors }
}

function emptyPlan(error: string | null): Plan {
  return {
    allocations: [],
    free: [],
    parentCidr: '',
    parentAddresses: 0,
    usedAddresses: 0,
    freeAddresses: 0,
    note: '',
    error,
  }
}

function freeBlocks(from: bigint, end: bigint): FreeBlock[] {
  const out: FreeBlock[] = []
  let cursor = from
  while (cursor < end) {
    let size = 1n
    let bits = 32
    while (size * 2n <= end - cursor && cursor % (size * 2n) === 0n) {
      size *= 2n
      bits--
    }
    const info = describe({ value: cursor, bits, v6: false })
    out.push({
      cidr: info.cidr,
      totalAddresses: Number(size),
      usableHosts: Number(info.usableHosts),
    })
    cursor += size
  }
  return out
}

export function planSubnets(parentText: string, requirements: Requirement[]): Plan {
  let prefix: Prefix
  try {
    prefix = parse(parentText)
  } catch (err) {
    return emptyPlan(err instanceof Error ? err.message : String(err))
  }
  if (prefix.v6) return emptyPlan(IPV6_REFUSED)

  const info = describe(prefix)
  const start = networkValue(prefix)
  const total = totalAddresses(prefix)
  const note =
    info.address === info.network
      ? ''
      : `${info.input} is inside ${info.cidr}, so the plan starts at ${info.network}.`

  let needed = 0n
  for (const requirement of requirements) needed += BigInt(requirement.size)
  if (needed > total) {
    return {
      ...emptyPlan(
        `These subnets need ${groupDigits(String(needed))} addresses and ${info.cidr} only has ${groupDigits(String(total))}. Use a bigger network, or ask for fewer hosts.`,
      ),
      parentCidr: info.cidr,
      parentAddresses: Number(total),
      note,
    }
  }

  // Largest first is what keeps the allocation gap free: every block size
  // divides the one before it, so each lands on its own boundary.
  const sorted = [...requirements].sort((a, b) => b.size - a.size)
  const allocations: Allocation[] = []
  let cursor = start
  for (let i = 0; i < sorted.length; i++) {
    const requirement = sorted[i]
    const block = describe({ value: cursor, bits: requirement.bits, v6: false })
    const usableHosts = Number(block.usableHosts)
    allocations.push({
      order: i + 1,
      name: requirement.name,
      requested:
        requirement.hosts > 0
          ? `${requirement.hosts} host${requirement.hosts === 1 ? '' : 's'}`
          : `/${requirement.prefix}`,
      hosts: requirement.hosts,
      cidr: block.cidr,
      netmask: block.netmask,
      network: block.network,
      firstHost: block.firstHost,
      lastHost: block.lastHost,
      broadcast: block.broadcast,
      usableHosts,
      spare: requirement.hosts > 0 ? usableHosts - requirement.hosts : 0,
    })
    cursor += BigInt(requirement.size)
  }

  return {
    allocations,
    free: freeBlocks(cursor, start + total),
    parentCidr: info.cidr,
    parentAddresses: Number(total),
    usedAddresses: Number(needed),
    freeAddresses: Number(total - needed),
    note,
    error: null,
  }
}
