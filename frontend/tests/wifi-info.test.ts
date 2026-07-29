import test from 'node:test'
import assert from 'node:assert/strict'
import {
  channelText,
  rateText,
  signalText,
  signalTone,
  unsupportedLabel,
  widthText,
} from '../src/tools/wifi-info/link.ts'
import type { Link } from '../src/tools/wifi-info/api.ts'

function link(over: Partial<Link> = {}): Link {
  return {
    interface: 'wlan0',
    connected: true,
    ssid: 'EV',
    bssid: '38:ff:36:90:a8:a8',
    band: '2.4 GHz',
    channel: 6,
    frequencyMhz: 2437,
    widthMhz: 20,
    signalDbm: -38,
    signalPercent: 100,
    rxMbps: 144.4,
    txMbps: 144.4,
    security: '',
    reading: 'Excellent. This is right next to the access point.',
    source: 'iw',
    ...over,
  }
}

test('signalText prefers dBm and falls back to the Windows percentage', () => {
  assert.equal(signalText(link()), '-38 dBm')
  assert.equal(signalText(link({ signalDbm: 0, signalPercent: 82 })), '82%')
  assert.equal(signalText(link({ signalDbm: 0, signalPercent: 0 })), '')
})

test('signalTone matches the percentage form of the backend ladder', () => {
  assert.equal(signalTone(100), 'ok')
  assert.equal(signalTone(80), 'ok')
  assert.equal(signalTone(79), 'warn')
  assert.equal(signalTone(66), 'warn')
  assert.equal(signalTone(65), 'danger')
  assert.equal(signalTone(50), 'danger')
  assert.equal(signalTone(0), 'danger')
})

test('rateText drops trailing zeroes and renders nothing for an unknown rate', () => {
  assert.equal(rateText(144.4), '144.4 Mbps')
  assert.equal(rateText(144), '144 Mbps')
  assert.equal(rateText(0), '')
  assert.equal(rateText(-1), '')
  assert.equal(rateText(Number.NaN), '')
})

test('widthText renders nothing when the OS did not report a width', () => {
  assert.equal(widthText(20), '20 MHz')
  assert.equal(widthText(160), '160 MHz')
  assert.equal(widthText(0), '')
  assert.equal(widthText(-1), '')
})

test('channelText shows the frequency beside the channel when there is one', () => {
  assert.equal(channelText(link()), '6 (2437 MHz)')
  assert.equal(channelText(link({ frequencyMhz: 0 })), '6')
  assert.equal(channelText(link({ channel: 0 })), '')
})

test('unsupportedLabel names the operating system and returns null when the field is fine', () => {
  assert.equal(unsupportedLabel('linux', 'security', ['security']), 'not reported on Linux')
  assert.equal(unsupportedLabel('windows', 'width', ['signalDbm', 'width']), 'not reported on Windows')
  assert.equal(unsupportedLabel('darwin', 'ssid', ['ssid']), 'not reported on macOS')
  assert.equal(
    unsupportedLabel('plan9', 'security', ['security']),
    'not reported on this operating system',
  )
  assert.equal(unsupportedLabel('linux', 'width', ['security']), null)
  assert.equal(unsupportedLabel('linux', 'security', []), null)
  assert.equal(unsupportedLabel('linux', 'security', null), null)
})
