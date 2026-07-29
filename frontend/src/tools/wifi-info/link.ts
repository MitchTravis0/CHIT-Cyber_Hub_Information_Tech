import type { Link } from './api'

/** The big number on the card. Windows reports a percentage and never dBm. */
export function signalText(link: Link): string {
  if (link.signalDbm !== 0) return `${link.signalDbm} dBm`
  if (link.signalPercent > 0) return `${link.signalPercent}%`
  return ''
}

/**
 * The tint on the signal figure. Never the only signal: the plain reading sits
 * beside it. The thresholds are the percentage form of the dBm ladder the
 * backend uses, so the colour and the sentence cannot disagree.
 */
export function signalTone(percent: number): 'ok' | 'warn' | 'danger' {
  if (percent >= 80) return 'ok'
  if (percent >= 66) return 'warn'
  return 'danger'
}

/** A negotiated rate, or nothing at all when this OS did not report one. */
export function rateText(mbps: number): string {
  if (!Number.isFinite(mbps) || mbps <= 0) return ''
  return `${Number(mbps.toFixed(1))} Mbps`
}

/** A channel width, or nothing at all. */
export function widthText(mhz: number): string {
  if (!Number.isFinite(mhz) || mhz <= 0) return ''
  return `${mhz} MHz`
}

/** The channel, with the frequency beside it when there is one. */
export function channelText(link: Link): string {
  if (link.channel <= 0) return ''
  if (link.frequencyMhz > 0) return `${link.channel} (${link.frequencyMhz} MHz)`
  return String(link.channel)
}

const OS_NAMES: Record<string, string> = {
  windows: 'Windows',
  darwin: 'macOS',
  linux: 'Linux',
}

/**
 * The label for a field this operating system would not report, or null when it
 * did. A blank cell reads as a bug; "not reported on Linux" reads as the truth.
 */
export function unsupportedLabel(
  os: string,
  field: string,
  unsupported: string[] | null,
): string | null {
  if (!(unsupported ?? []).includes(field)) return null
  return `not reported on ${OS_NAMES[os] ?? 'this operating system'}`
}
