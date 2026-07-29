import type { Timing } from './api'

export interface Segment {
  key: string
  label: string
  ms: number
  /** Share of the time to the first byte, 0 to 100. */
  percent: number
}

/**
 * Splits the time to the first byte into the four stages the bar draws. The
 * server stage is whatever is left after the lookup, the connection and the
 * handshake, which is the part the site owner controls.
 */
export function timingSegments(timing: Timing): Segment[] {
  const dns = Math.max(0, timing.dnsMs)
  const connect = Math.max(0, timing.connectMs)
  const tls = Math.max(0, timing.tlsMs)
  const server = Math.max(0, timing.ttfbMs - dns - connect - tls)
  const total = dns + connect + tls + server
  const stages = [
    { key: 'dns', label: 'DNS', ms: dns },
    { key: 'connect', label: 'Connect', ms: connect },
    { key: 'tls', label: 'TLS', ms: tls },
    { key: 'server', label: 'Server', ms: server },
  ]
  return stages.map((stage) => ({
    ...stage,
    percent: total === 0 ? 0 : (stage.ms / total) * 100,
  }))
}

/** True when the connection was reused, so there is no handshake to break down. */
export function reusedConnection(timing: Timing): boolean {
  return timing.dnsMs === 0 && timing.connectMs === 0 && timing.tlsMs === 0
}
