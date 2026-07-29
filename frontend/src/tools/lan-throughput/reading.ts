import { formatBytes, formatDuration } from '../../lib/format'
import type { Pull } from './api'

/**
 * The plain sentence beside a speed. The Go side is authoritative and puts its
 * answer in Pull.Reading, which the table renders; this copy exists only for the
 * summary line's "best" figure, which has no row of its own. The two are pinned
 * together by testdata/lanspeed-readings.json, which both suites read.
 */
export function speedReading(mbps: number): string {
  if (mbps >= 700) return 'Gigabit or better.'
  if (mbps >= 200) return 'Fast, but short of a full gigabit link.'
  if (mbps >= 70) return 'About 100 Mbps. Check for a 100 Mbps switch port or a damaged cable.'
  if (mbps >= 20) return 'Wi-Fi speeds, or a busy link.'
  if (mbps > 0)
    return 'Very slow for a local network. Check the cable, the port and what else is using the link.'
  return 'Too little was transferred to measure.'
}

/** The line under the table once the test has been stopped. */
export function summaryLine(
  pulls: number,
  bestMbps: number,
  bytesOut: number,
  ms: number,
): string {
  if (pulls === 0) return `No pulls in ${formatDuration(ms)}`
  const count = pulls === 1 ? '1 pull' : `${pulls} pulls`
  return `${count}, best ${mbpsText(bestMbps)}, ${formatBytes(bytesOut)} sent in ${formatDuration(ms)}`
}

/** One speed, to one decimal place. */
export function mbpsText(mbps: number): string {
  if (!Number.isFinite(mbps) || mbps <= 0) return '0 Mbps'
  return `${mbps.toFixed(1)} Mbps`
}

/** How long one pull took, to one decimal place. */
export function secondsText(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0 s'
  return `${seconds.toFixed(1)} s`
}

/** The command to run on the other machine when it has no browser open. */
export function curlFor(url: string): string {
  return `curl -o /dev/null ${url}/dl`
}

/**
 * The link rebuilt for the address the tech picked. The backend builds it from
 * the address it decided to offer first, and only the host part changes.
 *
 * It is a named function rather than an expression inside the page because two
 * things read it, the link on screen and the QR code's payload, and a mismatch
 * would hand the other machine a code that scans cleanly and points at the
 * wrong adapter. An IPv6 address is bracketed; anything that does not look like
 * a host and port is left exactly as it was.
 */
export function linkForAddress(url: string, ip: string): string {
  if (url === '' || ip === '') return url
  const slashes = url.indexOf('//')
  if (slashes < 0) return url

  const from = slashes + 2
  const end = url.indexOf('/', from)
  const authority = end < 0 ? url.slice(from) : url.slice(from, end)

  const colon = authority.lastIndexOf(':')
  // A colon inside brackets is part of an IPv6 literal, not a port separator.
  if (colon < 0 || colon < authority.lastIndexOf(']')) return url

  const host = ip.includes(':') ? `[${ip}]` : ip
  return url.slice(0, from) + host + authority.slice(colon) + (end < 0 ? '' : url.slice(end))
}

/** What the Result column says. */
export function statusLabel(status: string): string {
  return status === 'ok' ? 'Complete' : 'Stopped part way'
}

/** The RFC3339 stamp the backend sets, as a local wall-clock time. */
export function localTime(rfc3339: string): string {
  const at = new Date(rfc3339)
  if (Number.isNaN(at.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(at.getHours())}:${pad(at.getMinutes())}:${pad(at.getSeconds())}`
}

/** A stable key for one pull. */
export function pullId(pull: Pull): string {
  return `${pull.time}|${pull.peer}|${pull.bytes}`
}

/** Field-level validation, so a bad port never reaches the backend. */
export function validPort(text: string): { ok: true; port: number } | { ok: false; error: string } {
  const trimmed = text.trim()
  if (trimmed === '') return { ok: true, port: 0 }
  if (!/^\d+$/.test(trimmed)) {
    return { ok: false, error: 'That is not a port number. Ports run from 1024 to 65535 here.' }
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
