// The DNS Lookup page only sorts and filters what the backend sends it, so
// these three functions are all the logic that lives in the frontend.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { DnsRecord } from '../src/tools/dns-lookup/api.ts'
import {
  answerCount,
  sortRecords,
  visibleRecords,
} from '../src/tools/dns-lookup/records.ts'

function record(server: string, type: string, value: string, extra: Partial<DnsRecord> = {}): DnsRecord {
  return {
    server,
    type,
    name: 'example.com',
    value,
    priority: 0,
    status: 'ok',
    message: '',
    queryMs: 1,
    ...extra,
  }
}

test('sortRecords groups by server in first-seen order', () => {
  const sorted = sortRecords([
    record('8.8.8.8', 'A', '10.0.0.1'),
    record('1.1.1.1', 'A', '10.0.0.1'),
    record('8.8.8.8', 'MX', 'mail.example.com'),
    record('1.1.1.1', 'MX', 'mail.example.com'),
  ])
  assert.deepEqual(
    sorted.map((row) => row.server),
    ['8.8.8.8', '8.8.8.8', '1.1.1.1', '1.1.1.1'],
  )
})

test('sortRecords orders types by TYPE_ORDER, not alphabetically', () => {
  const sorted = sortRecords([
    record('8.8.8.8', 'NS', 'ns1.example.com'),
    record('8.8.8.8', 'TXT', 'v=spf1 -all'),
    record('8.8.8.8', 'AAAA', '2001:db8::1'),
    record('8.8.8.8', 'MX', 'mail.example.com'),
    record('8.8.8.8', 'A', '10.0.0.1'),
  ])
  assert.deepEqual(
    sorted.map((row) => row.type),
    ['A', 'AAAA', 'MX', 'TXT', 'NS'],
  )
})

test('sortRecords orders equal type and server by value', () => {
  const sorted = sortRecords([
    record('8.8.8.8', 'A', '10.0.0.3'),
    record('8.8.8.8', 'A', '10.0.0.1'),
    record('8.8.8.8', 'A', '10.0.0.2'),
  ])
  assert.deepEqual(
    sorted.map((row) => row.value),
    ['10.0.0.1', '10.0.0.2', '10.0.0.3'],
  )
})

test('sortRecords is stable for identical keys', () => {
  const sorted = sortRecords([
    record('8.8.8.8', 'TXT', 'v=spf1 -all', { queryMs: 11 }),
    record('8.8.8.8', 'TXT', 'v=spf1 -all', { queryMs: 22 }),
  ])
  assert.deepEqual(
    sorted.map((row) => row.queryMs),
    [11, 22],
  )
})

test('sortRecords leaves the input array alone', () => {
  const input = [record('8.8.8.8', 'MX', 'mail.example.com'), record('8.8.8.8', 'A', '10.0.0.1')]
  sortRecords(input)
  assert.deepEqual(
    input.map((row) => row.type),
    ['MX', 'A'],
  )
})

const mixed: DnsRecord[] = [
  record('8.8.8.8', 'A', '10.0.0.1'),
  record('8.8.8.8', 'MX', '', { status: 'empty', message: 'No MX record for example.com according to 8.8.8.8.' }),
  record('192.0.2.1', 'A', '', { status: 'error', message: '192.0.2.1 did not answer within 3000 ms.' }),
]

test('visibleRecords hides empty rows when asked', () => {
  const visible = visibleRecords(mixed, false)
  assert.deepEqual(
    visible.map((row) => row.status),
    ['ok', 'error'],
  )
})

test('visibleRecords always keeps error rows', () => {
  assert.equal(visibleRecords(mixed, true).length, 3)
  assert.equal(visibleRecords(mixed, true).filter((row) => row.status === 'error').length, 1)
  assert.equal(visibleRecords(mixed, false).filter((row) => row.status === 'error').length, 1)
})

test('answerCount counts only ok rows', () => {
  const records = [
    record('8.8.8.8', 'A', '10.0.0.1'),
    record('8.8.8.8', 'A', '10.0.0.2'),
    record('8.8.8.8', 'MX', '', { status: 'empty', message: 'No MX record.' }),
    record('8.8.8.8', 'NS', '', { status: 'empty', message: 'No NS record.' }),
    record('192.0.2.1', 'A', '', { status: 'error', message: 'No answer.' }),
  ]
  assert.equal(answerCount(records), 2)
  assert.equal(answerCount([]), 0)
})
