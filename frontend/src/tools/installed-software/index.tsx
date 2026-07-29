import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import {
  Button,
  ResultsTable,
  Spinner,
  ToolShell,
  type Column,
} from '../../components'
import { errorMessage, formatBytes } from '../../lib/format'
import { installedSoftware, type Program, type Report } from './api'
import {
  countLine,
  filterPrograms,
  hasAny,
  programId,
  sortPrograms,
} from './programs'

export default function InstalledSoftwarePage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('all')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const next = await installedSoftware()
      setReport(next)
      setFilter('all')
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const programs = useMemo(() => sortPrograms(report?.programs ?? []), [report])
  const sources = useMemo(() => report?.sources ?? [], [report])
  const shown = useMemo(() => filterPrograms(programs, filter), [programs, filter])

  const showDates = useMemo(() => hasAny(programs, 'installedOn'), [programs])
  const showSizes = useMemo(() => hasAny(programs, 'sizeBytes'), [programs])

  // A column nothing fills is dropped rather than rendered empty, which reads as
  // a bug. Same approach usb-history takes for its First seen column.
  const columns = useMemo<Column<Program>[]>(() => {
    const all: Column<Program>[] = [
      { key: 'name', header: 'Name' },
      {
        key: 'version',
        header: 'Version',
        width: '10rem',
        render: (row) =>
          row.version === '' ? (
            <span className="text-fg-muted italic">not recorded</span>
          ) : (
            <span className="font-mono text-xs">{row.version}</span>
          ),
      },
      { key: 'publisher', header: 'Publisher', width: '14rem' },
    ]
    if (showDates) {
      all.push({ key: 'installedOn', header: 'Installed', width: '9rem' })
    }
    if (showSizes) {
      all.push({
        key: 'sizeBytes',
        header: 'Size',
        align: 'right',
        width: '8rem',
        value: (row) => (row.sizeBytes === 0 ? null : row.sizeBytes),
        render: (row) => (row.sizeBytes === 0 ? '' : formatBytes(row.sizeBytes)),
      })
    }
    all.push({ key: 'source', header: 'Source', width: '12rem' })
    return all
  }, [showDates, showSizes])

  return (
    <ToolShell
      title="Installed Software List"
      description="List what is installed on this machine, with versions, and export it."
      help={
        <>
          <p>
            This is the list you would otherwise read out of Add or Remove Programs, already
            searchable and already exportable. Type into the filter box to narrow it, and press
            Export CSV to attach it to a ticket or paste it into an asset record.
          </p>
          <p className="mt-1.5">
            The <strong>Source</strong> column matters. On Windows there are three places software
            registers itself: the machine-wide list, the 32-bit view of it, and the current user's
            own. All three are read. Windows updates and the components of larger suites are
            deliberately left out, because four hundred numbered update entries bury the software you
            were looking for. Apps from the Microsoft Store are not in this list at all.
          </p>
          <p className="mt-1.5">
            On Linux this is what the package manager knows: anything unpacked from a tarball or
            installed by a script is invisible to it and so is invisible here. On macOS it is the
            applications, which is not everything installed: Homebrew packages, drivers and
            command-line tools are not applications.
          </p>
          <p className="mt-1.5">
            Nothing on this page installs, updates or removes anything. It only reads.
          </p>
        </>
      }
      actions={
        <Button
          onClick={() => void load()}
          disabled={loading}
          icon={<RefreshCw size={14} aria-hidden />}
        >
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {loading && <Spinner label="Reading installed software" />}

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {report !== null && (
          <>
            <div>
              <p className="text-sm text-fg">{countLine(programs, sources)}</p>
              <p className="mt-1 text-xs text-fg-muted">{report.note}</p>
            </div>

            {sources.length > 1 && (
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant={filter === 'all' ? 'primary' : 'secondary'}
                  onClick={() => setFilter('all')}
                >
                  All
                </Button>
                {sources.map((source) => (
                  <Button
                    key={source}
                    size="sm"
                    variant={filter === source ? 'primary' : 'secondary'}
                    onClick={() => setFilter(source)}
                  >
                    {source}
                  </Button>
                ))}
              </div>
            )}

            <ResultsTable
              columns={columns}
              rows={shown}
              getRowId={programId}
              csvName="installed-software"
              emptyMessage="Nothing was found. On Linux that usually means CHIT does not know this machine's package manager."
            />
          </>
        )}
      </div>
    </ToolShell>
  )
}
