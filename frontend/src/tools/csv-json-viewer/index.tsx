import { useMemo, useRef, useState } from 'react'
import type { ChangeEvent } from 'react'
import { FileDown, Trash2, TriangleAlert, Upload } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  Textarea,
  ToolShell,
  type Column,
} from '../../components'
import { downloadText } from '../../lib/download'
import { formatBytes } from '../../lib/format'
import { formatCsv, type Delimiter } from './csv'
import { formatJson } from './json'
import {
  MAX_COLUMNS,
  MAX_FILE_BYTES,
  MAX_TABLE_ROWS,
  columnKey,
  csvNameFor,
  hasDuplicateHeaders,
  parseInput,
  type DelimiterChoice,
  type Format,
  type FormatChoice,
  type Table,
} from './table'

const DESCRIPTION =
  'Open a CSV or JSON export, read it as a sortable table, and convert between the two.'

const FORMAT_OPTIONS = [
  { value: 'auto', label: 'Detect automatically' },
  { value: 'csv', label: 'CSV or tab separated' },
  { value: 'json', label: 'JSON' },
]

const DELIMITER_OPTIONS = [
  { value: 'auto', label: 'Detect automatically' },
  { value: ',', label: 'Comma' },
  { value: ';', label: 'Semicolon' },
  { value: '\t', label: 'Tab' },
]

const OUTPUT_OPTIONS = [
  { value: 'json', label: 'JSON' },
  { value: 'csv', label: 'CSV' },
]

const DELIMITER_NAMES: Record<Delimiter, string> = {
  ',': 'Comma',
  ';': 'Semicolon',
  '\t': 'Tab',
}

const TOO_MUCH_TEXT =
  'That is more than 5 MB of text and this tool can handle up to 5 MB. Paste a smaller part of it.'

const UNREADABLE =
  'That file could not be read. It may have been moved or deleted since you picked it. Try choosing it again.'

const EMPTY_TABLE: Table = { headers: [], rows: [], notes: [] }

interface TableRow {
  i: number
  cells: string[]
}

function Warning({ children }: { children: string }) {
  return (
    <p className="flex items-start gap-1.5 text-xs text-warn">
      <TriangleAlert size={14} aria-hidden className="mt-0.5 shrink-0" />
      {children}
    </p>
  )
}

export default function CsvJsonViewerPage() {
  const [text, setText] = useState('')
  const [fileName, setFileName] = useState('')
  const [fileError, setFileError] = useState<string | null>(null)
  const [formatChoice, setFormatChoice] = useState<FormatChoice>('auto')
  const [delimiterChoice, setDelimiterChoice] = useState<DelimiterChoice>('auto')
  const [hasHeader, setHasHeader] = useState(true)
  const [outputFormat, setOutputFormat] = useState<Format>('json')
  const [lastInputFormat, setLastInputFormat] = useState<Format>('csv')
  const fileRef = useRef<HTMLInputElement>(null)

  const oversize = text.length > MAX_FILE_BYTES

  const reading = useMemo(
    () => parseInput(oversize ? '' : text, formatChoice, delimiterChoice, hasHeader),
    [text, oversize, formatChoice, delimiterChoice, hasHeader],
  )

  // The output format follows the input each time the input format changes, and
  // the user can pick something else afterwards.
  if (reading.format !== lastInputFormat) {
    setLastInputFormat(reading.format)
    setOutputFormat(reading.format === 'csv' ? 'json' : 'csv')
  }

  const result = reading.result
  const table = result.ok ? result.table : EMPTY_TABLE

  const view = useMemo(() => {
    const columns: Column<TableRow>[] = table.headers
      .slice(0, MAX_COLUMNS)
      .map((header, i) => ({ key: columnKey(i), header, align: 'left', value: (row) => row.cells[i] }))
    const rows = table.rows.slice(0, MAX_TABLE_ROWS).map((cells, i) => ({ i, cells }))
    return { columns, rows }
  }, [table])

  const output = useMemo(
    () => (outputFormat === 'json' ? formatJson(table) : formatCsv(table)),
    [table, outputFormat],
  )

  const onFileChosen = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    // Reset the input so choosing the same file twice fires onChange both times.
    event.target.value = ''
    if (file === undefined) return
    if (file.size > MAX_FILE_BYTES) {
      setFileError(
        `That file is ${formatBytes(file.size)} and this tool can open files up to 5 MB. Open it in a text editor and take the part you need, or use the source system's own filter to export fewer rows.`,
      )
      return
    }
    try {
      const content = await file.text()
      setText(content)
      setFileName(file.name)
      setFileError(null)
    } catch {
      setFileError(UNREADABLE)
    }
  }

  const onClear = () => {
    setText('')
    setFileName('')
    setFileError(null)
  }

  const onDownload = () => {
    // No UTF-8 BOM, unlike the table's Export CSV: the BOM is there to make Excel
    // read the export as UTF-8 and it would corrupt a JSON file read by anything else.
    // downloadText is used rather than downloadJson for the same reason: output is
    // already the exact text to write, in either format.
    downloadText(
      `${csvNameFor(fileName)}.${outputFormat}`,
      output,
      outputFormat === 'json' ? 'application/json;charset=utf-8' : 'text/csv;charset=utf-8',
    )
  }

  const alert = fileError ?? (oversize ? TOO_MUCH_TEXT : null)
  const empty = text.trim() === ''
  const readable = !empty && !oversize
  const totalRows = table.rows.length
  const totalColumns = table.headers.length
  const tableMessage =
    totalColumns === 0
      ? 'That file parsed correctly but has no records in it.'
      : totalRows === 0
        ? 'That file has a header row and no data rows under it.'
        : null

  return (
    <ToolShell
      title="CSV / JSON Viewer and Converter"
      description={DESCRIPTION}
      help={
        <>
          <p>
            Paste a CSV or JSON file into the box, or press Choose a file. The tool works out which
            format it is and which character separates the columns, and shows you the result as a
            table you can filter and sort. Nothing is sent anywhere: the file is read inside CHIT
            and never leaves this machine.
          </p>
          <p className="mt-2">
            CSV files vary. If the columns look wrong, set the Delimiter box yourself: exports from
            European systems usually use a semicolon, and a <code>.tsv</code> file uses a tab. If
            the first line of your file is data rather than column names, untick "First row is a
            header" and the columns will be called Column 1, Column 2 and so on. Rows that have
            more or fewer values than the header row are still shown in full, and a note tells you
            how many there were.
          </p>
          <p className="mt-2">
            Converting CSV to JSON always produces text values, even for something that looks like a
            number, because a CSV file does not record what anything is meant to be. Converting the
            other way always writes commas as the separator, and anything that was a list or a
            nested record inside the JSON becomes plain text in the cell. The table on screen shows
            up to 2,000 rows so the app stays quick, but Copy and Download always give you
            everything.
          </p>
        </>
      }
      actions={
        <Button onClick={onClear} disabled={text === ''} icon={<Trash2 size={14} aria-hidden />}>
          Clear
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <Select
            label="Input format"
            options={FORMAT_OPTIONS}
            value={formatChoice}
            onChange={(event) => setFormatChoice(event.target.value as FormatChoice)}
            className="w-52"
          />
          {reading.format === 'csv' && (
            <>
              <Select
                label="Delimiter"
                options={DELIMITER_OPTIONS}
                value={delimiterChoice}
                onChange={(event) => setDelimiterChoice(event.target.value as DelimiterChoice)}
                hint={
                  delimiterChoice === 'auto'
                    ? `Detected: ${DELIMITER_NAMES[reading.delimiter]}`
                    : undefined
                }
                className="w-48"
              />
              <label className="flex h-8 items-center gap-2 text-sm text-fg">
                <input
                  type="checkbox"
                  checked={hasHeader}
                  onChange={(event) => setHasHeader(event.target.checked)}
                  className="size-4 accent-[var(--accent)]"
                />
                First row is a header
              </label>
            </>
          )}
          <input
            ref={fileRef}
            type="file"
            accept=".csv,.tsv,.txt,.json,text/csv,text/plain,application/json"
            className="hidden"
            onChange={(event) => void onFileChosen(event)}
          />
          <Button onClick={() => fileRef.current?.click()} icon={<Upload size={14} aria-hidden />}>
            Choose a file
          </Button>
        </div>

        {alert !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {alert}
          </p>
        )}

        <Textarea
          id="csv-json-input"
          label="Data to read"
          value={text}
          onChange={(event) => setText(event.target.value)}
          className="h-56 resize-y font-mono text-xs"
          spellCheck={false}
          placeholder="Paste CSV or JSON here, or use Choose a file."
        />

        {empty && !oversize && (
          <p className="text-sm text-fg-muted">
            Paste a CSV or JSON file above, or press Choose a file, and it will appear here as a
            table.
          </p>
        )}

        {readable && !result.ok && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {result.error}
          </p>
        )}

        {readable && result.ok && (
          <>
            {table.notes.map((note) => (
              <Warning key={note}>{note}</Warning>
            ))}

            {tableMessage !== null ? (
              <p className="text-sm text-fg-muted">{tableMessage}</p>
            ) : (
              <ResultsTable
                columns={view.columns}
                rows={view.rows}
                getRowId={(row) => String(row.i)}
                emptyMessage="Nothing to show. Paste some data above, or use Choose a file."
                csvName={csvNameFor(fileName)}
              />
            )}

            {totalRows > MAX_TABLE_ROWS && (
              <Warning>
                {`Showing the first ${MAX_TABLE_ROWS.toLocaleString()} rows of ${totalRows.toLocaleString()}. The converted output and the download below contain all ${totalRows.toLocaleString()}.`}
              </Warning>
            )}
            {totalColumns > MAX_COLUMNS && (
              <Warning>
                {`Showing the first ${MAX_COLUMNS.toLocaleString()} columns of ${totalColumns.toLocaleString()}. The converted output and the download below contain all ${totalColumns.toLocaleString()}.`}
              </Warning>
            )}

            <section className="flex flex-col gap-2 border-t border-border pt-4">
              <div className="flex flex-wrap items-end gap-2">
                <Select
                  label="Convert to"
                  options={OUTPUT_OPTIONS}
                  value={outputFormat}
                  onChange={(event) => setOutputFormat(event.target.value as Format)}
                  className="w-36"
                />
                <CopyButton value={output} label="Copy" />
                <Button onClick={onDownload} icon={<FileDown size={14} aria-hidden />}>
                  Download
                </Button>
              </div>

              {outputFormat === 'json' && hasDuplicateHeaders(table.headers) && (
                <Warning>
                  Two or more columns have the same name, and a JSON record cannot hold the same
                  name twice, so only the right-hand one is in the output. Rename the columns in the
                  source file if you need both.
                </Warning>
              )}

              <pre className="max-h-96 overflow-auto rounded border border-border bg-surface-2 p-2 font-mono text-xs whitespace-pre text-fg">
                {output}
              </pre>
            </section>
          </>
        )}
      </div>
    </ToolShell>
  )
}
