import test from 'node:test'
import assert from 'node:assert/strict'
import {
  blockFor,
  MAX_REQUIREMENTS,
  MIN_BLOCK_BITS,
  parseRequirements,
  planSubnets,
  type Requirement,
} from '../src/tools/subnet-planner/plan.ts'
import { parse } from '../src/tools/subnet-calculator/subnet.ts'

const WORKED = ['Office PCs: 40', 'Printers: 12', 'Wi-Fi: 100', 'Link to branch: /30'].join('\n')

function planFor(parent: string, text: string) {
  const parsed = parseRequirements(text)
  assert.deepEqual(parsed.errors, [])
  return planSubnets(parent, parsed.requirements)
}

test('blockFor sizes every boundary', () => {
  assert.deepEqual(blockFor(1), { size: 4, bits: 30 })
  assert.deepEqual(blockFor(2), { size: 4, bits: 30 })
  assert.deepEqual(blockFor(3), { size: 8, bits: 29 })
  assert.deepEqual(blockFor(6), { size: 8, bits: 29 })
  assert.deepEqual(blockFor(7), { size: 16, bits: 28 })
  assert.deepEqual(blockFor(14), { size: 16, bits: 28 })
  assert.deepEqual(blockFor(15), { size: 32, bits: 27 })
  assert.deepEqual(blockFor(30), { size: 32, bits: 27 })
  assert.deepEqual(blockFor(31), { size: 64, bits: 26 })
  assert.deepEqual(blockFor(62), { size: 64, bits: 26 })
  assert.deepEqual(blockFor(254), { size: 256, bits: 24 })
  assert.deepEqual(blockFor(255), { size: 512, bits: 23 })
  assert.deepEqual(blockFor(65534), { size: 65536, bits: 16 })
})

test('blockFor never returns smaller than a /30', () => {
  assert.equal(MIN_BLOCK_BITS, 30)
  assert.equal(blockFor(1).bits, 30)
  assert.equal(blockFor(2).bits, 30)
})

test('blockFor holds what it promises', () => {
  for (let hosts = 1; hosts <= 5000; hosts++) {
    const { size, bits } = blockFor(hosts)
    assert.equal(size, 2 ** (32 - bits), `size and bits disagree at ${hosts}`)
    assert.ok(size - 2 >= hosts, `${bits} is too small for ${hosts} hosts`)
    if (bits < MIN_BLOCK_BITS) {
      // The next smaller block must not have fitted, or this one is wasteful.
      assert.ok(size / 2 - 2 < hosts, `${bits} is bigger than ${hosts} hosts needs`)
    }
  }
})

test('parseRequirements reads every line shape', () => {
  const parsed = parseRequirements(
    ['Office: 40', 'Office, 40', 'Office = 40', '40', 'Office: 40 hosts', 'Office: 40 users', 'Link: /30'].join('\n'),
  )
  assert.deepEqual(parsed.errors, [])
  assert.equal(parsed.requirements.length, 7)
  for (const index of [0, 1, 2, 4, 5]) {
    assert.equal(parsed.requirements[index].name, 'Office')
    assert.equal(parsed.requirements[index].hosts, 40)
    assert.equal(parsed.requirements[index].bits, 26)
    assert.equal(parsed.requirements[index].prefix, 0)
  }
  assert.equal(parsed.requirements[3].name, 'Subnet 4')
  assert.equal(parsed.requirements[3].hosts, 40)
  assert.deepEqual(
    { ...parsed.requirements[6] },
    { line: 7, name: 'Link', hosts: 0, prefix: 30, size: 4, bits: 30 },
  )
})

test('parseRequirements names an unnamed line', () => {
  const parsed = parseRequirements('40\n12')
  assert.deepEqual(parsed.requirements.map((r) => r.name), ['Subnet 1', 'Subnet 2'])
})

test('parseRequirements splits at the last separator', () => {
  const parsed = parseRequirements('Site A: Floor 2: 40')
  assert.equal(parsed.requirements[0].name, 'Site A: Floor 2')
  assert.equal(parsed.requirements[0].hosts, 40)
})

test('parseRequirements skips blanks and comments and still counts lines', () => {
  const parsed = parseRequirements('Office: 40\n# a comment\n\nBad: lots')
  assert.equal(parsed.requirements.length, 1)
  assert.deepEqual(parsed.errors, [
    'Line 4: "Bad: lots" does not end in a number of hosts or a size like /30.',
  ])
})

test('parseRequirements rejects each bad line with its exact message', () => {
  assert.deepEqual(parseRequirements('Office: lots').errors, [
    'Line 1: "Office: lots" does not end in a number of hosts or a size like /30.',
  ])
  assert.deepEqual(parseRequirements('Office: 40\nNone: 0').errors, [
    'Line 2: 0 hosts is not something to plan for. Ask for at least 1 host, or write /30 for a point-to-point link.',
  ])
  assert.deepEqual(parseRequirements('a: 1\nb: 1\nc: 1\nLink: /33').errors, [
    'Line 4: /33 is not an IPv4 prefix. They run from /0 to /32.',
  ])
  assert.deepEqual(parseRequirements('Huge: 5000000000').errors, [
    'Line 1: 5000000000 hosts will not fit in any IPv4 subnet. The largest, a /0, holds 4,294,967,294.',
  ])
})

test('parseRequirements caps at 64 lines', () => {
  assert.equal(MAX_REQUIREMENTS, 64)
  const many = Array.from({ length: 65 }, (_, i) => `Net ${i}: 2`).join('\n')
  const parsed = parseRequirements(many)
  assert.equal(parsed.requirements.length, 64)
  assert.deepEqual(parsed.errors, [
    'Only the first 64 lines were planned. That is as many subnets as this tool carves in one go.',
  ])

  const exact = Array.from({ length: 64 }, (_, i) => `Net ${i}: 2`).join('\n')
  assert.deepEqual(parseRequirements(exact).errors, [])
})

// Every expected value below was produced by python's ipaddress module first.
test('the worked example comes out exactly', () => {
  const plan = planFor('192.168.10.0/24', WORKED)
  assert.equal(plan.error, null)
  assert.deepEqual(
    plan.allocations.map((row) => [
      row.order,
      row.name,
      row.requested,
      row.cidr,
      row.netmask,
      `${row.firstHost} to ${row.lastHost}`,
      row.broadcast,
      row.usableHosts,
      row.spare,
    ]),
    [
      [1, 'Wi-Fi', '100 hosts', '192.168.10.0/25', '255.255.255.128', '192.168.10.1 to 192.168.10.126', '192.168.10.127', 126, 26],
      [2, 'Office PCs', '40 hosts', '192.168.10.128/26', '255.255.255.192', '192.168.10.129 to 192.168.10.190', '192.168.10.191', 62, 22],
      [3, 'Printers', '12 hosts', '192.168.10.192/28', '255.255.255.240', '192.168.10.193 to 192.168.10.206', '192.168.10.207', 14, 2],
      [4, 'Link to branch', '/30', '192.168.10.208/30', '255.255.255.252', '192.168.10.209 to 192.168.10.210', '192.168.10.211', 2, 0],
    ],
  )
  assert.deepEqual(plan.free.map((block) => block.cidr), [
    '192.168.10.212/30',
    '192.168.10.216/29',
    '192.168.10.224/27',
  ])
  assert.equal(plan.parentAddresses, 256)
  assert.equal(plan.usedAddresses, 212)
  assert.equal(plan.freeAddresses, 44)
  assert.equal(plan.note, '')
})

test('allocations are gap free and in address order', () => {
  const parents = ['192.168.10.0/24', '10.42.0.0/22', '172.16.0.0/20']
  const lists = [
    'a: 40\nb: 12\nc: 100\nd: /30',
    'a: 500\nb: 200\nc: 2\nd: 60\ne: 2',
    'a: 1000\nb: 1000\nc: 30\nd: 30\ne: /29',
  ]
  for (const parent of parents) {
    for (const list of lists) {
      const parsed = parseRequirements(list)
      const plan = planSubnets(parent, parsed.requirements)
      if (plan.error !== null) continue
      const start = parse(parent).value
      const total = 2n ** BigInt(32 - parse(parent).bits)
      let expected = start
      for (const row of plan.allocations) {
        const value = parse(row.cidr).value
        const size = 2n ** BigInt(32 - parse(row.cidr).bits)
        assert.equal(value, expected, `${parent} ${list}: ${row.cidr} is not where the last one ended`)
        assert.equal(value % size, 0n, `${row.cidr} is not aligned to its own size`)
        assert.ok(value >= start && value + size <= start + total, `${row.cidr} is outside ${parent}`)
        expected = value + size
      }
      assert.equal(Number(expected - start), plan.usedAddresses)
    }
  }
})

test('largest first, ties in input order', () => {
  const plan = planFor('192.168.0.0/23', 'small one: 10\nbig: 200\nsmall two: 10\nmiddle: 50')
  assert.deepEqual(plan.allocations.map((row) => row.name), [
    'big',
    'middle',
    'small one',
    'small two',
  ])
})

test('an exact fit leaves no free blocks', () => {
  const plan = planFor('192.168.10.0/24', 'a: 126\nb: 126')
  assert.deepEqual(plan.free, [])
  assert.equal(plan.freeAddresses, 0)
  assert.equal(plan.usedAddresses, 256)
})

test('one address over the size is rejected', () => {
  const parsed = parseRequirements('a: /25\nb: /25\nc: /30')
  const tooSmall = planSubnets('192.168.10.0/24', parsed.requirements)
  assert.equal(
    tooSmall.error,
    'These subnets need 260 addresses and 192.168.10.0/24 only has 256. Use a bigger network, or ask for fewer hosts.',
  )
  assert.deepEqual(tooSmall.allocations, [])

  const roomy = planSubnets('192.168.10.0/23', parsed.requirements)
  assert.equal(roomy.error, null)
  assert.equal(roomy.allocations.length, 3)
})

test('free space is the fewest whole blocks', () => {
  const empty = planFor('192.168.10.0/24', '')
  assert.deepEqual(empty.free.map((block) => block.cidr), ['192.168.10.0/24'])
  assert.equal(empty.error, null)
  assert.equal(empty.allocations.length, 0)
})

test('free blocks are aligned and total correctly', () => {
  for (const list of ['a: 40', 'a: 100\nb: 12', 'a: 2\nb: 2\nc: 2', 'a: 200\nb: 20\nc: 5']) {
    const plan = planFor('192.168.10.0/24', list)
    let previous = -1n
    let sum = 0
    for (const block of plan.free) {
      const value = parse(block.cidr).value
      const size = 2n ** BigInt(32 - parse(block.cidr).bits)
      assert.equal(value % size, 0n, `${block.cidr} is not aligned`)
      assert.ok(value > previous, `${block.cidr} is out of order`)
      assert.equal(Number(size), block.totalAddresses)
      previous = value
      sum += block.totalAddresses
    }
    assert.equal(sum, plan.freeAddresses, `free blocks do not add up for ${list}`)
  }
})

test('an IPv6 parent is refused', () => {
  const plan = planFor('2001:db8::/32', 'a: 40')
  assert.equal(
    plan.error,
    'Subnet planning here is IPv4 only. IPv6 links get a /64 each however many devices are on them, so carving one up by host count is not how IPv6 is addressed.',
  )
  assert.deepEqual(plan.allocations, [])
})

test('a parent with host bits is planned from its network', () => {
  const plan = planFor('10.42.0.9/22', 'a: 40')
  assert.equal(plan.parentCidr, '10.42.0.0/22')
  assert.equal(plan.note, '10.42.0.9/22 is inside 10.42.0.0/22, so the plan starts at 10.42.0.0.')
  assert.equal(plan.allocations[0].network, '10.42.0.0')
})

test('a bad parent passes the calculator own message through', () => {
  let thrown = ''
  try {
    parse('not-an-ip')
  } catch (err) {
    thrown = (err as Error).message
  }
  assert.notEqual(thrown, '')
  assert.equal(planFor('not-an-ip', 'a: 40').error, thrown)
})

test('an empty requirement list is not an error', () => {
  const plan = planSubnets('192.168.10.0/24', [] as Requirement[])
  assert.equal(plan.error, null)
  assert.deepEqual(plan.allocations, [])
  assert.equal(plan.usedAddresses, 0)
  assert.equal(plan.freeAddresses, 256)
})

test('a /32 requirement is a single host route', () => {
  const plan = planFor('192.168.10.0/24', 'loopback: /32')
  assert.equal(plan.allocations[0].cidr, '192.168.10.0/32')
  assert.equal(plan.allocations[0].usableHosts, 1)
  assert.equal(plan.allocations[0].spare, 0)
  assert.ok(plan.free.every((block) => block.usableHosts >= 0))
})
