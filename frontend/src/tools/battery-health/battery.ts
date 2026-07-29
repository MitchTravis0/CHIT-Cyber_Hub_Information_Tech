/** The tint on the health figure. Never the only signal: the verdict sits beside it. */
export function healthTone(percent: number): 'ok' | 'warn' | 'danger' | 'muted' {
  if (percent <= 0) return 'muted'
  if (percent >= 80) return 'ok'
  if (percent >= 40) return 'warn'
  return 'danger'
}

/** The state, in words a user would say. */
export function stateLabel(state: string): string {
  switch (state) {
    case 'charging':
      return 'Charging'
    case 'discharging':
      return 'On battery'
    case 'full':
      return 'Fully charged'
  }
  return 'State not reported'
}

/** A capacity, or nothing at all when this OS did not report one. */
export function whText(wh: number): string {
  if (!Number.isFinite(wh) || wh <= 0) return ''
  return `${Number(wh.toFixed(1))} Wh`
}

/** The headline figure. */
export function healthText(percent: number): string {
  if (!Number.isFinite(percent) || percent <= 0) return 'Unknown'
  return `${percent}%`
}

const OS_NAMES: Record<string, string> = {
  windows: 'Windows',
  darwin: 'macOS',
  linux: 'Linux',
}

/**
 * The label for a field this operating system would not report, or null when it
 * did. A blank cell reads as a bug; "not reported on Windows" reads as the truth.
 */
export function unsupportedLabel(
  os: string,
  field: string,
  unsupported: string[] | null,
): string | null {
  if (!(unsupported ?? []).includes(field)) return null
  return `not reported on ${OS_NAMES[os] ?? 'this operating system'}`
}
