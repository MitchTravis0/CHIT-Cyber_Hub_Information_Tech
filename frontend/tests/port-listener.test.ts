import test from 'node:test'
import assert from 'node:assert/strict'
import {
  arrivalLine,
  commandsFor,
  hitId,
  localTime,
  validPort,
} from '../src/tools/port-listener/commands.ts'
import type { Hit } from '../src/tools/port-listener/api.ts'

function hit(over: Partial<Hit> = {}): Hit {
  return {
    time: '2026-07-28T10:11:12Z',
    protocol: 'tcp',
    peer: '10.0.0.7',
    peerPort: 51234,
    bytes: 0,
    preview: '',
    ...over,
  }
}

test('commandsFor gives two TCP lines and no UDP one', () => {
  const lines = commandsFor('10.40.21.153', 8730, 'tcp')
  assert.equal(lines.length, 2)
  assert.equal(lines[0].command, 'Test-NetConnection 10.40.21.153 -Port 8730')
  assert.equal(lines[1].command, 'nc -vz 10.40.21.153 8730')
  for (const line of lines) assert.equal(line.command.includes('-u'), false)
})

test('commandsFor gives exactly one UDP line', () => {
  const lines = commandsFor('10.40.21.153', 5514, 'udp')
  assert.equal(lines.length, 1)
  assert.equal(lines[0].command, 'printf hello | nc -u -w1 10.40.21.153 5514')
})

test('commandsFor gives three lines for both', () => {
  const lines = commandsFor('10.40.21.153', 8730, 'both')
  assert.equal(lines.length, 3)
  for (const line of lines) {
    assert.equal(line.command.includes('10.40.21.153'), true)
    assert.equal(line.command.includes('8730'), true)
  }
})

test('commandsFor brackets an IPv6 literal', () => {
  const lines = commandsFor('fe80::1', 8730, 'both')
  assert.equal(lines[0].command, 'Test-NetConnection [fe80::1] -Port 8730')
  assert.equal(lines[2].command, 'printf hello | nc -u -w1 [fe80::1] 8730')
})

test('arrivalLine counts machines, not arrivals', () => {
  assert.equal(arrivalLine([]), 'No arrivals yet')
  assert.equal(arrivalLine([hit()]), '1 arrival from 1 machine')
  assert.equal(arrivalLine([hit(), hit()]), '2 arrivals from 1 machine')
  assert.equal(
    arrivalLine([hit(), hit(), hit({ peer: '10.0.0.9' }), hit({ peer: '10.0.0.9' })]),
    '4 arrivals from 2 machines',
  )
})

test('localTime renders a wall-clock time and rejects garbage', () => {
  const at = new Date(2026, 6, 28, 14, 22, 5)
  assert.equal(localTime(at.toISOString()), '14:22:05')
  assert.equal(localTime('not a time'), '')
  assert.equal(localTime(''), '')
})

test('hitId separates two arrivals from the same machine at different ports', () => {
  assert.notEqual(hitId(hit({ peerPort: 1 })), hitId(hit({ peerPort: 2 })))
  assert.equal(hitId(hit()), hitId(hit()))
})

test('validPort accepts the range and rejects outside it', () => {
  assert.deepEqual(validPort('8730'), { ok: true, port: 8730 })
  assert.deepEqual(validPort('1024'), { ok: true, port: 1024 })
  assert.deepEqual(validPort('65535'), { ok: true, port: 65535 })
  assert.deepEqual(validPort(''), { ok: true, port: 0 })
  assert.deepEqual(validPort('  8730 '), { ok: true, port: 8730 })

  const low = validPort('1023')
  assert.equal(low.ok, false)
  assert.match(low.ok === false ? low.error : '', /administrator rights/)

  const high = validPort('65536')
  assert.equal(high.ok, false)

  const text = validPort('eighty')
  assert.equal(text.ok, false)
  assert.match(text.ok === false ? text.error : '', /not a port number/)
})
