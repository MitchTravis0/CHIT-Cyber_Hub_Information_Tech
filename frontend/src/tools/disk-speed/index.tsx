import { useCallback, useMemo, useState } from 'react'
import { FolderOpen, HardDrive, Square } from 'lucide-react'
import {
  Button,
  ProgressBar,
  ResultsTable,
  Select,
  StatusDot,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { formatBytes } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { pickBenchFolder, startDiskSpeed, type Sample } from './api'
import { phaseLabel, rateText, runLine, sampleId, secondsText } from './format'

const SIZES = [
  { value: '64', label: '64 MB' },
  { value: '256', label: '256 MB' },
  { value: '1024', label: '1024 MB' },
]

export default function DiskSpeedPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Sample>()

  const [path, setPath] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)
  const [sizeMb, setSizeMb] = useState('256')

  const onStart = useCallback(async () => {
    const folder = path.trim()
    if (folder === '') {
      setPathError('Choose a folder to test, or type the full path to it.')
      return
    }
    setPathError(null)
    await start(() => startDiskSpeed({ path: folder, sizeMb: Number(sizeMb) }))
  }, [path, sizeMb, start])

  const summary = done?.summary ?? {}
  const num = (key: string) => (typeof summary[key] === 'number' ? (summary[key] as number) : 0)
  const text = (key: string) => (typeof summary[key] === 'string' ? (summary[key] as string) : '')
  const cacheBypassed = summary.cacheBypassed !== false

  const columns = useMemo<Column<Sample>[]>(
    () => [
      { key: 'phase', header: 'Phase', width: '7rem', value: (row) => phaseLabel(row.phase) },
      {
        key: 'bytes',
        header: 'Done',
        align: 'right',
        width: '8rem',
        render: (row) => formatBytes(row.bytes),
      },
      {
        key: 'seconds',
        header: 'At',
        align: 'right',
        width: '6rem',
        render: (row) => secondsText(row.seconds),
      },
      {
        key: 'mbps',
        header: 'Rate',
        align: 'right',
        width: '9rem',
        render: (row) => rateText(row.mbps),
      },
    ],
    [],
  )

  return (
    <ToolShell
      title="Disk Speed Test"
      description="Measure how fast a drive or a network share reads and writes, with a plain reading of the result."
      help={
        <>
          <p>
            This writes one temporary file into the folder you choose, reads it back, and reports how
            fast each went. Whatever drive that folder lives on is what gets measured, so point it at{' '}
            <code>C:</code> for the system drive, at a USB stick's folder for the stick, or at a
            mapped network drive to measure the share rather than the disk.
          </p>
          <p className="mt-1.5">
            Rough guide: an NVMe drive reads well over 1,000 MB/s, an ordinary SSD 300 to 600, a
            healthy spinning hard disk 80 to 160, and a USB 2.0 stick around 25. A drive that starts
            fast and then collapses part way through the run is the classic sign of a failing disk or
            a full SSD, and the table below the figures is where you see that shape.
          </p>
          <p className="mt-1.5">
            The file is deleted again on the way out, including if you press Stop or if something
            goes wrong. It is written inside the folder you picked and nowhere else. Nothing else on
            the drive is touched and nothing is read except the file CHIT just wrote.
          </p>
          <p className="mt-1.5">
            The figures are approximate. Antivirus, a backup running in the background, and anything
            else using the drive all change them, so run it twice before drawing a conclusion.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void onStart()
          }}
        >
          <div className="min-w-56 flex-1">
            <TextInput
              label="Folder to test"
              value={path}
              spellCheck={false}
              autoComplete="off"
              disabled={running}
              onChange={(event) => setPath(event.target.value)}
              error={pathError ?? undefined}
              hint="The folder the test file is written into."
            />
          </div>
          <Button
            disabled={running}
            icon={<FolderOpen size={14} aria-hidden />}
            onClick={() => {
              void pickBenchFolder().then((chosen) => {
                if (chosen === null || chosen === '') return
                setPath(chosen)
                setPathError(null)
              })
            }}
          >
            Choose folder
          </Button>
          <div className="w-40">
            <Select
              label="Test size"
              options={SIZES}
              value={sizeMb}
              disabled={running}
              onChange={(event) => setSizeMb(event.target.value)}
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={running || path.trim() === ''}
            icon={<HardDrive size={14} aria-hidden />}
          >
            Start
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Stop
            </Button>
          )}
        </form>

        <p className="text-xs text-fg-muted">
          A temporary file is written into that folder and deleted again, including if you press
          Stop.
        </p>

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Starting the test' : progress.message}
          />
        )}

        {done !== null && !done.cancelled && (
          <div className="rounded border border-border bg-surface-2 px-3 py-3">
            <div className="flex flex-wrap gap-8">
              <div>
                <p className="text-xs text-fg-muted">Write</p>
                <p className="text-2xl text-fg">{rateText(num('writeMbps'))}</p>
              </div>
              <div>
                <p className="text-xs text-fg-muted">Read</p>
                <p className="text-2xl text-fg">{rateText(num('readMbps'))}</p>
                {!cacheBypassed && (
                  <span className="mt-1 flex items-center gap-1.5 text-xs text-warn">
                    <StatusDot status="warn" />
                    may be from memory
                  </span>
                )}
              </div>
            </div>
            <p className="mt-2 text-sm text-fg">{text('verdict')}</p>
            <p className="mt-1 text-xs text-fg-muted">{text('note')}</p>
          </div>
        )}

        {done !== null && (
          <p className="text-sm text-fg">
            {runLine(num('sizeMb') || Number(sizeMb), done.durationMs)}
            {done.cancelled && <span className="text-fg-muted"> (stopped early)</span>}
          </p>
        )}

        <ResultsTable
          columns={columns}
          rows={results}
          getRowId={sampleId}
          csvName="disk-speed"
          emptyMessage={running ? 'Writing the test file.' : 'Choose a folder and press Start.'}
        />
      </div>
    </ToolShell>
  )
}
