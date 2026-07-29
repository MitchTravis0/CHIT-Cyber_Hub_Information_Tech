/** Turns a host into something safe to use in a downloaded file name. */
export function hostSlug(host: string): string {
  const slug = host
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
    .replace(/-+$/, '')
  return slug === '' ? 'link' : slug
}

/** Maps a finding severity to the tone words the page styles from. */
export function severityTone(severity: string): 'danger' | 'warn' | 'info' {
  if (severity === 'danger') return 'danger'
  if (severity === 'warn') return 'warn'
  return 'info'
}

export type LevelTone = 'ok' | 'warn' | 'danger' | 'unknown'

/**
 * Maps the report level to the banner tone. "unknown" means no step of the chain
 * answered, so it must not be red: nothing was found wrong, nothing was checked
 * either. Anything the backend has not taught this page about is treated as
 * unknown rather than as danger, so a new level can never render as an alarm.
 */
export function levelTone(level: string): LevelTone {
  if (level === 'ok' || level === 'warn' || level === 'danger') return level
  return 'unknown'
}
