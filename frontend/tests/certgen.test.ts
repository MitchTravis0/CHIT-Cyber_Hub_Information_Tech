import test from 'node:test'
import assert from 'node:assert/strict'
import { fileNameFor, splitNames } from '../src/tools/cert-generator/api.ts'

test('splitNames turns the textarea into a list', () => {
  assert.deepEqual(splitNames('nas\n192.168.1.50'), ['nas', '192.168.1.50'])
  assert.deepEqual(splitNames('nas\r\n192.168.1.50'), ['nas', '192.168.1.50'])
  assert.deepEqual(splitNames('nas\r192.168.1.50'), ['nas', '192.168.1.50'])
  assert.deepEqual(splitNames('  nas  \n\n   \n other '), ['nas', 'other'])
  assert.deepEqual(splitNames(''), [])
  assert.deepEqual(splitNames('   \n  '), [])
  assert.deepEqual(splitNames('b\na\nc'), ['b', 'a', 'c'])
})

test('fileNameFor builds the three names', () => {
  assert.equal(fileNameFor('nas-branch-local', 'key'), 'nas-branch-local.key')
  assert.equal(fileNameFor('nas-branch-local', 'crt'), 'nas-branch-local.crt')
  assert.equal(fileNameFor('nas-branch-local', 'csr'), 'nas-branch-local.csr')
})
