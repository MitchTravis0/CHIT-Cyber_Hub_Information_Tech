/** Joins class names, skipping falsy values. */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ')
}

/** 1500 -> "1.5 s", 65000 -> "1 m 5 s" */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes} m ${Math.round(seconds - minutes * 60)} s`
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes)) return '-'
  let value = Math.abs(bytes)
  let unit = 0
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024
    unit++
  }
  const rounded = unit === 0 ? value : Number(value.toFixed(value < 10 ? 1 : 0))
  return `${bytes < 0 ? '-' : ''}${rounded} ${BYTE_UNITS[unit]}`
}

/**
 * Turns whatever a rejected Wails call threw into a line worth showing a tech.
 * Go bound methods reject with the plain error string.
 */
export function errorMessage(err: unknown): string {
  if (typeof err === 'string' && err.trim() !== '') return err
  if (err instanceof Error && err.message !== '') return err.message
  return 'Something went wrong and the operation stopped. Try again, and check the details if it keeps happening.'
}
