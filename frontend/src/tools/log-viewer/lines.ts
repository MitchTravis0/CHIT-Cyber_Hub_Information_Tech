import type { Chunk, Line } from './api'

/** The three-letter tag shown in the gutter, so colour is never the only signal. */
export function levelTag(level: string): string {
  switch (level) {
    case 'error':
      return 'ERR'
    case 'warn':
      return 'WRN'
    case 'info':
      return 'INF'
    default:
      return ''
  }
}

/** The theme token class for one line's text. */
export function levelClass(level: string): string {
  switch (level) {
    case 'error':
      return 'text-danger'
    case 'warn':
      return 'text-warn'
    case 'info':
      return 'text-fg'
    default:
      return 'text-fg-muted'
  }
}

/**
 * A line read from the tail of a file has no line number, because counting one
 * would mean reading the whole file. The byte offset is shown instead, marked
 * so nobody mistakes it for a line number.
 */
export function gutterLabel(line: Line): string {
  return line.number > 0 ? String(line.number) : '@' + line.offset
}

/**
 * Splits a line into the text before a match, the match itself, and the text
 * after it, so the page can wrap the middle in a mark element. col is a byte
 * index from Go, and the source lines are held as JavaScript strings, so this
 * is only exact for text where the two agree; a multi-byte character before the
 * hit is handled by slicing on the same units Go counted (see the test).
 */
export function splitMatch(text: string, col: number, len: number): [string, string, string] {
  const bytes = new TextEncoder().encode(text)
  const decoder = new TextDecoder()
  const safeCol = Math.max(0, Math.min(col, bytes.length))
  const safeEnd = Math.max(safeCol, Math.min(safeCol + len, bytes.length))
  return [
    decoder.decode(bytes.slice(0, safeCol)),
    decoder.decode(bytes.slice(safeCol, safeEnd)),
    decoder.decode(bytes.slice(safeEnd)),
  ]
}

/** What Copy visible lines puts on the clipboard: the text, and nothing else. */
export function visibleText(lines: Line[]): string {
  return lines.map((line) => line.text).join('\n')
}

/** The sentence under the log pane saying which part of the file is on screen. */
export function windowLabel(chunk: Chunk): string {
  const lines = chunk.lines ?? []
  if (lines.length === 0) return 'No lines to show.'
  const count = lines.length === 1 ? '1 line' : `${lines.length.toLocaleString()} lines`
  return (
    `Showing ${count}, bytes ${chunk.start.toLocaleString()} to ` +
    `${chunk.end.toLocaleString()} of ${chunk.bytes.toLocaleString()}`
  )
}

/** The facts line under the file name. */
export function fileFacts(name: string, bytes: number, modified: string, crlf: boolean): string {
  const at = new Date(modified)
  const when = Number.isNaN(at.getTime()) ? '' : `, changed ${at.toLocaleString()}`
  const endings = crlf ? ', Windows line endings' : ''
  return `${name}, ${formatSize(bytes)}${when}${endings}`
}

// A local copy of the byte formatting, kept here rather than importing
// formatBytes, because this string is asserted whole in a test and must not
// move when the shared helper's rounding changes.
function formatSize(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  const rounded = unit === 0 ? value : Number(value.toFixed(1))
  return `${rounded} ${units[unit]}`
}
