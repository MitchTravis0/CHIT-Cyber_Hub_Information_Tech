import type { Hit } from './api'

export interface Command {
  /** What kind of machine this line is for. */
  label: string
  command: string
}

/** An IPv6 literal has to be bracketed before it goes into a command line. */
function hostFor(ip: string): string {
  return ip.includes(':') ? `[${ip}]` : ip
}

/**
 * The lines to read out to whoever is at the other machine. Only the protocols
 * actually being listened on get a line, so nobody is told to run a UDP test
 * against a TCP-only listener.
 */
export function commandsFor(ip: string, port: number, protocol: string): Command[] {
  const host = hostFor(ip)
  const out: Command[] = []
  if (protocol === 'tcp' || protocol === 'both') {
    out.push({ label: 'Windows', command: `Test-NetConnection ${host} -Port ${port}` })
    out.push({ label: 'macOS or Linux', command: `nc -vz ${host} ${port}` })
  }
  if (protocol === 'udp' || protocol === 'both') {
    out.push({
      label: 'macOS or Linux (UDP)',
      command: `printf hello | nc -u -w1 ${host} ${port}`,
    })
  }
  return out
}

/** The live count under the progress bar. */
export function arrivalLine(hits: Hit[]): string {
  if (hits.length === 0) return 'No arrivals yet'
  const machines = new Set(hits.map((hit) => hit.peer)).size
  const arrivals = hits.length === 1 ? '1 arrival' : `${hits.length} arrivals`
  const from = machines === 1 ? '1 machine' : `${machines} machines`
  return `${arrivals} from ${from}`
}

/** The RFC3339 stamp the backend sets, as a local wall-clock time. */
export function localTime(rfc3339: string): string {
  const at = new Date(rfc3339)
  if (Number.isNaN(at.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(at.getHours())}:${pad(at.getMinutes())}:${pad(at.getSeconds())}`
}

/** A stable key for one arrival. */
export function hitId(hit: Hit): string {
  return `${hit.time}|${hit.protocol}|${hit.peer}|${hit.peerPort}|${hit.bytes}`
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
