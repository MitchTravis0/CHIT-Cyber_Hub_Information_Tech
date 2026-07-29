import { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronRight, FolderOpen, PieChart, Square } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  ResultsTable,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { formatBytes, formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { diskScanHome, largestFrom, pickScanFolder, startDiskScan, type Entry } from './api'
import { TreeMap } from './TreeMap'
import { crumbLabel, crumbs, csvBase, sharePct } from './treemap'

export default function DiskVisualizerPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Entry>()

  const [path, setPath] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)
  const [scanned, setScanned] = useState('')

  useEffect(() => {
    void diskScanHome().then((home) => {
      if (home !== null && home !== '') setPath((current) => (current === '' ? home : current))
    })
  }, [])

  const runScan = useCallback(
    async (target: string) => {
      const text = target.trim()
      if (text === '') {
        setPathError('Choose a folder to scan, or type the full path to it.')
        return
      }
      setPathError(null)
      setPath(text)
      setScanned(text)
      await start(() => startDiskScan({ path: text }))
    },
    [start],
  )

  // A backend is allowed to re-emit, so the page folds by path before rendering.
  const entries = useMemo(() => {
    const byPath = new Map<string, Entry>()
    for (const entry of results) byPath.set(entry.path, entry)
    return Array.from(byPath.values()).sort((a, b) => b.bytes - a.bytes)
  }, [results])

  const total = useMemo(() => entries.reduce((sum, entry) => sum + entry.bytes, 0), [entries])
  const largest = useMemo(() => largestFrom(done?.summary), [done])
  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''

  const columns = useMemo<Column<Entry>[]>(
    () => [
      {
        key: 'name',
        header: 'Name',
        render: (row) =>
          row.dir ? (
            <button
              type="button"
              onClick={() => void runScan(row.path)}
              className="text-accent hover:underline"
            >
              {row.name}
            </button>
          ) : (
            row.name
          ),
      },
      { key: 'dir', header: 'Kind', width: '6rem', value: (row) => (row.dir ? 'Folder' : 'File') },
      {
        key: 'bytes',
        header: 'Size',
        align: 'right',
        width: '8rem',
        render: (row) => formatBytes(row.bytes),
      },
      {
        key: 'share',
        header: 'Share',
        align: 'right',
        width: '6rem',
        value: (row) => sharePct(row.bytes, total),
        render: (row) => `${sharePct(row.bytes, total)}%`,
      },
      {
        key: 'files',
        header: 'Files',
        align: 'right',
        width: '7rem',
        render: (row) => row.files.toLocaleString(),
      },
    ],
    [runScan, total],
  )

  const trail = scanned === '' ? [] : crumbs(scanned)
  const summary = done?.summary ?? {}
  const totalFiles = typeof summary.files === 'number' ? summary.files : 0
  const totalDirs = typeof summary.dirs === 'number' ? summary.dirs : 0

  return (
    <ToolShell
      title="Disk Space Visualizer"
      description="See what is actually filling a drive, folder by folder, and drill into the biggest one."
      help={
        <>
          <p>
            Point this at a folder and it measures everything inside it, then draws one box per
            subfolder sized by how much space that subfolder uses. The biggest box is what is
            filling the drive. Click a box to go into it, and use the breadcrumb at the top to come
            back out. To measure a whole drive, scan <code>C:\</code> on Windows or <code>/</code>{' '}
            on macOS and Linux.
          </p>
          <p className="mt-1.5">
            Nothing is deleted, moved or opened. The tool only counts. When you have found the
            culprit, copy its path and delete it in Explorer or Finder, where the recycle bin will
            still catch a mistake.
          </p>
          <p className="mt-1.5">
            Shortcuts, symbolic links and junctions are stepped over rather than followed, so a
            scan cannot get stuck in a loop. A file that genuinely exists under two names, which is
            common inside <code>node_modules</code>, is counted under both, the same way Explorer's
            Properties dialog counts it. Folders belonging to
            another user or to Windows itself cannot be opened without admin rights, and CHIT never
            asks for them; when that happens the note under the total says how many folders were
            missed, and the real figure is higher than the one shown.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {trail.length > 1 && (
          <nav aria-label="Folders above this one" className="flex flex-wrap items-center gap-0.5">
            {trail.map((crumb, index) => (
              <span key={crumb} className="flex items-center gap-0.5">
                {index > 0 && <ChevronRight size={12} aria-hidden className="text-fg-muted" />}
                {index === trail.length - 1 ? (
                  <span className="px-1 text-xs font-medium text-fg">{crumbLabel(crumb)}</span>
                ) : (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={running}
                    onClick={() => void runScan(crumb)}
                  >
                    {crumbLabel(crumb)}
                  </Button>
                )}
              </span>
            ))}
          </nav>
        )}

        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void runScan(path)
          }}
        >
          <div className="min-w-56 flex-1">
            <TextInput
              label="Folder to measure"
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder="/home/you"
              spellCheck={false}
              autoComplete="off"
              error={pathError ?? undefined}
              hint="Everything inside this folder is counted, including subfolders."
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={running}
            icon={<PieChart size={14} aria-hidden />}
          >
            Scan
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
              void pickScanFolder().then((chosen) => {
                if (chosen !== null && chosen !== '') void runScan(chosen)
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

        {scanned !== '' && entries.length > 0 && (
          <div className="rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-sm font-medium text-fg">
              Measured {formatBytes(total)} in {totalFiles.toLocaleString()} files and{' '}
              {totalDirs.toLocaleString()} folders
              {done !== null && (
                <span className="font-normal text-fg-muted">
                  {' '}
                  in {formatDuration(done.durationMs)}
                </span>
              )}
              {done?.cancelled === true && (
                <span className="font-normal text-fg-muted"> (stopped early)</span>
              )}
            </p>
            {note !== '' && <p className="mt-1 text-xs text-warn">{note}</p>}
          </div>
        )}

        {scanned === '' ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Press Scan to measure everything inside this folder.
          </p>
        ) : (
          <>
            <TreeMap entries={entries} total={total} onOpen={(entry) => {
              if (entry.dir && !running) void runScan(entry.path)
            }} />

            <ResultsTable
              columns={columns}
              rows={entries}
              getRowId={(row) => row.path}
              csvName={csvBase(scanned)}
              emptyMessage={
                running ? 'Measuring, folders appear as each one finishes.' : 'That folder is empty.'
              }
            />

            {largest.length > 0 && (
              <div className="rounded border border-border bg-surface-2 px-3 py-2">
                <h2 className="mb-1.5 text-xs font-semibold tracking-wide text-fg-muted uppercase">
                  Biggest single files
                </h2>
                <ul className="flex flex-col gap-1">
                  {largest.map((file) => (
                    <li key={file.path} className="flex items-start gap-2 text-sm">
                      <span className="w-20 shrink-0 text-right tabular-nums text-fg">
                        {formatBytes(file.bytes)}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="text-fg">{file.name}</span>
                        <span className="block text-xs break-all text-fg-muted">{file.path}</span>
                      </span>
                      <CopyButton value={file.path} />
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </>
        )}
      </div>
    </ToolShell>
  )
}
