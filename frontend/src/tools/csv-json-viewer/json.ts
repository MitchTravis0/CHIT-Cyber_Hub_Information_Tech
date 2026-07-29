// JSON read into the table shape, and the table written back out as JSON. Pure
// functions only, no React and no DOM.
import type { ParseResult, Table } from './table'

const INVALID =
  'That is not valid JSON. Check for a missing comma, a bracket that was never closed, or a comma just before a closing bracket or brace.'

const NOT_A_LIST =
  'That JSON is a single value, not a list of records, so there is nothing to put in a table. This tool expects something like [{"name":"PC01"},{"name":"PC02"}].'

const NESTED_NOTE =
  'Some values were lists or nested records. They are shown as the JSON text they came from, and converting back to JSON will make them plain text rather than lists again.'

const NULL_NOTE =
  'Some values were empty (JSON null). They show as blank cells, and converting back to JSON will write them as empty text rather than null.'

interface Flags {
  nested: boolean
  nulls: boolean
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function cellText(value: unknown, flags: Flags): string {
  if (value === null) {
    flags.nulls = true
    return ''
  }
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  flags.nested = true
  return JSON.stringify(value)
}

// Columns are the union of the keys in first-seen order. Note that JavaScript
// lists keys that look like non-negative integers first, in numeric order, ahead
// of every other key regardless of where they appeared in the text, so
// {"2":"x","name":"y"} gives the columns 2 then name. That is how JSON.parse
// builds the object and this tool cannot change it.
function objectRows(records: Record<string, unknown>[], flags: Flags): Table {
  const headers: string[] = []
  for (const record of records) {
    for (const key of Object.keys(record)) {
      if (!headers.includes(key)) headers.push(key)
    }
  }
  if (headers.length === 0) return { headers: [], rows: [], notes: [] }
  const rows = records.map((record) =>
    headers.map((key) => (key in record ? cellText(record[key], flags) : '')),
  )
  return { headers, rows, notes: [] }
}

function scalarRows(values: unknown[], flags: Flags): Table {
  return { headers: ['value'], rows: values.map((value) => [cellText(value, flags)]), notes: [] }
}

function withNotes(table: Table, flags: Flags): ParseResult {
  const notes: string[] = []
  if (flags.nested) notes.push(NESTED_NOTE)
  if (flags.nulls) notes.push(NULL_NOTE)
  return { ok: true, table: { ...table, notes } }
}

export function parseJson(text: string): ParseResult {
  if (text.trim() === '') return { ok: true, table: { headers: [], rows: [], notes: [] } }

  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return { ok: false, error: INVALID }
  }

  const flags: Flags = { nested: false, nulls: false }
  if (Array.isArray(value)) {
    if (value.length === 0) return { ok: true, table: { headers: [], rows: [], notes: [] } }
    // Object mode only when every element is a record: one stray scalar and the
    // whole array is read as a single "value" column instead.
    if (value.every(isRecord)) return withNotes(objectRows(value, flags), flags)
    return withNotes(scalarRows(value, flags), flags)
  }
  if (isRecord(value)) return withNotes(objectRows([value], flags), flags)
  return { ok: false, error: NOT_A_LIST }
}

/** A JSON array of objects, two space indent, keyed by the column headers. Every
 *  value is a string: a CSV records no types, so 42 comes out as "42". */
export function formatJson(table: Table): string {
  const objects = table.rows.map((row) => {
    const record: Record<string, string> = {}
    // A duplicate header name means the right-hand column wins, because an
    // object cannot hold the same key twice.
    table.headers.forEach((header, i) => {
      record[header] = row[i]
    })
    return record
  })
  return JSON.stringify(objects, null, 2)
}
