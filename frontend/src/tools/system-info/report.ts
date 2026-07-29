import { formatBytes } from '../../lib/format'
import type { Disk, Report } from './api'

/** The exact words used when an operating system will not report a field. */
export const NOT_AVAILABLE = 'not available on this OS'

/** The exact words used when a field is normally available and this time was not. */
export const NOT_REPORTED = 'not reported'

/**
 * The one place a field turns into text. The two empty cases are genuinely
 * different: "not available on this OS" means Linux will not give a serial
 * number without root, while "not reported" means the source that normally
 * answers did not. Neither ever renders as a blank, which would read as a bug.
 */
export function fieldText(value: string, field: string, unsupported: string[]): string {
  if (value !== '') return value
  return unsupported.includes(field) ? NOT_AVAILABLE : NOT_REPORTED
}

/**
 * The two largest non-zero units, largest first. Seconds never appear above one
 * minute: nobody cares that a server has been up for 12 days, 3 hours and 7
 * seconds.
 */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 60) return 'less than a minute'

  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  const unit = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`

  if (days > 0) {
    return hours > 0 ? `${unit(days, 'day')} ${unit(hours, 'hour')}` : unit(days, 'day')
  }
  if (hours > 0) {
    return minutes > 0 ? `${unit(hours, 'hour')} ${unit(minutes, 'minute')}` : unit(hours, 'hour')
  }
  return unit(minutes, 'minute')
}

function pad(text: string, width: number): string {
  return text.length >= width ? text.slice(0, width) : text + ' '.repeat(width - text.length)
}

/** One drive's line in the copied report. */
export function diskLine(d: Disk): string {
  return (
    pad(d.mount, 12) +
    ' ' +
    pad(d.fs, 8) +
    ' ' +
    `${formatBytes(d.total)} total, ${formatBytes(d.used)} used, ${formatBytes(d.free)} free, ${d.usedPct}% full`
  )
}

/** What the Copy report button puts on the clipboard. */
export function reportText(r: Report): string {
  const unsupported = r.unsupported ?? []
  const disks = r.disks ?? []
  const f = (value: string, field: string) => fieldText(value, field, unsupported)

  const lines = [
    'CHIT system snapshot',
    '',
    `Computer name: ${f(r.hostname, 'hostname')}`,
    `Signed in as: ${f(r.user, 'user')}`,
    `Operating system: ${f(r.osName, 'osName')}`,
    `Version: ${f(r.osVersion, 'osVersion')}`,
    `Architecture: ${f(r.arch, 'arch')}`,
    `Up for: ${unsupported.includes('uptime') ? NOT_AVAILABLE : formatUptime(r.uptimeS)}`,
    `Started: ${f(r.bootTime, 'bootTime')}`,
    '',
    `Manufacturer: ${f(r.manufacturer, 'manufacturer')}`,
    `Model: ${f(r.model, 'model')}`,
    `Serial number: ${f(r.serial, 'serial')}`,
    `Processor: ${f(r.cpuModel, 'cpuModel')}`,
    `Processor cores: ${r.cpuCores}`,
    `Memory fitted: ${r.memoryTotal > 0 ? formatBytes(r.memoryTotal) : NOT_REPORTED}`,
    `Memory free: ${unsupported.includes('memoryFree') ? NOT_AVAILABLE : formatBytes(r.memoryFree)}`,
    '',
    'Drives',
  ]

  if (disks.length === 0) {
    lines.push('none reported')
  } else {
    for (const d of disks) lines.push(diskLine(d))
  }

  lines.push('', `Read with CHIT ${r.appVersion} on ${r.os}/${r.arch}`)
  return lines.join('\n')
}
