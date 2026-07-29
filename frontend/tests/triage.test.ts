import test from 'node:test'
import assert from 'node:assert/strict'
import {
  ladderText,
  statusLabel,
  statusTone,
} from '../src/tools/internet-triage/ladder.ts'
import type { Rung } from '../src/tools/internet-triage/api.ts'

function rung(over: Partial<Rung>): Rung {
  return {
    id: 'adapter',
    step: 1,
    name: 'Network adapter',
    status: 'ok',
    detail: '',
    advice: '',
    target: '',
    ms: 0,
    ...over,
  }
}

test('statusTone maps every status to a dot colour', () => {
  assert.equal(statusTone('ok'), 'ok')
  assert.equal(statusTone('warn'), 'warn')
  assert.equal(statusTone('fail'), 'danger')
  assert.equal(statusTone('skipped'), 'idle')
  // An unknown status must not be tinted as a failure.
  assert.equal(statusTone('something else'), 'idle')
})

test('statusLabel names every status', () => {
  assert.equal(statusLabel('ok'), 'Passed')
  assert.equal(statusLabel('warn'), 'Worth checking')
  assert.equal(statusLabel('fail'), 'Failed')
  assert.equal(statusLabel('skipped'), 'Not checked')
  assert.equal(statusLabel(''), 'Not checked')
})

test('ladderText renders the block a tech pastes into a ticket', () => {
  // The expected string was produced by running the real function once and
  // pasting the result, column padding included.
  const got = ladderText([
    rung({ id: 'adapter', step: 1, name: 'Network adapter', status: 'ok', detail: '192.168.1.42 on wlan0' }),
    rung({ id: 'gateway', step: 2, name: 'Gateway', status: 'warn', detail: '192.168.1.1 did not answer' }),
    rung({ id: 'dns', step: 3, name: 'DNS', status: 'skipped', detail: 'not checked' }),
  ])
  assert.equal(
    got,
    '1. Network adapter    PASSED          192.168.1.42 on wlan0\n' +
      '2. Gateway            WORTH CHECKING  192.168.1.1 did not answer\n' +
      '3. DNS                NOT CHECKED     not checked',
  )
})

test('ladderText handles an empty ladder', () => {
  assert.equal(ladderText([]), '')
})
