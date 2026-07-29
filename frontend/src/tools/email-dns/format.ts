/** The number of DNS lookups an SPF record is allowed, matching maildns.SPFLookupLimit. */
export const SPF_LOOKUP_LIMIT = 10

/** Where a record is close enough to the limit to be worth warning about. */
export const SPF_LOOKUP_WARN_AT = 8

/** The word shown in the Verdict column of the findings table. */
export function verdictLabel(level: string): string {
  switch (level) {
    case 'ok':
      return 'Good'
    case 'warn':
      return 'Watch'
    default:
      return 'Problem'
  }
}

/** The StatusDot tone for a finding. */
export function verdictTone(level: string): 'ok' | 'warn' | 'danger' {
  if (level === 'ok') return 'ok'
  if (level === 'warn') return 'warn'
  return 'danger'
}

/** Names the selectors that were probed, so the page never implies it checked them all. */
export function selectorSentence(selectors: string[]): string {
  return `Selectors checked: ${selectors.join(', ')} (${selectors.length} in all).`
}

/** The colour of the SPF lookup count line. */
export function lookupTone(lookups: number): string {
  if (lookups > SPF_LOOKUP_LIMIT) return 'text-danger'
  if (lookups >= SPF_LOOKUP_WARN_AT) return 'text-warn'
  return 'text-fg-muted'
}
