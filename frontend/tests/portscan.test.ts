// The port scanner's tallies and its open-only view are derived in the
// frontend from the streamed results, so this is the arithmetic the summary
// line and the CSV are built on. The port spec parser is deliberately not
// mirrored here: Go owns it.
import test from 'node:test'
import assert from 'node:assert/strict'
import type { Result } from '../src/tools/port-scanner/api.ts'
import { mergePorts, tally, visibleRows } from '../src/tools/port-scanner/results.ts'
import { PRESETS } from '../src/tools/port-scanner/presets.ts'

function result(port: number, state: string, extra: Partial<Result> = {}): Result {
  return { port, state, service: '', banner: '', latencyMs: 0, ...extra }
}

test('mergePorts keeps the last result per port', () => {
  const merged = mergePorts([
    result(80, 'filtered'),
    result(443, 'open'),
    result(80, 'open', { latencyMs: 2.5 }),
  ])
  assert.equal(merged.size, 2)
  assert.equal(merged.get(80)?.state, 'open')
  assert.equal(merged.get(80)?.latencyMs, 2.5)
})

test('tally counts each state', () => {
  const merged = mergePorts([
    result(22, 'open'),
    result(80, 'open'),
    result(443, 'closed'),
    result(445, 'closed'),
    result(3389, 'closed'),
    result(9100, 'filtered'),
  ])
  assert.deepEqual(tally(merged), { total: 6, open: 2, closed: 3, filtered: 1 })
})

test('tally of an empty map is all zeroes', () => {
  assert.deepEqual(tally(mergePorts([])), { total: 0, open: 0, closed: 0, filtered: 0 })
})

test('visibleRows hides closed and filtered by default', () => {
  const merged = mergePorts([result(22, 'open'), result(80, 'closed'), result(443, 'filtered')])
  assert.deepEqual(
    visibleRows(merged, false).map((row) => row.port),
    [22],
  )
  assert.deepEqual(
    visibleRows(merged, true).map((row) => row.port),
    [22, 80, 443],
  )
})

test('visibleRows sorts ascending by port', () => {
  const merged = mergePorts([result(443, 'open'), result(22, 'open'), result(80, 'open')])
  assert.deepEqual(
    visibleRows(merged, false).map((row) => row.port),
    [22, 80, 443],
  )
})

test('presets are well formed', () => {
  const ids = new Set<string>()
  for (const preset of PRESETS) {
    assert.equal(ids.has(preset.id), false, `duplicate preset id ${preset.id}`)
    ids.add(preset.id)
    assert.notEqual(preset.name, '')
    assert.notEqual(preset.note, '')
    assert.match(preset.spec, /^\d+(-\d+)?(,\d+(-\d+)?)*$/)
  }
  assert.equal(PRESETS.length, 8)
})
