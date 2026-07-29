import test from 'node:test'
import assert from 'node:assert/strict'
import { readBatch } from '../src/tools/bulk-renamer/journal.ts'

test('readBatch rejects what it cannot read', () => {
  const unreadable = [
    null,
    undefined,
    42,
    'text',
    [],
    {},
    { version: 1 },
    { version: 1, batch: null },
    { batch: {} },
    { batch: { folder: '', renames: [] } },
    { batch: { folder: 'x', renames: 'no' } },
  ]
  for (const raw of unreadable) {
    assert.equal(readBatch(raw), null, `expected null for ${JSON.stringify(raw ?? null)}`)
  }
})

test('readBatch reads a good document', () => {
  const batch = readBatch({
    version: 1,
    batch: {
      folder: 'C:\\Users\\tech\\Documents\\Scans',
      appliedAt: '2026-07-26T14:03:11Z',
      renames: [
        { from: 'SKMBT_0001.pdf', to: 'ACME-invoice-001.pdf' },
        { from: 'SKMBT_0002.pdf', to: 'ACME-invoice-002.pdf' },
      ],
    },
  })

  assert.notEqual(batch, null)
  assert.equal(batch?.folder, 'C:\\Users\\tech\\Documents\\Scans')
  assert.equal(batch?.appliedAt, '2026-07-26T14:03:11Z')
  assert.deepEqual(batch?.renames, [
    { from: 'SKMBT_0001.pdf', to: 'ACME-invoice-001.pdf' },
    { from: 'SKMBT_0002.pdf', to: 'ACME-invoice-002.pdf' },
  ])
})

test('readBatch drops bad entries', () => {
  const batch = readBatch({
    version: 1,
    batch: {
      folder: '/home/tech/scans',
      appliedAt: '2026-07-26T14:03:11Z',
      renames: [{ from: 'a.txt', to: 'b.txt' }, null, { from: 'a' }, { from: '', to: 'b' }],
    },
  })

  assert.deepEqual(batch?.renames, [{ from: 'a.txt', to: 'b.txt' }])
})

test('readBatch returns null when every entry was dropped', () => {
  const batch = readBatch({
    version: 1,
    batch: {
      folder: '/home/tech/scans',
      appliedAt: '2026-07-26T14:03:11Z',
      renames: [null, 7, { to: 'b.txt' }, { from: 'a.txt', to: '' }],
    },
  })

  assert.equal(batch, null)
})

test('readBatch tolerates a missing timestamp', () => {
  const missing = readBatch({
    version: 1,
    batch: { folder: '/scans', renames: [{ from: 'a.txt', to: 'b.txt' }] },
  })
  assert.equal(missing?.appliedAt, '')

  const numeric = readBatch({
    version: 1,
    batch: { folder: '/scans', appliedAt: 1712345678, renames: [{ from: 'a.txt', to: 'b.txt' }] },
  })
  assert.equal(numeric?.appliedAt, '')
})

test('readBatch ignores extra fields', () => {
  const batch = readBatch({
    version: 2,
    somethingNew: { deep: true },
    batch: {
      folder: '/scans',
      appliedAt: '2026-07-26T14:03:11Z',
      renames: [{ from: 'a.txt', to: 'b.txt', extra: 'ignored' }],
    },
  })

  assert.equal(batch?.folder, '/scans')
  assert.deepEqual(batch?.renames, [{ from: 'a.txt', to: 'b.txt' }])
})
