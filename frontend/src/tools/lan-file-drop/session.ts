import { formatBytes, formatDuration } from '../../lib/format'
import type { Transfer } from './api'

/** One file the tech picked, before anything is shared. */
export interface PickedFile {
  path: string
  name: string
}

export interface Summary {
  downloads: number
  uploads: number
  bytesOut: number
  bytesIn: number
}

/** The address to read out or point a camera at. IPv6 has to be bracketed. */
export function shareUrl(ip: string, port: number, token: string): string {
  const host = ip.includes(':') ? `[${ip}]` : ip
  return `http://${host}:${port}/d/${token}`
}

/** The last path segment, whichever separator the operating system uses. */
export function baseName(path: string): string {
  const parts = path.split(/[\\/]/).filter((part) => part !== '')
  return parts.length === 0 ? path : parts[parts.length - 1]
}

export function toPicked(paths: string[]): PickedFile[] {
  return paths.map((path) => ({ path, name: baseName(path) }))
}

function plural(n: number, singular: string, many: string): string {
  return `${n.toLocaleString()} ${n === 1 ? singular : many}`
}

/** The line under the file list. */
export function fileListLine(files: PickedFile[], bytes: number): string {
  if (files.length === 0) return 'No files chosen yet.'
  return `${plural(files.length, 'file', 'files')}, ${formatBytes(bytes)}`
}

/** The line after Stop. */
export function summaryLine(s: Summary, ms: number): string {
  return (
    `Shared for ${formatDuration(ms)}: ` +
    `${plural(s.downloads, 'file', 'files')} sent (${formatBytes(s.bytesOut)}), ` +
    `${s.uploads.toLocaleString()} received (${formatBytes(s.bytesIn)})`
  )
}

/**
 * The port box. Below 1024 needs administrator rights, which CHIT never asks
 * for, so the floor is a real constraint rather than a preference.
 */
export function validPort(text: string): { ok: true; port: number } | { ok: false; error: string } {
  const trimmed = text.trim()
  if (trimmed === '') {
    return { ok: false, error: 'Type a port number, or leave it at 8722.' }
  }
  if (!/^\d{1,5}$/.test(trimmed)) {
    return { ok: false, error: 'A port is a number between 1024 and 65535, for example 8722.' }
  }
  const port = Number(trimmed)
  if (port < 1024 || port > 65535) {
    return {
      ok: false,
      error:
        'The port must be between 1024 and 65535. Below 1024 needs administrator rights, which CHIT never asks for.',
    }
  }
  return { ok: true, port }
}

/** A stable key for a transfer row: two identical downloads are two events. */
export function transferId(t: Transfer): string {
  return `${t.time}|${t.peer}|${t.direction}|${t.name}|${t.bytes}`
}

/** The clock time shown in the log, tolerating a value it cannot read. */
export function transferTime(t: Transfer): string {
  const at = new Date(t.time)
  return Number.isNaN(at.getTime()) ? '' : at.toLocaleTimeString()
}

export function directionLabel(direction: string): string {
  return direction === 'download' ? 'Sent' : 'Received'
}

export function statusLabel(status: string): string {
  return status === 'ok' ? 'Done' : 'Failed'
}
