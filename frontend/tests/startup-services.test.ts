import test from 'node:test'
import assert from 'node:assert/strict'
import {
  concernCount,
  countLine,
  filterItems,
  startModeLabel,
  stateLabel,
} from '../src/tools/startup-services/items.ts'
import type { Item } from '../src/tools/startup-services/api.ts'

function item(over: Partial<Item> = {}): Item {
  return {
    name: 'Thing',
    kind: 'startup',
    source: 'HKCU Run',
    command: 'C:\\App\\thing.exe',
    publisher: '',
    startMode: 'automatic',
    state: '',
    enabled: true,
    concern: '',
    ...over,
  }
}

test('startModeLabel covers every value including the gap', () => {
  assert.equal(startModeLabel('automatic'), 'Automatic')
  assert.equal(startModeLabel('manual'), 'When needed')
  assert.equal(startModeLabel('disabled'), 'Disabled')
  assert.equal(startModeLabel('boot'), 'At boot')
  assert.equal(startModeLabel(''), 'not reported')
  assert.equal(startModeLabel('something-new'), 'not reported')
})

test('startModeLabel never returns an empty string', () => {
  for (const mode of ['automatic', 'manual', 'disabled', 'boot', '', 'nonsense']) {
    assert.notEqual(startModeLabel(mode), '', `mode ${mode}`)
  }
})

test('stateLabel is empty only when the OS did not say', () => {
  assert.equal(stateLabel('running'), 'Running')
  assert.equal(stateLabel('stopped'), 'Stopped')
  assert.equal(stateLabel(''), '')
  assert.equal(stateLabel('who knows'), '')
})

test('concernCount counts only the entries with a sentence', () => {
  assert.equal(concernCount([]), 0)
  assert.equal(concernCount([item(), item()]), 0)
  assert.equal(concernCount([item({ concern: 'Because.' }), item()]), 1)
})

test('countLine reads as a sentence', () => {
  const items = [
    ...Array.from({ length: 34 }, () => item({ kind: 'startup' })),
    ...Array.from({ length: 212 }, () => item({ kind: 'service' })),
  ]
  items[0] = item({ kind: 'startup', concern: 'Because.' })
  assert.equal(countLine(items), '34 startup entries and 212 services. 1 worth a look.')
})

test('countLine uses the singular forms', () => {
  assert.equal(
    countLine([item({ kind: 'startup' }), item({ kind: 'service' })]),
    '1 startup entry and 1 service.',
  )
})

test('countLine leaves out a zero concern count entirely', () => {
  const line = countLine([item(), item({ kind: 'service' })])
  assert.ok(!line.includes('worth a look'), line)
})

test('countLine handles an empty machine', () => {
  assert.equal(countLine([]), '0 startup entries and 0 services.')
})

const FIXTURE: Item[] = [
  item({ name: 'A', kind: 'startup', startMode: 'automatic' }),
  item({ name: 'B', kind: 'startup', startMode: 'disabled' }),
  item({ name: 'C', kind: 'service', startMode: 'automatic', concern: 'Because.' }),
  item({ name: 'D', kind: 'service', startMode: 'manual' }),
  item({ name: 'E', kind: 'service', startMode: 'boot' }),
]

test('filterItems keeps exactly the right rows', () => {
  const counts = (f: Parameters<typeof filterItems>[1]) => filterItems(FIXTURE, f).length
  assert.equal(counts('all'), 5)
  assert.equal(counts('startup'), 2)
  assert.equal(counts('services'), 3)
  // Automatic covers boot too: both start without anyone asking.
  assert.equal(counts('automatic'), 3)
  assert.equal(counts('concern'), 1)
})

test('filterItems returns the whole list for an unknown filter', () => {
  assert.equal(filterItems(FIXTURE, 'nonsense' as never).length, 5)
})

test('filterItems on an empty list is empty, not a crash', () => {
  assert.deepEqual(filterItems([], 'concern'), [])
})
