import test from 'node:test'
import assert from 'node:assert/strict'
import {
  detailsCell,
  friendlyService,
  mergeDevices,
} from '../src/tools/device-discovery/services.ts'
import type { Device } from '../src/tools/device-discovery/api.ts'

function device(over: Partial<Device>): Device {
  return {
    key: 'mDNS|10.0.0.1|_ipp._tcp|Printer',
    protocol: 'mDNS',
    ip: '10.0.0.1',
    name: 'Printer',
    service: '_ipp._tcp',
    host: '',
    port: 0,
    details: '',
    adapter: 'eth0',
    ...over,
  }
}

test('friendlyService names every type the tool asks about', () => {
  const pairs: [string, string][] = [
    ['_ipp._tcp', 'Printer (IPP)'],
    ['_ipps._tcp', 'Printer (IPP)'],
    ['_printer._tcp', 'Printer (raw)'],
    ['_pdl-datastream._tcp', 'Printer (raw)'],
    ['_scanner._tcp', 'Scanner'],
    ['_uscan._tcp', 'Scanner'],
    ['_smb._tcp', 'Windows file share'],
    ['_afpovertcp._tcp', 'Mac file share'],
    ['_http._tcp', 'Web page'],
    ['_https._tcp', 'Web page (secure)'],
    ['_ssh._tcp', 'SSH'],
    ['_sftp-ssh._tcp', 'SFTP'],
    ['_rfb._tcp', 'Screen sharing (VNC)'],
    ['_workstation._tcp', 'Computer'],
    ['_device-info._tcp', 'Device information'],
    ['_googlecast._tcp', 'Chromecast'],
    ['_airplay._tcp', 'AirPlay'],
    ['_raop._tcp', 'AirPlay'],
    ['_spotify-connect._tcp', 'Spotify speaker'],
    ['_hap._tcp', 'HomeKit accessory'],
    ['_daap._tcp', 'iTunes share'],
    ['_companion-link._tcp', 'Apple device'],
    ['upnp:rootdevice', 'UPnP device'],
  ]
  for (const [service, phrase] of pairs) {
    assert.equal(friendlyService(service), phrase, service)
  }
})

test('friendlyService leaves an unknown type alone', () => {
  // Inventing a description would be worse than showing what the device said.
  assert.equal(friendlyService('_weird._tcp'), '_weird._tcp')
  assert.equal(friendlyService('urn:schemas-upnp-org:device:MediaServer:1'), 'urn:schemas-upnp-org:device:MediaServer:1')
  assert.equal(friendlyService(''), '')
})

test('detailsCell keeps the raw service type visible', () => {
  // The Service column shows a phrase, so the raw type is appended here rather
  // than lost from the screen and the CSV.
  assert.equal(
    detailsCell(device({ service: '_ipp._tcp', details: 'Brother HL-L2350DW' })),
    'Brother HL-L2350DW (_ipp._tcp)',
  )
  assert.equal(detailsCell(device({ service: '_ipp._tcp', details: '' })), '(_ipp._tcp)')
  // An unrecognised type is already shown in full in the Service column.
  assert.equal(
    detailsCell(device({ service: '_weird._tcp', details: 'Some Server/1.0' })),
    'Some Server/1.0',
  )
  assert.equal(detailsCell(device({ service: '', details: 'Some Server/1.0' })), 'Some Server/1.0')
})

test('mergeDevices folds repeated emissions, last write wins', () => {
  const first = device({ key: 'a', port: 0, details: '' })
  const better = device({ key: 'a', port: 631, details: 'Brother' })
  const other = device({ key: 'b', name: 'TV' })

  const got = mergeDevices([first, other, better])
  assert.equal(got.length, 2)
  const merged = got.find((d) => d.key === 'a')
  assert.equal(merged?.port, 631)
  assert.equal(merged?.details, 'Brother')
})

test('mergeDevices keeps the order a device was first heard in', () => {
  const got = mergeDevices([
    device({ key: 'a' }),
    device({ key: 'b' }),
    device({ key: 'a', name: 'again' }),
    device({ key: 'c' }),
  ])
  assert.deepEqual(
    got.map((d) => d.key),
    ['a', 'b', 'c'],
  )
})

test('mergeDevices handles an empty list', () => {
  assert.deepEqual(mergeDevices([]), [])
})
