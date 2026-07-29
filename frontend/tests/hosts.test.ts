// The scanner's grid, tallies and "where can I put a static IP" answer are all
// derived here from the streamed results, so this is the arithmetic a tech acts
// on. It runs entirely in the frontend, hence the tests.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Host } from '../src/tools/ip-range-scanner/api.ts'
import {
  cellStates,
  firstFreeAddress,
  largestFreeBlock,
  mergeHosts,
  tally,
  FREE,
  PENDING,
  USED,
} from '../src/tools/ip-range-scanner/hosts.ts'
import { parseRange, type IPRange } from '../src/tools/ip-range-scanner/range.ts'

function range(text: string): IPRange {
  const got = parseRange(text)
  if (!got.ok) throw new Error(got.error)
  return got.range
}

function host(ip: string, extra: Partial<Host> = {}): Host {
  return {
    ip,
    status: 'up',
    respondedVia: 'icmp',
    latencyMs: 1,
    hostname: '',
    mac: '',
    vendor: '',
    openPorts: [],
    ...extra,
  }
}

test('the last record for an address wins', () => {
  // The ARP stage re-emits a host to promote it or to fill in its MAC.
  const merged = mergeHosts([
    host('10.0.0.1', { status: 'down', respondedVia: '' }),
    host('10.0.0.2'),
    host('10.0.0.1', { respondedVia: 'arp', mac: 'AA:BB:CC:00:11:22' }),
  ])
  assert.equal(merged.size, 2)
  assert.equal(merged.get('10.0.0.1')?.status, 'up')
  assert.equal(merged.get('10.0.0.1')?.mac, 'AA:BB:CC:00:11:22')
})

test('cells are pending until an answer arrives', () => {
  const r = range('10.0.0.1-10.0.0.4')
  const states = cellStates(r, mergeHosts([host('10.0.0.2'), host('10.0.0.3', { status: 'down' })]))
  assert.deepEqual(Array.from(states), [PENDING, USED, FREE, PENDING])
})

test('the tally counts what answered and how', () => {
  const r = range('10.0.0.1-10.0.0.5')
  const byIP = mergeHosts([
    host('10.0.0.1', { respondedVia: 'icmp' }),
    host('10.0.0.2', { respondedVia: 'tcp', mac: 'AA:BB:CC:00:11:22' }),
    host('10.0.0.3', { respondedVia: 'arp', mac: 'AA:BB:CC:00:11:33' }),
    host('10.0.0.4', { status: 'down', respondedVia: '' }),
  ])
  const counts = tally(r, cellStates(r, byIP), byIP)
  assert.deepEqual(counts, {
    total: 5,
    scanned: 4,
    used: 3,
    free: 1,
    viaIcmp: 1,
    viaTcp: 1,
    viaArp: 1,
    withMac: 2,
  })
})

test('the largest free block is the longest run of silent addresses', () => {
  const r = range('10.0.0.1-10.0.0.10')
  const down = (ip: string) => host(ip, { status: 'down', respondedVia: '' })
  const byIP = mergeHosts([
    down('10.0.0.1'),
    host('10.0.0.2'),
    down('10.0.0.3'),
    down('10.0.0.4'),
    down('10.0.0.5'),
    host('10.0.0.6'),
    down('10.0.0.9'),
    down('10.0.0.10'),
  ])
  const states = cellStates(r, byIP)
  assert.deepEqual(largestFreeBlock(r, states), {
    start: '10.0.0.3',
    end: '10.0.0.5',
    count: 3,
  })
  // 10.0.0.7 and .8 are still pending, so they break the run rather than
  // joining it: the answer never over-promises mid-scan.
  assert.equal(firstFreeAddress(r, states), '10.0.0.1')
})

test('nothing free yet reads as nothing free', () => {
  const r = range('10.0.0.1-10.0.0.3')
  const states = cellStates(r, mergeHosts([host('10.0.0.1'), host('10.0.0.2')]))
  assert.equal(largestFreeBlock(r, states), null)
  assert.equal(firstFreeAddress(r, states), null)
})

test('a free run reaching the end of the range still counts', () => {
  const r = range('10.0.0.1-10.0.0.4')
  const byIP = mergeHosts([
    host('10.0.0.1'),
    host('10.0.0.2', { status: 'down', respondedVia: '' }),
    host('10.0.0.3', { status: 'down', respondedVia: '' }),
    host('10.0.0.4', { status: 'down', respondedVia: '' }),
  ])
  assert.deepEqual(largestFreeBlock(r, cellStates(r, byIP)), {
    start: '10.0.0.2',
    end: '10.0.0.4',
    count: 3,
  })
})

test('addresses are indexed from the start of the range, not from zero', () => {
  const r = range('192.168.1.250-192.168.2.2')
  const byIP = mergeHosts([host('192.168.2.0', { status: 'down', respondedVia: '' })])
  const states = cellStates(r, byIP)
  assert.equal(states.length, 9)
  assert.equal(states[6], FREE)
  assert.equal(firstFreeAddress(r, states), '192.168.2.0')
})
