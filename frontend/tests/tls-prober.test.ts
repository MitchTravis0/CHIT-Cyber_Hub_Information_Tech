import test from 'node:test'
import assert from 'node:assert/strict'
import {
  csvNameFor,
  reportText,
  resultLabel,
  resultTone,
} from '../src/tools/tls-prober/format.ts'
import type { TlsAttempt } from '../src/tools/tls-prober/api.ts'

function attempt(over: Partial<TlsAttempt>): TlsAttempt {
  return {
    version: 'TLS 1.2',
    testable: true,
    accepted: false,
    cipher: '',
    alpn: '',
    message: '',
    handshakeMs: 0,
    ...over,
  }
}

test('resultLabel covers accepted, refused and not testable', () => {
  assert.equal(resultLabel(attempt({ accepted: true })), 'Accepted')
  assert.equal(resultLabel(attempt({ accepted: false })), 'Refused')
  // SSL 3.0: not testable and not accepted, which must not read as "Refused".
  assert.equal(
    resultLabel(attempt({ version: 'SSL 3.0', testable: false, accepted: false })),
    'Not testable',
  )
})

test('resultTone never colours an untestable row as a failure', () => {
  assert.equal(resultTone(attempt({ accepted: true })), 'ok')
  assert.equal(resultTone(attempt({ accepted: false })), 'danger')
  assert.equal(resultTone(attempt({ testable: false, accepted: false })), 'idle')
})

test('csvNameFor strips what a file name cannot hold', () => {
  assert.equal(csvNameFor('mail.example.com', 443), 'tls-mail-example-com-443')
  assert.equal(csvNameFor('192.168.1.10', 8443), 'tls-192-168-1-10-8443')
  assert.equal(csvNameFor('2606:4700::1111', 443), 'tls-2606-4700--1111-443')
  assert.equal(csvNameFor('[2606:4700::1111]', 443), 'tls--2606-4700--1111--443')
})

test('reportText writes one line per version for a ticket', () => {
  const got = reportText([
    attempt({ version: 'SSL 3.0', testable: false }),
    attempt({ version: 'TLS 1.0' }),
    attempt({
      version: 'TLS 1.2',
      accepted: true,
      cipher: 'TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256',
    }),
  ])
  assert.equal(
    got,
    'SSL 3.0  Not testable\n' +
      'TLS 1.0  Refused\n' +
      'TLS 1.2  Accepted      TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256',
  )
})
