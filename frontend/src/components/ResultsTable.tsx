import type { ReactNode } from 'react'
import { useMemo, useState } from 'react'
import { ChevronDown, ChevronUp, ChevronsUpDown, Download, Search } from 'lucide-react'
import { cn } from '../lib/format'
import { Button, focusRing } from './Button'
import { fieldClass } from './TextInput'

export type CellValue = string | number | boolean | null | undefined

export type RowTone = 'ok' | 'warn' | 'danger'

export interface Column<T> {
  /** Unique per table. Doubles as the row property name when `value` is omitted. */
  key: string
  header: string
  /** Value used for sorting, filtering and CSV. Defaults to row[key]. */
  value?: (row: T) => CellValue
  /** Cell contents. Defaults to the value as text. */
  render?: (row: T) => ReactNode
  align?: 'left' | 'right'
  /** Any CSS width, e.g. "9rem". */
  width?: string
  /** Sortable unless set to false. */
  sortable?: boolean
}

export interface ResultsTableProps<T> {
  columns: Column<T>[]
  rows: T[]
  getRowId: (row: T) => string
  emptyMessage?: string
  /** Base name of the exported file, without the extension. */
  csvName?: string
  /** Optional per-row tint. Never the only signal: pair it with a visible column. */
  rowStatus?: (row: T) => RowTone | undefined
}

interface SortState {
  key: string
  dir: 'asc' | 'desc'
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

const ROW_TONES: Record<RowTone, string> = {
  ok: 'bg-ok/10',
  warn: 'bg-warn/10',
  danger: 'bg-danger/10',
}

function cellValue<T>(column: Column<T>, row: T): CellValue {
  if (column.value) return column.value(row)
  return (row as Record<string, unknown>)[column.key] as CellValue
}

function asText(value: CellValue): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'boolean') return value ? 'yes' : 'no'
  return String(value)
}

function compare(a: CellValue, b: CellValue): number {
  const aEmpty = a === null || a === undefined || a === ''
  const bEmpty = b === null || b === undefined || b === ''
  if (aEmpty || bEmpty) return aEmpty === bEmpty ? 0 : aEmpty ? 1 : -1
  if (typeof a === 'number' && typeof b === 'number') return a - b
  if (typeof a === 'boolean' && typeof b === 'boolean') return Number(a) - Number(b)
  return collator.compare(asText(a), asText(b))
}

function csvCell(value: CellValue): string {
  const text = asText(value)
  // A leading =, + or @ makes Excel treat the cell as a formula.
  const guarded = /^[=+@\t\r]/.test(text) ? `'${text}` : text
  return /["\r\n,]/.test(guarded) ? `"${guarded.replace(/"/g, '""')}"` : guarded
}

function downloadCsv(name: string, headers: string[], rows: CellValue[][]) {
  const lines = [headers.map(csvCell).join(','), ...rows.map((row) => row.map(csvCell).join(','))]
  // The BOM is what makes Excel read the file as UTF-8.
  const blob = new Blob(['\uFEFF' + lines.join('\r\n') + '\r\n'], {
    type: 'text/csv;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = name.toLowerCase().endsWith('.csv') ? name : `${name}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

/**
 * Sortable, filterable results grid with CSV export. Sorting, filtering and the
 * export all run off the same per-column value, so the file matches the screen.
 */
export function ResultsTable<T>({
  columns,
  rows,
  getRowId,
  emptyMessage = 'No results yet.',
  csvName = 'results',
  rowStatus,
}: ResultsTableProps<T>) {
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<SortState | null>(null)

  // One pass over the data per change, reused by filter, sort, render and export.
  const indexed = useMemo(
    () =>
      rows.map((row) => {
        const values = columns.map((column) => cellValue(column, row))
        return {
          row,
          id: getRowId(row),
          values,
          haystack: values.map(asText).join(' ').toLowerCase(),
        }
      }),
    [rows, columns, getRowId],
  )

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (needle === '') return indexed
    return indexed.filter((entry) => entry.haystack.includes(needle))
  }, [indexed, query])

  const sorted = useMemo(() => {
    if (sort === null) return filtered
    const index = columns.findIndex((column) => column.key === sort.key)
    if (index === -1) return filtered
    const sign = sort.dir === 'asc' ? 1 : -1
    return filtered.slice().sort((a, b) => sign * compare(a.values[index], b.values[index]))
  }, [filtered, sort, columns])

  // asc, then desc, then back to the order the job produced.
  const toggleSort = (key: string) => {
    setSort((prev) => {
      if (prev === null || prev.key !== key) return { key, dir: 'asc' }
      return prev.dir === 'asc' ? { key, dir: 'desc' } : null
    })
  }

  const onExport = () => {
    downloadCsv(
      csvName,
      columns.map((column) => column.header),
      sorted.map((entry) => entry.values),
    )
  }

  const showing = sorted.length === rows.length ? `${rows.length}` : `${sorted.length} of ${rows.length}`

  return (
    <div className="flex min-h-0 flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1">
          <Search
            size={14}
            aria-hidden
            className="pointer-events-none absolute top-1/2 left-2 -translate-y-1/2 text-fg-muted"
          />
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Filter results"
            aria-label="Filter results"
            className={cn(fieldClass, focusRing, 'pl-7')}
          />
        </div>
        <span className="text-xs whitespace-nowrap text-fg-muted tabular-nums">
          {showing} {rows.length === 1 ? 'row' : 'rows'}
        </span>
        <Button
          onClick={onExport}
          disabled={sorted.length === 0}
          icon={<Download size={14} aria-hidden />}
        >
          Export CSV
        </Button>
      </div>

      <div className="max-h-[70vh] min-h-0 overflow-auto rounded border border-border">
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr>
              {columns.map((column) => {
                const active = sort?.key === column.key
                const sortable = column.sortable !== false
                return (
                  <th
                    key={column.key}
                    scope="col"
                    style={column.width ? { width: column.width } : undefined}
                    aria-sort={
                      active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'
                    }
                    className={cn(
                      'sticky top-0 z-10 border-b border-border bg-surface-2 px-2 py-1.5 font-display text-[11px] font-semibold tracking-wide whitespace-nowrap text-fg-muted uppercase',
                      column.align === 'right' ? 'text-right' : 'text-left',
                    )}
                  >
                    {sortable ? (
                      <button
                        type="button"
                        onClick={() => toggleSort(column.key)}
                        className={cn(
                          'inline-flex items-center gap-1 rounded hover:text-fg',
                          focusRing,
                          active && 'text-fg',
                        )}
                      >
                        {column.header}
                        {active ? (
                          sort.dir === 'asc' ? (
                            <ChevronUp size={12} aria-hidden />
                          ) : (
                            <ChevronDown size={12} aria-hidden />
                          )
                        ) : (
                          <ChevronsUpDown size={12} aria-hidden className="opacity-40" />
                        )}
                      </button>
                    ) : (
                      column.header
                    )}
                  </th>
                )
              })}
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length}
                  className="px-2 py-8 text-center text-sm text-fg-muted"
                >
                  {rows.length === 0 ? emptyMessage : 'No rows match the filter.'}
                </td>
              </tr>
            ) : (
              sorted.map((entry) => {
                const tone = rowStatus?.(entry.row)
                return (
                  <tr
                    key={entry.id}
                    className={cn(
                      'border-b border-border last:border-b-0 hover:bg-surface-3',
                      tone && ROW_TONES[tone],
                    )}
                  >
                    {columns.map((column, index) => (
                      <td
                        key={column.key}
                        className={cn(
                          'px-2 py-1 whitespace-nowrap text-fg',
                          column.align === 'right' ? 'text-right tabular-nums' : 'text-left',
                        )}
                      >
                        {column.render ? column.render(entry.row) : asText(entry.values[index])}
                      </td>
                    ))}
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
