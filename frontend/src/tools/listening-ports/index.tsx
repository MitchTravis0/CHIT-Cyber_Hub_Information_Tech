import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import {
  Button,
  ResultsTable,
  Spinner,
  StatusDot,
  ToolShell,
  type Column,
} from '../../components'
import { errorMessage } from '../../lib/format'
import { listeningPorts, type Entry, type Report } from './api'
import {
  countLine,
  entryId,
  filterEntries,
  protocolLabel,
  reachLabel,
  reachTone,
  sortEntries,
  type Filter,
} from './entries'

const FILTERS: Array<{ id: Filter; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'tcp', label: 'TCP' },
  { id: 'udp', label: 'UDP' },
  { id: 'reachable', label: 'Reachable from other machines' },
]

export default function ListeningPortsPage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await listeningPorts())
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

  const entries = useMemo(() => sortEntries(report?.entries ?? []), [report])
  const shown = useMemo(() => filterEntries(entries, filter), [entries, filter])

  const columns = useMemo<Column<Entry>[]>(
    () => [
      { key: 'port', header: 'Port', align: 'right', width: '6rem' },
      {
        key: 'protocol',
        header: 'Protocol',
        width: '7rem',
        value: (row) => protocolLabel(row.protocol),
      },
      {
        key: 'address',
        header: 'Address',
        width: '12rem',
        render: (row) => <span className="font-mono text-xs">{row.address}</span>,
      },
      {
        key: 'reach',
        header: 'Reachable',
        width: '11rem',
        value: (row) => reachLabel(row.reach),
        render: (row) => <StatusDot status={reachTone(row.reach)} label={reachLabel(row.reach)} />,
      },
      {
        key: 'process',
        header: 'Program',
        width: '14rem',
        render: (row) =>
          row.process === '' ? (
            <span className="text-fg-muted italic">not visible to you</span>
          ) : (
            row.process
          ),
      },
      {
        key: 'pid',
        header: 'PID',
        align: 'right',
        width: '6rem',
        value: (row) => (row.pid === 0 ? null : row.pid),
        render: (row) => (row.pid === 0 ? '' : String(row.pid)),
      },
      { key: 'service', header: 'Usually' },
    ],
    [],
  )

  return (
    <ToolShell
      title="Listening Ports"
      description="See what is listening on this machine, on which address and port, and which program owns it."
      help={
        <>
          <p>
            This is what <code>netstat -ano</code> on Windows and <code>ss -ltnp</code> on Linux tell
            you, with the program name already looked up. Use it when an installer says "address
            already in use", when a service will not start, or when somebody asks what is serving on
            a port.
          </p>
          <p className="mt-1.5">
            The <strong>Reachable</strong> column is the one to read during a security check.{' '}
            <em>Local only</em> means the service is bound to this computer's loopback address and no
            other machine can reach it, whatever the firewall says. <em>Everywhere</em> means it is
            bound to every address on the machine, so any machine that can route to this one can try
            it. <em>One address</em> means it is bound to a single adapter.
          </p>
          <p className="mt-1.5">
            UDP has no "listening" state, so every bound UDP socket is shown. That is normal and does
            not mean something is wrong.
          </p>
          <p className="mt-1.5">
            On Windows every program name is shown. On Linux and macOS you only see the names of
            programs you started yourself; the rest belong to another user or to the system and are
            labelled rather than left blank. Seeing them all would need administrator rights, which
            CHIT never asks for. Nothing on this page changes anything: no port is closed and no
            program is stopped.
          </p>
        </>
      }
      actions={
        <Button onClick={() => void load()} disabled={loading} icon={<RefreshCw size={14} aria-hidden />}>
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {loading && <Spinner label="Reading listening ports" />}

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
              <p className="text-sm text-fg">{countLine(entries)}</p>
              <p className="mt-1 text-xs text-fg-muted">{report.note}</p>
            </div>

            <div className="flex flex-wrap gap-2">
              {FILTERS.map((option) => (
                <Button
                  key={option.id}
                  size="sm"
                  variant={filter === option.id ? 'primary' : 'secondary'}
                  onClick={() => setFilter(option.id)}
                >
                  {option.label}
                </Button>
              ))}
            </div>

            <ResultsTable
              columns={columns}
              rows={shown}
              getRowId={entryId}
              csvName="listening-ports"
              emptyMessage="Nothing is listening on this machine. That is unusual on a desktop and normal on a locked-down server."
              rowStatus={(row) => (row.reach === 'local' ? undefined : 'warn')}
            />
          </>
        )}
      </div>
    </ToolShell>
  )
}
