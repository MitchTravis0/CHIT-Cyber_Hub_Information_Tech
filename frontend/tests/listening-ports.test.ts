import test from 'node:test'
import assert from 'node:assert/strict'
import {
  countLine,
  entryId,
  filterEntries,
  isReachable,
  protocolLabel,
  reachLabel,
  reachTone,
  sortEntries,
} from '../src/tools/listening-ports/entries.ts'
import type { Entry } from '../src/tools/listening-ports/api.ts'

function entry(over: Partial<Entry> = {}): Entry {
  return {
    protocol: 'tcp',
    address: '0.0.0.0',
    port: 80,
    reach: 'everywhere',
    pid: 0,
    process: '',
    service: '',
    source: '/proc/net',
    ...over,
  }
}

// Six rows: 4 TCP (2 of them IPv6), 2 UDP; 4 reachable, 2 local only.
const FIXTURE: Entry[] = [
  entry({ protocol: 'tcp', address: '0.0.0.0', port: 22, reach: 'everywhere' }),
  entry({ protocol: 'tcp', address: '127.0.0.1', port: 631, reach: 'local' }),
  entry({ protocol: 'tcp6', address: '::', port: 22, reach: 'everywhere' }),
  entry({ protocol: 'tcp6', address: '::1', port: 631, reach: 'local' }),
  entry({ protocol: 'udp', address: '0.0.0.0', port: 5353, reach: 'everywhere' }),
  entry({ protocol: 'udp6', address: '::', port: 5353, reach: 'everywhere' }),
]

test('protocolLabel names all four and passes anything else through', () => {
  assert.equal(protocolLabel('tcp'), 'TCP')
  assert.equal(protocolLabel('tcp6'), 'TCP (IPv6)')
  assert.equal(protocolLabel('udp'), 'UDP')
  assert.equal(protocolLabel('udp6'), 'UDP (IPv6)')
  assert.equal(protocolLabel('sctp'), 'SCTP')
})

test('reachLabel defaults an unknown value to the exposed reading, not the safe one', () => {
  assert.equal(reachLabel('everywhere'), 'Everywhere')
  assert.equal(reachLabel('local'), 'Local only')
  assert.equal(reachLabel('one'), 'One address')
  assert.equal(reachLabel('something new'), 'One address')
  assert.equal(reachLabel(''), 'One address')
})

test('reachTone is green only for a socket no other machine can reach', () => {
  assert.equal(reachTone('local'), 'ok')
  assert.equal(reachTone('everywhere'), 'warn')
  assert.equal(reachTone('one'), 'warn')
  assert.equal(reachTone(''), 'warn')
})

test('isReachable treats anything but local as reachable', () => {
  assert.equal(isReachable(entry({ reach: 'local' })), false)
  assert.equal(isReachable(entry({ reach: 'everywhere' })), true)
  assert.equal(isReachable(entry({ reach: 'one' })), true)
})

test('countLine reports the split and the exposure', () => {
  assert.equal(countLine(FIXTURE), '6 listening: 4 TCP, 2 UDP. 4 are reachable from other machines.')
  assert.equal(countLine([]), 'Nothing is listening on this machine.')
  assert.equal(
    countLine([entry({ protocol: 'tcp', reach: 'everywhere' })]),
    '1 listening: 1 TCP, 0 UDP. 1 is reachable from other machines.',
  )
  // Nothing reachable drops the second sentence rather than saying "0 are".
  assert.equal(
    countLine([entry({ protocol: 'tcp', address: '127.0.0.1', reach: 'local' })]),
    '1 listening: 1 TCP, 0 UDP.',
  )
})

test('filterEntries returns exactly the right rows, not merely the right ones among others', () => {
  assert.equal(filterEntries(FIXTURE, 'all').length, 6)
  assert.equal(filterEntries(FIXTURE, 'tcp').length, 4)
  assert.equal(filterEntries(FIXTURE, 'udp').length, 2)
  assert.equal(filterEntries(FIXTURE, 'reachable').length, 4)

  for (const row of filterEntries(FIXTURE, 'udp')) {
    assert.equal(row.protocol.startsWith('udp'), true)
  }
  for (const row of filterEntries(FIXTURE, 'reachable')) {
    assert.notEqual(row.reach, 'local')
  }
})

test('sortEntries orders by port, then TCP before UDP, then IPv4 before IPv6', () => {
  const sorted = sortEntries(FIXTURE)
  const keys = sorted.map((row) => `${row.port} ${row.protocol} ${row.address}`)
  assert.deepEqual(keys, [
    '22 tcp 0.0.0.0',
    '22 tcp6 ::',
    '631 tcp 127.0.0.1',
    '631 tcp6 ::1',
    '5353 udp 0.0.0.0',
    '5353 udp6 ::',
  ])
})

test('sortEntries does not mutate the array it was given', () => {
  const original = FIXTURE.map((row) => row.port)
  sortEntries(FIXTURE)
  assert.deepEqual(
    FIXTURE.map((row) => row.port),
    original,
  )
})

test('entryId separates the IPv4 and IPv6 sockets on the same port', () => {
  assert.notEqual(
    entryId(entry({ protocol: 'tcp', address: '0.0.0.0', port: 22 })),
    entryId(entry({ protocol: 'tcp6', address: '::', port: 22 })),
  )
})
