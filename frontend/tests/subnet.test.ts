// The subnet calculator's arithmetic is the shipping implementation, so this is
// the suite that guards it. Expected values are the ones a published subnet
// calculator prints for the same input, so a regression shows up as a mismatch
// rather than as a plausible looking number.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import {
  broadcast,
  calculate,
  describe,
  groupDigits,
  parse,
  split,
  splitInto,
  totalAddresses,
  usableHosts,
  MAX_SPLIT_SUBNETS,
  type Prefix,
  type SubnetInfo,
} from '../src/tools/subnet-calculator/subnet.ts'

/** The prefix as typed, host bits kept: what Go's netip.Prefix.String() gives. */
function asTyped(p: Prefix): string {
  return describe(p).input
}

test('parse takes the forms a tech types', () => {
  const cases: Array<[string, string]> = [
    ['192.168.1.10/24', '192.168.1.10/24'],
    ['10.0.0.0/8', '10.0.0.0/8'],
    ['192.168.1.10 255.255.255.0', '192.168.1.10/24'],
    ['192.168.1.10/255.255.128.0', '192.168.1.10/17'],
    ['192.168.1.10\t255.255.255.252', '192.168.1.10/30'],
    ['  192.168.1.10   255.255.0.0  ', '192.168.1.10/16'],
    ['8.8.8.8', '8.8.8.8/32'],
    ['1.2.3.4 0.0.0.0', '1.2.3.4/0'],
    ['1.2.3.4 255.255.255.255', '1.2.3.4/32'],
    ['1.2.3.4/0', '1.2.3.4/0'],
    ['10.1.1.4/31', '10.1.1.4/31'],
    ['2001:db8::1/64', '2001:db8::1/64'],
    ['2001:db8::1', '2001:db8::1/128'],
    ['fe80::1%eth0/10', 'fe80::1/10'],
    ['::ffff:192.168.1.10/24', '192.168.1.10/24'],
  ]
  for (const [input, want] of cases) {
    assert.equal(asTyped(parse(input)), want, input)
  }
})

test('parse rejects what is not an address', () => {
  const cases = [
    '',
    '   ',
    'hello',
    '192.168.1.256/24',
    '192.168.1/24',
    '010.1.1.1/24',
    '192.168.1.1/33',
    '192.168.1.1/-1',
    '192.168.1.1/abc',
    '2001:db8::1/129',
    '192.168.1.1 255.0.255.0',
    '192.168.1.1 255.255.0.255',
    '192.168.1.1 255.255.256.0',
    '2001:db8::1 255.255.255.0',
    '192.168.1.1/24 please',
    '2001:db8:::1/64',
    '2001:db8::1::2/64',
  ]
  for (const input of cases) {
    assert.throws(() => parse(input), Error, input)
    // The message is shown to the user, so it has to be a written sentence.
    try {
      parse(input)
    } catch (err) {
      assert.match((err as Error).message, /\.$/, input)
    }
  }
})

interface DescribeCase {
  input: string
  network: string
  netmask: string
  wildcard: string
  broadcast: string
  firstHost: string
  lastHost: string
  total: string
  usable: string
  hostBits: number
  class: string
  scope: string
  private?: boolean
}

const DESCRIBE_CASES: DescribeCase[] = [
  {
    input: '192.168.1.10/24', network: '192.168.1.0', netmask: '255.255.255.0',
    wildcard: '0.0.0.255', broadcast: '192.168.1.255', firstHost: '192.168.1.1',
    lastHost: '192.168.1.254', total: '256', usable: '254', hostBits: 8,
    class: 'C', scope: 'Private (RFC 1918)', private: true,
  },
  {
    input: '172.16.5.1/16', network: '172.16.0.0', netmask: '255.255.0.0',
    wildcard: '0.0.255.255', broadcast: '172.16.255.255', firstHost: '172.16.0.1',
    lastHost: '172.16.255.254', total: '65536', usable: '65534', hostBits: 16,
    class: 'B', scope: 'Private (RFC 1918)', private: true,
  },
  {
    input: '10.20.30.40/8', network: '10.0.0.0', netmask: '255.0.0.0',
    wildcard: '0.255.255.255', broadcast: '10.255.255.255', firstHost: '10.0.0.1',
    lastHost: '10.255.255.254', total: '16777216', usable: '16777214', hostBits: 24,
    class: 'A', scope: 'Private (RFC 1918)', private: true,
  },
  {
    input: '192.168.1.10/30', network: '192.168.1.8', netmask: '255.255.255.252',
    wildcard: '0.0.0.3', broadcast: '192.168.1.11', firstHost: '192.168.1.9',
    lastHost: '192.168.1.10', total: '4', usable: '2', hostBits: 2,
    class: 'C', scope: 'Private (RFC 1918)', private: true,
  },
  {
    // RFC 3021: both addresses usable, no broadcast.
    input: '10.1.1.5/31', network: '10.1.1.4', netmask: '255.255.255.254',
    wildcard: '0.0.0.1', broadcast: '', firstHost: '10.1.1.4',
    lastHost: '10.1.1.5', total: '2', usable: '2', hostBits: 1,
    class: 'A', scope: 'Private (RFC 1918)', private: true,
  },
  {
    input: '8.8.8.8/32', network: '8.8.8.8', netmask: '255.255.255.255',
    wildcard: '0.0.0.0', broadcast: '', firstHost: '8.8.8.8',
    lastHost: '8.8.8.8', total: '1', usable: '1', hostBits: 0,
    class: 'A', scope: 'Public',
  },
  {
    input: '0.0.0.0/0', network: '0.0.0.0', netmask: '0.0.0.0',
    wildcard: '255.255.255.255', broadcast: '255.255.255.255', firstHost: '0.0.0.1',
    lastHost: '255.255.255.254', total: '4294967296', usable: '4294967294', hostBits: 32,
    class: 'A', scope: 'This network (RFC 1122)',
  },
  {
    // Boundary: the last address of the RFC 1918 172.16/12 block.
    input: '172.31.255.254/12', network: '172.16.0.0', netmask: '255.240.0.0',
    wildcard: '0.15.255.255', broadcast: '172.31.255.255', firstHost: '172.16.0.1',
    lastHost: '172.31.255.254', total: '1048576', usable: '1048574', hostBits: 20,
    class: 'B', scope: 'Private (RFC 1918)', private: true,
  },
  {
    // Boundary: one address past that block is public.
    input: '172.32.0.1/12', network: '172.32.0.0', netmask: '255.240.0.0',
    wildcard: '0.15.255.255', broadcast: '172.47.255.255', firstHost: '172.32.0.1',
    lastHost: '172.47.255.254', total: '1048576', usable: '1048574', hostBits: 20,
    class: 'B', scope: 'Public',
  },
  {
    input: '169.254.13.7/16', network: '169.254.0.0', netmask: '255.255.0.0',
    wildcard: '0.0.255.255', broadcast: '169.254.255.255', firstHost: '169.254.0.1',
    lastHost: '169.254.255.254', total: '65536', usable: '65534', hostBits: 16,
    class: 'B', scope: 'Link-local (APIPA)',
  },
  {
    input: '100.64.0.1/10', network: '100.64.0.0', netmask: '255.192.0.0',
    wildcard: '0.63.255.255', broadcast: '100.127.255.255', firstHost: '100.64.0.1',
    lastHost: '100.127.255.254', total: '4194304', usable: '4194302', hostBits: 22,
    class: 'A', scope: 'Carrier-grade NAT (RFC 6598)',
  },
  {
    input: '127.0.0.1/8', network: '127.0.0.0', netmask: '255.0.0.0',
    wildcard: '0.255.255.255', broadcast: '127.255.255.255', firstHost: '127.0.0.1',
    lastHost: '127.255.255.254', total: '16777216', usable: '16777214', hostBits: 24,
    class: 'A', scope: 'Loopback',
  },
  {
    input: '203.0.113.5/24', network: '203.0.113.0', netmask: '255.255.255.0',
    wildcard: '0.0.0.255', broadcast: '203.0.113.255', firstHost: '203.0.113.1',
    lastHost: '203.0.113.254', total: '256', usable: '254', hostBits: 8,
    class: 'C', scope: 'Documentation (TEST-NET-3)',
  },
  {
    input: '239.1.2.3/32', network: '239.1.2.3', netmask: '255.255.255.255',
    wildcard: '0.0.0.0', broadcast: '', firstHost: '239.1.2.3',
    lastHost: '239.1.2.3', total: '1', usable: '1', hostBits: 0,
    class: 'D (multicast)', scope: 'Multicast',
  },
  {
    // IPv6 has no broadcast, so every address counts as usable.
    input: '2001:db8::1/64', network: '2001:db8::', netmask: '', wildcard: '',
    broadcast: '', firstHost: '2001:db8::', lastHost: '2001:db8::ffff:ffff:ffff:ffff',
    total: '18446744073709551616', usable: '18446744073709551616', hostBits: 64,
    class: '', scope: 'Documentation',
  },
  {
    input: 'fe80::1234/10', network: 'fe80::', netmask: '', wildcard: '',
    broadcast: '', firstHost: 'fe80::', lastHost: 'febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff',
    total: '332306998946228968225951765070086144',
    usable: '332306998946228968225951765070086144',
    hostBits: 118, class: '', scope: 'Link-local',
  },
  {
    input: 'fd00:1234::5/128', network: 'fd00:1234::5', netmask: '', wildcard: '',
    broadcast: '', firstHost: 'fd00:1234::5', lastHost: 'fd00:1234::5',
    total: '1', usable: '1', hostBits: 0, class: '', scope: 'Unique local (ULA)',
    private: true,
  },
]

for (const tc of DESCRIBE_CASES) {
  test(`describe ${tc.input}`, () => {
    const got = calculate(tc.input)
    assert.equal(got.network, tc.network, 'network')
    assert.equal(got.netmask, tc.netmask, 'netmask')
    assert.equal(got.wildcard, tc.wildcard, 'wildcard')
    assert.equal(got.broadcast, tc.broadcast, 'broadcast')
    assert.equal(got.firstHost, tc.firstHost, 'firstHost')
    assert.equal(got.lastHost, tc.lastHost, 'lastHost')
    assert.equal(got.totalAddresses, tc.total, 'totalAddresses')
    assert.equal(got.usableHosts, tc.usable, 'usableHosts')
    assert.equal(got.class, tc.class, 'class')
    assert.equal(got.scope, tc.scope, 'scope')
    assert.equal(got.hostBits, tc.hostBits, 'hostBits')
    assert.equal(got.private, tc.private === true, 'private')
    assert.equal(got.input, tc.input, 'input')
  })
}

test('scope flags land on the right side of every boundary', () => {
  const cases: Array<[string, boolean, boolean, boolean, boolean]> = [
    ['10.0.0.1/8', true, false, false, false],
    ['192.168.0.1/16', true, false, false, false],
    ['192.167.255.255/16', false, false, false, false],
    ['192.169.0.1/16', false, false, false, false],
    ['9.255.255.255/8', false, false, false, false],
    ['11.0.0.0/8', false, false, false, false],
    ['127.255.255.254/8', false, true, false, false],
    ['128.0.0.1/8', false, false, false, false],
    ['169.254.0.0/16', false, false, true, false],
    ['169.253.255.255/16', false, false, false, false],
    ['100.63.255.255/10', false, false, false, false],
    ['100.127.255.255/10', false, false, false, true],
    ['100.128.0.0/10', false, false, false, false],
    ['::1/128', false, true, false, false],
    ['fe80::1/64', false, false, true, false],
    ['fc00::1/7', true, false, false, false],
    ['2606:4700::1111/32', false, false, false, false],
  ]
  for (const [input, isPrivate, loopback, linkLocal, cgnat] of cases) {
    const got = calculate(input)
    assert.deepEqual(
      [got.private, got.loopback, got.linkLocal, got.cgnat],
      [isPrivate, loopback, linkLocal, cgnat],
      input,
    )
  }
})

test('version, address and cidr', () => {
  const v4 = calculate('192.168.1.10 255.255.255.0')
  assert.equal(v4.version, 4)
  assert.equal(v4.address, '192.168.1.10')
  assert.equal(v4.cidr, '192.168.1.0/24')
  assert.equal(v4.prefixLength, 24)

  const v6 = calculate('2001:db8::1/64')
  assert.equal(v6.version, 6)
  assert.equal(v6.cidr, '2001:db8::/64')
})

test('the note explains the awkward prefix lengths', () => {
  const cases: Array<[string, string]> = [
    ['10.0.0.1/24', ''],
    ['10.0.0.1/30', 'A /30 gives 2 usable addresses, the classic point-to-point subnet.'],
    [
      '10.0.0.1/31',
      'A /31 is a point-to-point link (RFC 3021). Both addresses are usable and there is no broadcast address.',
    ],
    [
      '10.0.0.1/32',
      'A /32 is one address, a single host route. There is no network or broadcast address.',
    ],
  ]
  for (const [input, want] of cases) {
    assert.equal(calculate(input).note, want, input)
  }
})

test('netmask and wildcard for every interesting prefix length', () => {
  const cases: Array<[number, string, string]> = [
    [0, '0.0.0.0', '255.255.255.255'],
    [1, '128.0.0.0', '127.255.255.255'],
    [8, '255.0.0.0', '0.255.255.255'],
    [12, '255.240.0.0', '0.15.255.255'],
    [19, '255.255.224.0', '0.0.31.255'],
    [24, '255.255.255.0', '0.0.0.255'],
    [25, '255.255.255.128', '0.0.0.127'],
    [30, '255.255.255.252', '0.0.0.3'],
    [31, '255.255.255.254', '0.0.0.1'],
    [32, '255.255.255.255', '0.0.0.0'],
  ]
  for (const [bits, netmask, wildcard] of cases) {
    const got = calculate(`10.0.0.1/${bits}`)
    assert.equal(got.netmask, netmask, `netmask /${bits}`)
    assert.equal(got.wildcard, wildcard, `wildcard /${bits}`)
  }
})

test('every dotted mask survives the round trip back to a prefix length', () => {
  for (let bits = 0; bits <= 32; bits++) {
    const mask = calculate(`10.0.0.1/${bits}`).netmask
    assert.equal(parse(`10.0.0.1 ${mask}`).bits, bits, mask)
  }
})

test('there is no broadcast address where there should not be one', () => {
  for (const input of ['10.0.0.1/31', '10.0.0.1/32', '2001:db8::1/64', '2001:db8::1/128']) {
    assert.equal(broadcast(parse(input)), null, input)
  }
  assert.notEqual(broadcast(parse('10.0.0.1/30')), null)
})

test('host counts across all IPv4 prefix lengths', () => {
  for (let bits = 0; bits <= 32; bits++) {
    const p = parse(`10.0.0.0/${bits}`)
    const want = 1n << BigInt(32 - bits)
    assert.equal(totalAddresses(p), want, `total /${bits}`)
    assert.equal(usableHosts(p), bits >= 31 ? want : want - 2n, `usable /${bits}`)
  }
})

test('split carves a network into equal pieces', () => {
  const cases: Array<{ input: string; newBits: number; want: string[] }> = [
    {
      input: '192.168.1.0/24', newBits: 26,
      want: [
        '192.168.1.0/26 192.168.1.1-192.168.1.62 192.168.1.63 62',
        '192.168.1.64/26 192.168.1.65-192.168.1.126 192.168.1.127 62',
        '192.168.1.128/26 192.168.1.129-192.168.1.190 192.168.1.191 62',
        '192.168.1.192/26 192.168.1.193-192.168.1.254 192.168.1.255 62',
      ],
    },
    {
      input: '192.168.1.77/24', newBits: 25,
      want: [
        '192.168.1.0/25 192.168.1.1-192.168.1.126 192.168.1.127 126',
        '192.168.1.128/25 192.168.1.129-192.168.1.254 192.168.1.255 126',
      ],
    },
    {
      input: '172.16.0.0/16', newBits: 18,
      want: [
        '172.16.0.0/18 172.16.0.1-172.16.63.254 172.16.63.255 16382',
        '172.16.64.0/18 172.16.64.1-172.16.127.254 172.16.127.255 16382',
        '172.16.128.0/18 172.16.128.1-172.16.191.254 172.16.191.255 16382',
        '172.16.192.0/18 172.16.192.1-172.16.255.254 172.16.255.255 16382',
      ],
    },
    {
      input: '10.0.0.0/30', newBits: 31,
      want: ['10.0.0.0/31 10.0.0.0-10.0.0.1  2', '10.0.0.2/31 10.0.0.2-10.0.0.3  2'],
    },
    {
      input: '10.0.0.4/31', newBits: 32,
      want: ['10.0.0.4/32 10.0.0.4-10.0.0.4  1', '10.0.0.5/32 10.0.0.5-10.0.0.5  1'],
    },
    {
      input: '10.0.0.0/24', newBits: 24,
      want: ['10.0.0.0/24 10.0.0.1-10.0.0.254 10.0.0.255 254'],
    },
    {
      input: '255.255.255.240/28', newBits: 30,
      want: [
        '255.255.255.240/30 255.255.255.241-255.255.255.242 255.255.255.243 2',
        '255.255.255.244/30 255.255.255.245-255.255.255.246 255.255.255.247 2',
        '255.255.255.248/30 255.255.255.249-255.255.255.250 255.255.255.251 2',
        '255.255.255.252/30 255.255.255.253-255.255.255.254 255.255.255.255 2',
      ],
    },
    {
      input: '0.0.0.0/0', newBits: 1,
      want: [
        '0.0.0.0/1 0.0.0.1-127.255.255.254 127.255.255.255 2147483646',
        '128.0.0.0/1 128.0.0.1-255.255.255.254 255.255.255.255 2147483646',
      ],
    },
    {
      input: '2001:db8::/48', newBits: 50,
      want: [
        '2001:db8::/50 2001:db8::-2001:db8:0:3fff:ffff:ffff:ffff:ffff  302231454903657293676544',
        '2001:db8:0:4000::/50 2001:db8:0:4000::-2001:db8:0:7fff:ffff:ffff:ffff:ffff  302231454903657293676544',
        '2001:db8:0:8000::/50 2001:db8:0:8000::-2001:db8:0:bfff:ffff:ffff:ffff:ffff  302231454903657293676544',
        '2001:db8:0:c000::/50 2001:db8:0:c000::-2001:db8:0:ffff:ffff:ffff:ffff:ffff  302231454903657293676544',
      ],
    },
  ]

  for (const tc of cases) {
    const rows = split(parse(tc.input), tc.newBits)
    assert.equal(rows.length, tc.want.length, tc.input)
    rows.forEach((row, i) => {
      const got = `${row.cidr} ${row.firstHost}-${row.lastHost} ${row.broadcast} ${row.usableHosts}`
      assert.equal(got, tc.want[i], `${tc.input} subnet ${i + 1}`)
      assert.equal(row.index, i + 1)
    })
  }
})

test('split produces contiguous subnets with matching counts', () => {
  const rows = split(parse('10.0.0.0/16'), 24)
  assert.equal(rows.length, 256)
  rows.forEach((row, i) => {
    assert.equal(row.network, `10.0.${i}.0`)
    assert.equal(row.netmask, '255.255.255.0')
    assert.equal(row.totalAddresses, '256')
    assert.equal(row.usableHosts, '254')
  })
})

test('splitInto rounds up to the next power of two', () => {
  const cases: Array<[string, number, number, string]> = [
    ['192.168.0.0/24', 1, 1, '/24'],
    ['192.168.0.0/24', 2, 2, '/25'],
    ['192.168.0.0/24', 3, 4, '/26'],
    ['192.168.0.0/24', 4, 4, '/26'],
    ['192.168.0.0/24', 5, 8, '/27'],
    ['192.168.0.0/24', 8, 8, '/27'],
    ['10.0.0.0/8', 1000, 1024, '/18'],
    ['2001:db8::/32', 4, 4, '/34'],
  ]
  for (const [input, count, wantCount, wantBits] of cases) {
    const rows = splitInto(parse(input), count)
    assert.equal(rows.length, wantCount, `${input} into ${count}`)
    assert.ok(rows[0].cidr.endsWith(wantBits), `${rows[0].cidr} should end with ${wantBits}`)
  }
})

test('split refuses what it cannot list', () => {
  const cases: Array<[string, number]> = [
    ['192.168.1.0/24', 16],
    ['192.168.1.0/24', 33],
    ['192.168.1.0/24', -1],
    ['192.168.1.0/24', 24.5],
    ['10.0.0.0/8', 21],
    ['2001:db8::/32', 64],
    ['2001:db8::/32', 129],
  ]
  for (const [input, newBits] of cases) {
    assert.throws(() => split(parse(input), newBits), Error, `${input} into /${newBits}`)
  }

  for (const count of [0, -4, 8192, 512, 1.5]) {
    assert.throws(() => splitInto(parse('192.168.1.0/24'), count), Error, `into ${count}`)
  }
  assert.equal(splitInto(parse('10.0.0.0/8'), MAX_SPLIT_SUBNETS).length, MAX_SPLIT_SUBNETS)
})

// internal/subnet is the same arithmetic written independently in Go. Holding
// both to one golden file is what keeps this copy, the one that ships, honest.
test('every golden case matches the Go implementation field for field', () => {
  const path = fileURLToPath(new URL('../../testdata/subnet-cases.json', import.meta.url))
  const golden = JSON.parse(readFileSync(path, 'utf8')) as { cases: SubnetInfo[] }
  assert.ok(golden.cases.length >= 50, `only ${golden.cases.length} cases`)
  for (const want of golden.cases) {
    assert.deepEqual(calculate(want.input), want, want.input)
  }
})

test('groupDigits puts separators in the counts', () => {
  assert.equal(groupDigits('1'), '1')
  assert.equal(groupDigits('254'), '254')
  assert.equal(groupDigits('4294967296'), '4,294,967,296')
  assert.equal(groupDigits('18446744073709551616'), '18,446,744,073,709,551,616')
})
