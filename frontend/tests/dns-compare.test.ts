import test from 'node:test'
import assert from 'node:assert/strict'
import {
  agreementLabel,
  agreementTone,
  comparisonText,
  csvNameFor,
  defaultTicks,
  MAX_SERVERS,
} from '../src/tools/dns-compare/compare.ts'
import type { DnsAnswer, ServerOption } from '../src/tools/dns-compare/api.ts'

function row(over: Partial<DnsAnswer>): DnsAnswer {
  return {
    server: '1.1.1.1',
    label: '1.1.1.1',
    values: [],
    status: 'ok',
    message: '',
    queryMs: 0,
    inStep: true,
    ...over,
  }
}

test('agreementLabel checks the status before the flag', () => {
  assert.equal(agreementLabel(row({ status: 'ok', inStep: true })), 'Agrees')
  assert.equal(agreementLabel(row({ status: 'ok', inStep: false })), 'Out of step')
  assert.equal(agreementLabel(row({ status: 'empty', inStep: false })), 'Out of step')
  assert.equal(agreementLabel(row({ status: 'empty', inStep: true })), 'Agrees')
  // An errored row carries inStep true because it has not disagreed about
  // anything. Reading the flag first would label it "Agrees", which is wrong.
  assert.equal(agreementLabel(row({ status: 'error', inStep: true })), 'No answer')
})

test('agreementTone never tints a failed resolver as agreeing', () => {
  assert.equal(agreementTone(row({ status: 'ok', inStep: true })), 'ok')
  assert.equal(agreementTone(row({ status: 'ok', inStep: false })), 'warn')
  assert.equal(agreementTone(row({ status: 'error', inStep: true })), 'idle')
})

test('csvNameFor is safe as a file name', () => {
  assert.equal(csvNameFor('example.com', 'A'), 'dns-compare-example-com-A')
  assert.equal(csvNameFor('mail.sub.example.co.uk', 'MX'), 'dns-compare-mail-sub-example-co-uk-MX')
})

test('defaultTicks stays inside the backend cap', () => {
  // The literal 8 is written in: reading MAX_SERVERS on both sides would pin
  // nothing at all.
  assert.equal(MAX_SERVERS, 8)

  const many: ServerOption[] = [
    { id: '', label: 'System resolver', detail: '' },
    ...Array.from({ length: 12 }, (_, i) => ({
      id: `10.0.0.${i}`,
      label: `10.0.0.${i}`,
      detail: '',
    })),
  ]
  const got = defaultTicks(many)
  assert.equal(got.length, 8)
  assert.deepEqual(got, [
    '',
    '10.0.0.0',
    '10.0.0.1',
    '10.0.0.2',
    '10.0.0.3',
    '10.0.0.4',
    '10.0.0.5',
    '10.0.0.6',
  ])
})

test('defaultTicks offers Quad9 but leaves it unticked', () => {
  const options: ServerOption[] = [
    { id: '', label: 'System resolver', detail: '' },
    { id: '8.8.8.8', label: '8.8.8.8', detail: '' },
    { id: '1.1.1.1', label: '1.1.1.1', detail: '' },
    { id: '9.9.9.9', label: '9.9.9.9', detail: '' },
  ]
  assert.deepEqual(defaultTicks(options), ['', '8.8.8.8', '1.1.1.1'])
})

test('comparisonText writes one line per resolver', () => {
  const got = comparisonText([
    row({ label: '1.1.1.1', values: ['93.184.216.34'], queryMs: 12.4 }),
    row({ label: '8.8.8.8', values: [], status: 'empty', queryMs: 30.6 }),
    row({ label: 'dc01', values: [], status: 'error', queryMs: 0 }),
  ])
  assert.equal(
    got,
    '1.1.1.1  93.184.216.34  (12 ms)\n' + '8.8.8.8  no record  (31 ms)\n' + 'dc01  no answer  (0 ms)',
  )
})
