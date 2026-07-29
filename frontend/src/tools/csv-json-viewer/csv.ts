// RFC 4180 CSV, parsed and written. Pure functions only, no React and no DOM.
import type { ParseResult, Table } from './table'

export type Delimiter = ',' | ';' | '\t'

/** Comma first, so a tie is broken towards the commonest separator. */
const CANDIDATES: Delimiter[] = [',', ';', '\t']

/** A pathological single-line file must not make detection slow. */
const DETECT_LIMIT = 4096

/** The text up to the first line break outside a quoted field, capped at the
 *  number of characters detection actually looks at. */
function firstLine(text: string): string {
  const end = Math.min(text.length, DETECT_LIMIT)
  let inQuotes = false
  for (let i = 0; i < end; i++) {
    const c = text[i]
    if (c === '"') {
      if (inQuotes && text[i + 1] === '"') {
        i++
        continue
      }
      inQuotes = !inQuotes
      continue
    }
    if (!inQuotes && (c === '\n' || c === '\r')) return text.slice(0, i)
  }
  return text.slice(0, end)
}

function countOutsideQuotes(line: string, candidate: string): number {
  let inQuotes = false
  let count = 0
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (c === '"') {
      if (inQuotes && line[i + 1] === '"') {
        i++
        continue
      }
      inQuotes = !inQuotes
      continue
    }
    if (!inQuotes && c === candidate) count++
  }
  return count
}

/** Picks the delimiter from the header line: the candidate that appears most
 *  often outside quotes, comma when nothing appears at all. */
export function detectDelimiter(text: string): Delimiter {
  const line = firstLine(text)
  let best: Delimiter = ','
  let bestCount = 0
  for (const candidate of CANDIDATES) {
    const count = countOutsideQuotes(line, candidate)
    if (count > bestCount) {
      best = candidate
      bestCount = count
    }
  }
  return best
}

/** RFC 4180 parse. `header` false generates the column names. */
export function parseCsv(text: string, delimiter: Delimiter, header: boolean): ParseResult {
  if (text.trim() === '') return { ok: true, table: { headers: [], rows: [], notes: [] } }

  const lines: string[][] = []
  let row: string[] = []
  let field = ''
  // Any character consumed for the current field, so a quote later in the field
  // is an ordinary character rather than the start of a quoted value.
  let touched = false
  let inQuotes = false

  const endField = () => {
    row.push(field)
    field = ''
    touched = false
  }
  const endRow = () => {
    // A line with no characters at all produces no row: real exports end with
    // blank lines and a padded empty row at the bottom would look like a bug.
    if (row.length === 0 && field === '' && !touched) return
    endField()
    lines.push(row)
    row = []
  }

  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (inQuotes) {
      if (c === '"') {
        if (text[i + 1] === '"') {
          field += '"'
          i++
        } else {
          inQuotes = false
        }
        continue
      }
      if (c === '\r') {
        // A CRLF inside a quoted field becomes one LF, so the same file gives
        // the same value on Windows and on Linux.
        if (text[i + 1] === '\n') i++
        field += '\n'
        continue
      }
      field += c
      continue
    }
    if (c === '"' && !touched) {
      inQuotes = true
      touched = true
      continue
    }
    if (c === delimiter) {
      endField()
      continue
    }
    if (c === '\r') {
      if (text[i + 1] === '\n') i++
      endRow()
      continue
    }
    if (c === '\n') {
      endRow()
      continue
    }
    field += c
    touched = true
  }
  const unclosed = inQuotes
  endRow()

  const notes: string[] = []
  if (lines.length === 0) return { ok: true, table: { headers: [], rows: [], notes } }

  let headers: string[]
  let dataRows: string[][]
  let ragged = 0
  if (header) {
    headers = lines[0].map((name, i) => (name === '' ? `Column ${i + 1}` : name))
    dataRows = lines.slice(1)
    ragged = dataRows.filter((line) => line.length !== headers.length).length
  } else {
    headers = []
    dataRows = lines
  }

  let width = headers.length
  for (const line of dataRows) {
    if (line.length > width) width = line.length
  }
  while (headers.length < width) headers.push(`Column ${headers.length + 1}`)
  const rows = dataRows.map((line) => {
    const padded = line.slice()
    while (padded.length < width) padded.push('')
    return padded
  })

  if (ragged > 0) {
    notes.push(
      `${ragged} ${ragged === 1 ? 'row' : 'rows'} did not have the same number of values as the header row. Short rows were padded with empty cells, and extra values were kept in extra columns named after their position. Nothing was thrown away.`,
    )
  }
  if (unclosed) {
    notes.push(
      'The last value in the file starts with a quote that is never closed. Everything after it was read as one value.',
    )
  }

  return { ok: true, table: { headers, rows, notes } }
}

// No formula-injection guard here, unlike ResultsTable's Export CSV button: that
// file is opened by Excel, this pane is a faithful text conversion of the input
// and must not alter the data.
function csvField(value: string): string {
  return /["\r\n,]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value
}

/** CSV text for the table. Always comma delimited, always CRLF. */
export function formatCsv(table: Table): string {
  if (table.headers.length === 0) return ''
  const lines = [table.headers.map(csvField).join(',')]
  for (const row of table.rows) lines.push(row.map(csvField).join(','))
  return `${lines.join('\r\n')}\r\n`
}
