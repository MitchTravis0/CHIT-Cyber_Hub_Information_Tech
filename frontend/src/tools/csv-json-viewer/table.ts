// The shared shape both parsers produce, the format detection and the caps. All
// of it is pure so the tests can run it without a DOM.
import { detectDelimiter, parseCsv, type Delimiter } from './csv'
import { parseJson } from './json'

/** One parsed data set. `headers` and every row in `rows` are the same length:
 *  the table code indexes cells by column position and relies on it. */
export interface Table {
  headers: string[]
  rows: string[][]
  /** Plain sentences worth showing the user. Empty when nothing needed saying. */
  notes: string[]
}

export type ParseResult = { ok: true; table: Table } | { ok: false; error: string }

export type Format = 'csv' | 'json'
export type FormatChoice = 'auto' | Format
export type DelimiterChoice = 'auto' | Delimiter

/** Everything the page needs from one pass: what was read, how, and the result. */
export interface Reading {
  format: Format
  delimiter: Delimiter
  result: ParseResult
}

/** The whole file is held as text, as a row array and as the table's own index,
 *  so the input is capped rather than letting a huge export freeze the window. */
export const MAX_FILE_BYTES = 5 * 1024 * 1024

/** ResultsTable renders every row and every cell into the DOM, so the table gets
 *  a slice. The converted output and the download always hold everything. */
export const MAX_TABLE_ROWS = 2000
export const MAX_COLUMNS = 100

/** Decides which parser to use when the format Select is on "Detect". */
export function detectFormat(text: string): Format {
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (c === ' ' || c === '\t' || c === '\r' || c === '\n') continue
    return c === '[' || c === '{' ? 'json' : 'csv'
  }
  return 'csv'
}

/** ResultsTable needs a unique key per column, and header text is not unique. */
export function columnKey(index: number): string {
  return `c${index}`
}

/** True when two columns share a name, which a JSON record cannot hold. */
export function hasDuplicateHeaders(headers: string[]): boolean {
  return new Set(headers).size !== headers.length
}

/** The chosen file's name without its extension, made safe for a download, or
 *  a fixed name when the data was pasted. */
export function csvNameFor(fileName: string): string {
  if (fileName === '') return 'converted-table'
  const base = fileName.replace(/\.[^.]+$/, '')
  const safe = base.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return safe === '' ? 'converted-table' : safe
}

/** Reads the text with the chosen settings, filling in whatever is on "Detect". */
export function parseInput(
  text: string,
  formatChoice: FormatChoice,
  delimiterChoice: DelimiterChoice,
  header: boolean,
): Reading {
  const format = formatChoice === 'auto' ? detectFormat(text) : formatChoice
  const delimiter = delimiterChoice === 'auto' ? detectDelimiter(text) : delimiterChoice
  const result = format === 'json' ? parseJson(text) : parseCsv(text, delimiter, header)
  return { format, delimiter, result }
}
