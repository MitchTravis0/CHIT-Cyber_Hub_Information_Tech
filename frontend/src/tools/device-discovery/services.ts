import type { Device } from './api'

/**
 * Plain phrases for the service types a field tech meets. Anything else is
 * returned unchanged: an unrecognised type is still useful, and inventing a
 * description for it would be worse than showing what the device said.
 */
const PHRASES: Record<string, string> = {
  '_ipp._tcp': 'Printer (IPP)',
  '_ipps._tcp': 'Printer (IPP)',
  '_printer._tcp': 'Printer (raw)',
  '_pdl-datastream._tcp': 'Printer (raw)',
  '_scanner._tcp': 'Scanner',
  '_uscan._tcp': 'Scanner',
  '_smb._tcp': 'Windows file share',
  '_afpovertcp._tcp': 'Mac file share',
  '_http._tcp': 'Web page',
  '_https._tcp': 'Web page (secure)',
  '_ssh._tcp': 'SSH',
  '_sftp-ssh._tcp': 'SFTP',
  '_rfb._tcp': 'Screen sharing (VNC)',
  '_workstation._tcp': 'Computer',
  '_device-info._tcp': 'Device information',
  '_googlecast._tcp': 'Chromecast',
  '_airplay._tcp': 'AirPlay',
  '_raop._tcp': 'AirPlay',
  '_spotify-connect._tcp': 'Spotify speaker',
  '_hap._tcp': 'HomeKit accessory',
  '_daap._tcp': 'iTunes share',
  '_companion-link._tcp': 'Apple device',
  'upnp:rootdevice': 'UPnP device',
}

export function friendlyService(service: string): string {
  return PHRASES[service] ?? service
}

/**
 * The Details cell. When the service type was turned into a phrase, the raw type
 * is appended so nothing is hidden from the screen or the CSV export.
 */
export function detailsCell(device: Device): string {
  const friendly = friendlyService(device.service)
  if (friendly === device.service || device.service === '') return device.details
  const raw = `(${device.service})`
  return device.details === '' ? raw : `${device.details} ${raw}`
}

/**
 * Folds every emission into one row per key, last write wins. The backend
 * re-emits a device when a later reply carries more than the first one did.
 */
export function mergeDevices(devices: Device[]): Device[] {
  const map = new Map<string, Device>()
  for (const device of devices) map.set(device.key, device)
  return Array.from(map.values())
}
