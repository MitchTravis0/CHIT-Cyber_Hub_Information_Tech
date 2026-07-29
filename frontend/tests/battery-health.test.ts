import test from 'node:test'
import assert from 'node:assert/strict'
import {
  healthText,
  healthTone,
  stateLabel,
  unsupportedLabel,
  whText,
} from '../src/tools/battery-health/battery.ts'

test('healthTone changes at 80 and 40, and greys out an unknown figure', () => {
  assert.equal(healthTone(102), 'ok')
  assert.equal(healthTone(80), 'ok')
  assert.equal(healthTone(79), 'warn')
  assert.equal(healthTone(40), 'warn')
  assert.equal(healthTone(39), 'danger')
  assert.equal(healthTone(1), 'danger')
  assert.equal(healthTone(0), 'muted')
  assert.equal(healthTone(-1), 'muted')
})

test('stateLabel names all four and never leaves a blank', () => {
  assert.equal(stateLabel('charging'), 'Charging')
  assert.equal(stateLabel('discharging'), 'On battery')
  assert.equal(stateLabel('full'), 'Fully charged')
  assert.equal(stateLabel('unknown'), 'State not reported')
  assert.equal(stateLabel(''), 'State not reported')
  assert.equal(stateLabel('something new'), 'State not reported')
})

test('whText drops trailing zeroes and renders nothing for an unknown capacity', () => {
  assert.equal(whText(60.6042), '60.6 Wh')
  assert.equal(whText(60), '60 Wh')
  assert.equal(whText(61.889), '61.9 Wh')
  assert.equal(whText(0), '')
  assert.equal(whText(-1), '')
  assert.equal(whText(Number.NaN), '')
})

test('healthText renders the real figure, above 100 included', () => {
  assert.equal(healthText(102), '102%')
  assert.equal(healthText(61), '61%')
  assert.equal(healthText(0), 'Unknown')
  assert.equal(healthText(-1), 'Unknown')
})

test('unsupportedLabel names the operating system and returns null when the field is fine', () => {
  assert.equal(unsupportedLabel('windows', 'health', ['health']), 'not reported on Windows')
  assert.equal(unsupportedLabel('darwin', 'cycles', ['cycles']), 'not reported on macOS')
  assert.equal(unsupportedLabel('linux', 'cycles', ['cycles']), 'not reported on Linux')
  assert.equal(unsupportedLabel('plan9', 'health', ['health']), 'not reported on this operating system')
  assert.equal(unsupportedLabel('windows', 'health', ['cycles']), null)
  assert.equal(unsupportedLabel('windows', 'health', []), null)
  assert.equal(unsupportedLabel('windows', 'health', null), null)
})
