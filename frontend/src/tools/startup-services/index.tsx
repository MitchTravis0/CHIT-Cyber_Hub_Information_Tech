import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw, TriangleAlert } from 'lucide-react'
import {
  Button,
  ResultsTable,
  Spinner,
  StatusDot,
  ToolShell,
  type Column,
} from '../../components'
import { errorMessage } from '../../lib/format'
import { startupItems, type Item, type Report } from './api'
import { concernCount, countLine, filterItems, startModeLabel, stateLabel, type Filter } from './items'

const FILTERS: Array<{ id: Filter; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'startup', label: 'Startup only' },
  { id: 'services', label: 'Services only' },
  { id: 'automatic', label: 'Automatic only' },
]

export default function StartupServicesPage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await startupItems())
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

  const items = useMemo(() => report?.items ?? [], [report])
  const shown = useMemo(() => filterItems(items, filter), [items, filter])
  const concerns = useMemo(() => concernCount(items), [items])

  const columns = useMemo<Column<Item>[]>(
    () => [
      { key: 'name', header: 'Name', width: '14rem' },
      {
        key: 'kind',
        header: 'Kind',
        width: '6rem',
        value: (row) => (row.kind === 'startup' ? 'Startup' : 'Service'),
      },
      { key: 'source', header: 'Found in', width: '12rem' },
      {
        key: 'startMode',
        header: 'Starts',
        width: '8rem',
        value: (row) => startModeLabel(row.startMode),
        render: (row) => (
          <span className={row.startMode === '' ? 'text-fg-muted italic' : undefined}>
            {startModeLabel(row.startMode)}
          </span>
        ),
      },
      {
        key: 'state',
        header: 'Now',
        width: '6rem',
        value: (row) => stateLabel(row.state),
        render: (row) =>
          row.state === '' ? (
            ''
          ) : (
            <StatusDot
              status={row.state === 'running' ? 'ok' : 'idle'}
              label={stateLabel(row.state)}
            />
          ),
      },
      {
        key: 'concern',
        header: 'Worth a look',
        width: '10rem',
        value: (row) => (row.concern === '' ? '' : 'Yes'),
        render: (row) =>
          row.concern === '' ? (
            ''
          ) : (
            <span className="text-warn" title={row.concern}>
              Yes
            </span>
          ),
      },
      {
        key: 'command',
        header: 'Runs',
        render: (row) => <span className="text-xs break-all">{row.command}</span>,
      },
    ],
    [],
  )

  return (
    <ToolShell
      title="Startup and Services Viewer"
      description="List what starts with this machine and what services are set to run, and flag the odd ones."
      help={
        <>
          <p>
            This is everything set to launch when the computer starts or when you sign in, plus
            every service that is configured on it. On Windows it includes the Run keys that Task
            Manager's Startup tab does not show, including the 32-bit ones, which is where a
            surprising amount of junk hides.
          </p>
          <p className="mt-1.5">
            <strong>Nothing here can be changed from CHIT.</strong> Turning a startup entry off or
            stopping a service needs administrator rights, and CHIT never asks for them. Use this
            to find the entry, then change it in Task Manager, <code>msconfig</code> or{' '}
            <code>services.msc</code> on Windows, <code>systemctl</code> on Linux, or System
            Settings on a Mac.
          </p>
          <p className="mt-1.5">
            "Worth a look" is a hint, not a verdict. It flags things like a program starting from a
            temporary folder, a script rather than a program, or a hidden PowerShell command:
            patterns that are unusual for ordinary software. Plenty of perfectly legitimate software
            trips it, and CHIT does not check digital signatures, so read the entry and decide for
            yourself rather than deleting on the strength of the flag.
          </p>
        </>
      }
      actions={
        <Button
          onClick={() => void load()}
          disabled={loading}
          icon={<RefreshCw size={14} aria-hidden className={loading ? 'animate-spin' : undefined} />}
        >
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {error === null && loading && report === null && (
          <div className="flex items-center gap-2 text-sm text-fg-muted">
            <Spinner /> Reading startup entries
          </div>
        )}

        {report !== null && (
          <>
            <div>
              <p className="text-sm text-fg">{countLine(items)}</p>
              {report.note !== '' && <p className="mt-1 text-xs text-warn">{report.note}</p>}
            </div>

            <div className="flex flex-wrap gap-1">
              {FILTERS.map((option) => (
                <Button
                  key={option.id}
                  size="sm"
                  variant={filter === option.id ? 'primary' : 'ghost'}
                  aria-pressed={filter === option.id}
                  onClick={() => setFilter(option.id)}
                >
                  {option.label}
                </Button>
              ))}
              <Button
                size="sm"
                variant={filter === 'concern' ? 'primary' : 'ghost'}
                aria-pressed={filter === 'concern'}
                disabled={concerns === 0}
                onClick={() => setFilter('concern')}
                icon={<TriangleAlert size={14} aria-hidden />}
              >
                Worth a look ({concerns})
              </Button>
            </div>

            <ResultsTable
              columns={columns}
              rows={shown}
              getRowId={(row) => `${row.kind}|${row.source}|${row.name}`}
              csvName="startup-and-services"
              emptyMessage="Nothing was found. That is unusual: this computer may be locked down, or CHIT may not have been able to read the places these entries live."
              rowStatus={(row) => (row.concern === '' ? undefined : 'warn')}
            />

            {filter === 'concern' && shown.length > 0 && (
              <ul className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
                {shown.map((item) => (
                  <li key={`${item.source}|${item.name}`} className="text-sm">
                    <span className="font-medium text-fg">{item.name}</span>
                    <span className="ml-2 text-fg-muted">{item.source}</span>
                    <p className="mt-0.5 text-warn">{item.concern}</p>
                    <p className="text-xs break-all text-fg-muted">{item.command}</p>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </div>
    </ToolShell>
  )
}
