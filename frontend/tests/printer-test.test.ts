import test from 'node:test'
import assert from 'node:assert/strict'
import { DEFAULT_PORT, onlineLabel, parsePort } from '../src/tools/printer-test/format.ts'

test('parsePort defaults an empty field to the standard port', () => {
  // The literal 9100 is written in: it is the number in the hint a user reads.
  assert.equal(DEFAULT_PORT, 9100)
  assert.deepEqual(parsePort(''), { ok: true, port: 9100, error: '' })
  assert.deepEqual(parsePort('   '), { ok: true, port: 9100, error: '' })
})

test('parsePort accepts a real port', () => {
  assert.deepEqual(parsePort('9100'), { ok: true, port: 9100, error: '' })
  assert.deepEqual(parsePort('9101'), { ok: true, port: 9101, error: '' })
  assert.deepEqual(parsePort(' 9102 '), { ok: true, port: 9102, error: '' })
  assert.deepEqual(parsePort('1'), { ok: true, port: 1, error: '' })
  assert.deepEqual(parsePort('65535'), { ok: true, port: 65535, error: '' })
})

test('parsePort rejects anything that is not a port', () => {
  for (const bad of ['abc', '0', '65536', '-1', '91 00', '9100x', '9.1']) {
    const got = parsePort(bad)
    assert.equal(got.ok, false, `${bad} was accepted`)
    assert.equal(got.error, 'The port must be a number, usually 9100.')
  }
})

test('onlineLabel omits the row when the printer did not say', () => {
  assert.equal(onlineLabel('true'), 'Yes')
  assert.equal(onlineLabel('false'), 'No')
  assert.equal(onlineLabel(''), null)
  assert.equal(onlineLabel('maybe'), null)
})
