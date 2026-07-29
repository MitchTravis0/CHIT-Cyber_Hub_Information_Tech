import { useMemo, useState } from 'react'
import { ScanLine, Square } from 'lucide-react'
import {
  Button,
  ProgressBar,
  ResultsTable,
  Select,
  StatusDot,
  TextInput,
  ToolShell,
  type Column,
  type StatusTone,
} from '../../components'
import { formatDuration } from '../../lib/format'
import type { JobDone } from '../../lib/types'
import { useJob } from '../../lib/useJob'
import { startPortScan, type Result } from './api'
import { PRESETS } from './presets'
import { mergePorts, tally, visibleRows, type Tally } from './results'

const TIMEOUTS = [
  { value: '500', label: 'Fast (0.5 s)' },
  { value: '1000', label: 'Normal (1 s)' },
  { value: '2000', label: 'Patient (2 s)' },
  { value: '4000', label: 'Very patient (4 s)' },
]

const WORKERS = [
  { value: '16', label: '16 (gentle)' },
  { value: '64', label: '64 (normal)' },
  { value: '128', label: '128 (fast)' },
  { value: '256', label: '256 (fastest)' },
]

const STATE_LABELS: Record<string, string> = {
  open: 'Open',
  closed: 'Closed',
  filtered: 'No answer',
}

const STATE_TONES: Record<string, StatusTone> = {
  open: 'ok',
  closed: 'danger',
  filtered: 'warn',
}

const DEFAULT_PORTS = PRESETS[0].spec

export default function PortScannerPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Result>()

  const [host, setHost] = useState('')
  const [portsText, setPortsText] = useState(DEFAULT_PORTS)
  const [timeoutMs, setTimeoutMs] = useState('1000')
  const [workers, setWorkers] = useState('64')
  const [grabBanners, setGrabBanners] = useState(false)
  const [showAll, setShowAll] = useState(false)

  // What the results on screen belong to, so the summary and the Greeting
  // column describe the run that produced them rather than the form as it is
  // now.
  const [scannedHost, setScannedHost] = useState<string | null>(null)
  const [scannedBanners, setScannedBanners] = useState(false)

  const byPort = useMemo(() => mergePorts(results), [results])
  const counts = useMemo(() => tally(byPort), [byPort])
  const rows = useMemo(() => visibleRows(byPort, showAll), [byPort, showAll])

  const columns = useMemo<Column<Result>[]>(() => {
    const list: Column<Result>[] = [
      { key: 'port', header: 'Port', align: 'right', width: '6rem' },
      {
        key: 'state',
        header: 'State',
        width: '8rem',
        value: (row) => STATE_LABELS[row.state] ?? row.state,
        render: (row) => (
          <StatusDot
            status={STATE_TONES[row.state] ?? 'idle'}
            label={STATE_LABELS[row.state] ?? row.state}
          />
        ),
      },
      { key: 'service', header: 'Service' },
      {
        key: 'latencyMs',
        header: 'Time',
        align: 'right',
        width: '6rem',
        value: (row) => (row.state === 'open' ? row.latencyMs : null),
        render: (row) => (row.state === 'open' ? `${row.latencyMs} ms` : ''),
      },
    ]
    if (scannedBanners) {
      list.push({ key: 'banner', header: 'Greeting' })
    }
    return list
  }, [scannedBanners])

  const onSubmit = async () => {
    const text = host.trim()
    if (text === '') return

    setScannedHost(text)
    setScannedBanners(grabBanners)
    await start(() =>
      startPortScan({
        host: text,
        ports: portsText,
        timeoutMs: Number(timeoutMs),
        workers: Number(workers),
        grabBanners,
      }),
    )
  }

  return (
    <ToolShell
      title="Port Scanner"
      description="Check which TCP ports answer on a host, so you know whether to blame the firewall."
      help={
        <>
          <p>
            Enter a host and a list of ports, then press Scan. Ports can be a list (
            <code>80,443</code>), a range (<code>1-1024</code>), or both (<code>80, 8000-8100</code>
            ). The presets fill the box for you: Printers is 515, 631 and 9100, Windows is the file
            sharing and remote management set, and so on.
          </p>
          <p className="mt-1.5">
            A port comes back one of three ways. <strong>Open</strong> means something accepted the
            connection, so the service is running and reachable from here. <strong>Closed</strong>{' '}
            means the host answered and refused, which proves the machine is on and reachable but
            nothing is listening on that port. <strong>No answer</strong> means the check timed out,
            which usually means a firewall is dropping the traffic silently, either on the host or
            somewhere in between.
          </p>
          <p className="mt-1.5">
            All closed and none open is a useful result: the box is up, the service is not. Nothing
            but no-answers usually means the host is off or a firewall is in the way, not that every
            service is down. Remember you are testing from this computer, so a result only tells you
            what this machine can reach.
          </p>
          <p className="mt-1.5">
            Port scanning is normal admin work, but managed antivirus and network monitoring can
            flag it. Tell whoever watches the alerts before scanning a wide range on a customer
            site, and use the gentle speed setting on fragile kit like industrial controllers and
            older printers.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void onSubmit()
          }}
        >
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1">
              <TextInput
                label="Host"
                value={host}
                onChange={(event) => setHost(event.target.value)}
                placeholder="192.168.1.50"
                spellCheck={false}
                autoComplete="off"
                hint="An IP address or a name."
              />
            </div>
            <div className="min-w-56 flex-1">
              <TextInput
                label="Ports"
                value={portsText}
                onChange={(event) => setPortsText(event.target.value)}
                placeholder="80,443"
                spellCheck={false}
                autoComplete="off"
                hint="A list, a range, or both: 80,443 or 1-1024."
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={running}
              icon={<ScanLine size={14} aria-hidden />}
            >
              Scan
            </Button>
            {running && (
              <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
                Cancel
              </Button>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-1">
            <span className="text-xs text-fg-muted">Presets:</span>
            {PRESETS.map((preset) => (
              <Button
                key={preset.id}
                size="sm"
                variant="ghost"
                title={preset.note}
                onClick={() => setPortsText(preset.spec)}
              >
                {preset.name}
              </Button>
            ))}
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Scan options
            </summary>
            <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
              <Select
                label="Wait per port"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
                hint="Longer finds slow devices, shorter finishes sooner."
              />
              <Select
                label="Ports checked at once"
                options={WORKERS}
                value={workers}
                onChange={(event) => setWorkers(event.target.value)}
                hint="Lower this on a busy or fragile network."
              />
              <div className="flex flex-col justify-center gap-1 text-sm">
                <label className="flex items-center gap-2 text-fg">
                  <input
                    type="checkbox"
                    checked={grabBanners}
                    onChange={(event) => setGrabBanners(event.target.checked)}
                    className="size-4 accent-[var(--accent)]"
                  />
                  Read the greeting from open ports
                </label>
                <p className="text-xs text-fg-muted">
                  Only services that speak first (SSH, SMTP, FTP) send one. A web server stays
                  quiet.
                </p>
              </div>
            </div>
          </details>
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

        {scannedHost !== null && (
          <Summary
            host={scannedHost}
            counts={counts}
            expected={progress.total}
            running={running}
            done={done}
          />
        )}

        {scannedHost === null ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Enter a host, pick a preset, and press Scan.
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            <label className="flex items-center gap-2 text-sm text-fg">
              <input
                type="checkbox"
                checked={showAll}
                onChange={(event) => setShowAll(event.target.checked)}
                className="size-4 accent-[var(--accent)]"
              />
              Show closed and filtered ports too
            </label>
            <p className="text-xs text-fg-muted">The CSV contains whatever the table shows.</p>
            <ResultsTable
              columns={columns}
              rows={rows}
              getRowId={(row) => String(row.port)}
              csvName={`port-scan-${scannedHost.replace(/[.:/]/g, '-')}`}
              rowStatus={(row) =>
                row.state === 'open' ? 'ok' : row.state === 'filtered' ? 'warn' : undefined
              }
              emptyMessage={
                running ? 'Scanning, results appear as ports answer.' : 'No open ports in that list.'
              }
            />
          </div>
        )}
      </div>
    </ToolShell>
  )
}

interface SummaryProps {
  host: string
  counts: Tally
  /** The port count the backend is working through, known once it starts. */
  expected: number
  running: boolean
  done: JobDone | null
}

function Summary({ host, counts, expected, running, done }: SummaryProps) {
  const ip = typeof done?.summary.ip === 'string' ? done.summary.ip : ''
  // The backend explains here when a result needs interpreting, for example
  // when everything went unanswered because a firewall is in the way.
  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''

  const total = running && expected > 0 ? expected : counts.total
  const states = `${counts.open} open, ${counts.closed} closed, ${counts.filtered} no answer`
  const headline = running
    ? `Checking ${total} ports on ${host}: ${states} so far`
    : `Checked ${total} ports on ${host}${ip === '' ? '' : ` (${ip})`}: ${states}`

  return (
    <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
      <p className="text-sm font-medium text-fg">
        {headline}
        {!running && done !== null && (
          <span className="font-normal text-fg-muted"> in {formatDuration(done.durationMs)}</span>
        )}
        {!running && done?.cancelled === true && (
          <span className="font-normal text-fg-muted"> (stopped early)</span>
        )}
      </p>

      {note !== '' && <p className="text-xs text-warn">{note}</p>}
    </div>
  )
}
