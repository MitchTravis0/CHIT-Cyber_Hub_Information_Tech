import test from 'node:test'
import assert from 'node:assert/strict'
import {
  lookupTone,
  selectorSentence,
  SPF_LOOKUP_LIMIT,
  SPF_LOOKUP_WARN_AT,
  verdictLabel,
  verdictTone,
} from '../src/tools/email-dns/format.ts'

test('verdictLabel names every level', () => {
  assert.equal(verdictLabel('ok'), 'Good')
  assert.equal(verdictLabel('warn'), 'Watch')
  assert.equal(verdictLabel('error'), 'Problem')
  assert.equal(verdictLabel('something else'), 'Problem')
})

test('verdictTone maps every level to a dot colour', () => {
  assert.equal(verdictTone('ok'), 'ok')
  assert.equal(verdictTone('warn'), 'warn')
  assert.equal(verdictTone('error'), 'danger')
  assert.equal(verdictTone(''), 'danger')
})

test('selectorSentence says how many were checked', () => {
  assert.equal(
    selectorSentence(['default', 'google']),
    'Selectors checked: default, google (2 in all).',
  )
  assert.equal(selectorSentence(['default']), 'Selectors checked: default (1 in all).')
})

test('lookupTone turns amber approaching the limit and red past it', () => {
  // The literals are written in rather than read from the constants: these are
  // the numbers in the sentence a user reads.
  assert.equal(SPF_LOOKUP_LIMIT, 10)
  assert.equal(SPF_LOOKUP_WARN_AT, 8)

  assert.equal(lookupTone(0), 'text-fg-muted')
  assert.equal(lookupTone(7), 'text-fg-muted')
  assert.equal(lookupTone(8), 'text-warn')
  assert.equal(lookupTone(9), 'text-warn')
  assert.equal(lookupTone(10), 'text-warn')
  assert.equal(lookupTone(11), 'text-danger')
  assert.equal(lookupTone(30), 'text-danger')
})
