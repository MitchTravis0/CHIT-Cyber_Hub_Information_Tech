import { useCallback, useMemo, useState } from 'react'
import { CopyCheck, FolderOpen, LayoutList, Square, Table2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  ResultsTable,
  Select,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { formatBytes } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { pickDuplicateFolder, startDuplicateScan, type Group } from './api'
import {
  csvBase,
  mergeGroups,
  modifiedLabel,
  summaryLine,
  toRows,
  totals,
  type Row,
} from './groups'

const SIZES = [
  { value: '1024', label: '1 KB and up' },
  { value: '102400', label: '100 KB and up' },
  { value: '1048576', label: '1 MB and up' },
  { value: '10485760', label: '10 MB and up' },
  { value: '104857600', label: '100 MB and up' },
]

// Rendering five thousand open cards would take the page down, so the tail is
// folded away rather than dropped.
const CARDS_OPEN = 20

export default function DuplicateFinderPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Group>()

  const [path, setPath] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)
  const [minBytes, setMinBytes] = useState('1024')
  const [scanned, setScanned] = useState('')
  const [view, setView] = useState<'cards' | 'table'>('cards')

  const runScan = useCallback(
    async (target: string) => {
      const text = target.trim()
      if (text === '') {
        setPathError('Choose a folder to look in, or type the full path to it.')
        return
      }
      setPathError(null)
      setPath(text)
      setScanned(text)
      await start(() => startDuplicateScan({ path: text, minBytes: Number(minBytes) }))
    },
    [minBytes, start],
  )

  const groups = useMemo(() => mergeGroups(results), [results])
  const counts = useMemo(() => totals(groups), [groups])
  const rows = useMemo(() => toRows(groups), [groups])

  const summary = done?.summary ?? {}
  const filesScanned = typeof summary.scanned === 'number' ? summary.scanned : 0
  const note = typeof summary.note === 'string' ? summary.note : ''

  const columns = useMemo<Column<Row>[]>(
    () => [
      { key: 'group', header: 'Group', align: 'right', width: '6rem' },
      {
        key: 'bytes',
        header: 'Size',
        align: 'right',
        width: '8rem',
        render: (row) => formatBytes(row.bytes),
      },
      { key: 'count', header: 'Copies', align: 'right', width: '6rem' },
      { key: 'name', header: 'File', width: '14rem' },
      { key: 'path', header: 'Full path' },
      {
        key: 'modified',
        header: 'Modified',
        width: '12rem',
        render: (row) => modifiedLabel(row),
      },
    ],
    [],
  )

  return (
    <ToolShell
      title="Duplicate File Finder"
      description="Find files whose contents are identical, so you can see what is worth deleting."
      help={
        <>
          <p>
            This compares the <strong>contents</strong> of files, not their names. Two files called{' '}
            <code>Report.docx</code> and <code>Report (1).docx</code> are only reported if every
            byte in them is the same, and a photo saved twice under completely different names still
            shows up. Files smaller than the size you pick are ignored, and empty files are never
            reported: every empty file is identical to every other one, which is true and useless.
          </p>
          <p className="mt-1.5">
            It works in three passes so it does not have to read your whole drive. First it groups
            files by their exact size, which costs nothing. Then it reads the first 64 KB of each
            candidate, which throws out nearly everything. Only files that survive both are read all
            the way through. The biggest files are compared first, so if you stop it early you
            already have the results worth acting on.
          </p>
          <p className="mt-1.5">
            <strong>Nothing is ever deleted, moved or changed by this tool.</strong> It shows you
            the paths and you delete in Explorer or Finder, where the recycle bin will catch a
            mistake. The first copy in each group is marked "keep this one" purely because it was
            found first; look at the paths and keep whichever is in the right folder.
          </p>
        </>
      }
      actions={
        <div className="flex items-center gap-1 rounded border border-border bg-surface-2 p-0.5">
          <Button
            size="sm"
            variant={view === 'cards' ? 'primary' : 'ghost'}
            onClick={() => setView('cards')}
            icon={<LayoutList size={14} aria-hidden />}
            aria-pressed={view === 'cards'}
          >
            Cards
          </Button>
          <Button
            size="sm"
            variant={view === 'table' ? 'primary' : 'ghost'}
            onClick={() => setView('table')}
            icon={<Table2 size={14} aria-hidden />}
            aria-pressed={view === 'table'}
          >
            Table
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void runScan(path)
          }}
        >
          <div className="min-w-56 flex-1">
            <TextInput
              label="Folder to look in"
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder="/home/you/Documents"
              spellCheck={false}
              autoComplete="off"
              error={pathError ?? undefined}
              hint="Every folder inside it is searched too."
            />
          </div>
          <Select
            label="Smallest file to compare"
            options={SIZES}
            value={minBytes}
            onChange={(event) => setMinBytes(event.target.value)}
            hint="Raise this to finish sooner and ignore small clutter."
          />
          <Button
            type="submit"
            variant="primary"
            disabled={running}
            icon={<CopyCheck size={14} aria-hidden />}
          >
            Find duplicates
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Cancel
            </Button>
          )}
          <Button
            disabled={running}
            icon={<FolderOpen size={14} aria-hidden />}
            onClick={() => {
              void pickDuplicateFolder().then((chosen) => {
                if (chosen !== null && chosen !== '') setPath(chosen)
              })
            }}
          >
            Choose folder
          </Button>
        </form>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Starting the scan' : progress.message}
          />
        )}

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {done !== null && (
          <div className="rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-sm font-medium text-fg">
              {summaryLine(counts, filesScanned, done.durationMs, done.cancelled)}
            </p>
            {note !== '' && <p className="mt-1 text-xs text-warn">{note}</p>}
          </div>
        )}

        {scanned === '' ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Choose a folder and press Find duplicates. Nothing is deleted: this only tells you what
            is identical.
          </p>
        ) : view === 'table' ? (
          <ResultsTable
            columns={columns}
            rows={rows}
            getRowId={(row) => row.path}
            csvName={csvBase(scanned)}
            emptyMessage={
              running
                ? 'Comparing, groups appear as each one is confirmed.'
                : 'Nothing in that folder is an exact copy of anything else.'
            }
          />
        ) : groups.length === 0 ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            {running
              ? 'Comparing, groups appear as each one is confirmed.'
              : 'Nothing in that folder is an exact copy of anything else.'}
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {groups.slice(0, CARDS_OPEN).map((group) => (
              <GroupCard key={group.hash} group={group} />
            ))}
            {groups.length > CARDS_OPEN && (
              <details className="rounded border border-border bg-surface-2">
                <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
                  Show the remaining {groups.length - CARDS_OPEN} groups
                </summary>
                <div className="flex flex-col gap-2 px-3 pt-1 pb-3">
                  {groups.slice(CARDS_OPEN).map((group) => (
                    <GroupCard key={group.hash} group={group} />
                  ))}
                </div>
              </details>
            )}
          </div>
        )}
      </div>
    </ToolShell>
  )
}

function GroupCard({ group }: { group: Group }) {
  const files = group.files ?? []
  return (
    <div className="rounded border border-border bg-surface-2 px-3 py-2">
      <p className="flex flex-wrap items-baseline gap-2 text-sm">
        <span className="font-medium text-fg">{formatBytes(group.bytes)}</span>
        <span className="text-fg-muted">
          &times; {group.count} {group.count === 1 ? 'copy' : 'copies'}
        </span>
        <span className="text-warn">{formatBytes(group.waste)} wasted</span>
      </p>
      <ol className="mt-1.5 flex flex-col gap-1">
        {files.map((file, index) => (
          <li key={file.path} className="flex items-start gap-2 text-xs">
            <span className={index === 0 ? 'w-24 shrink-0 text-ok' : 'w-24 shrink-0 text-fg-muted'}>
              {index === 0 ? 'keep this one' : `copy ${index + 1}`}
            </span>
            <span className="min-w-0 flex-1 break-all text-fg">{file.path}</span>
            <span className="shrink-0 text-fg-muted">{modifiedLabel(file)}</span>
            <CopyButton value={file.path} />
          </li>
        ))}
      </ol>
    </div>
  )
}
