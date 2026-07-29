import { useCallback, useEffect, useMemo, useState } from 'react'
import { LayoutGrid, List, Radar, Square, Wifi } from 'lucide-react'
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
import { cn, formatDuration } from '../../lib/format'
import type { JobDone } from '../../lib/types'
import { useJob } from '../../lib/useJob'
import { scanDefaults, startScan, type Host, type ScanDefaults } from './api'
import { HostGrid, MAX_GRID_ADDRESSES, StateSwatch } from './HostGrid'
import {
  cellStates,
  firstFreeAddress,
  largestFreeBlock,
  mergeHosts,
  tally,
  FREE,
  USED,
  type FreeBlock,
  type Tally,
} from './hosts'
import { parseRange, type IPRange } from './range'

const TIMEOUTS = [
  { value: '500', label: 'Fast (0.5 s per device)' },
  { value: '1000', label: 'Normal (1 s per device)' },
  { value: '2000', label: 'Patient (2 s per device)' },
  { value: '4000', label: 'Very patient (4 s per device)' },
]

const WORKERS = [
  { value: '32', label: '32 at a time (gentle)' },
  { value: '64', label: '64 at a time (normal)' },
  { value: '128', label: '128 at a time (fast)' },
  { value: '256', label: '256 at a time (fastest)' },
]

const VIA_LABELS: Record<string, string> = {
  icmp: 'Ping',
  tcp: 'TCP port',
  arp: 'ARP cache',
}

function parsePorts(text: string): { ok: true; ports: number[] } | { ok: false; error: string } {
  const parts = text
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter((part) => part !== '')

  const ports: number[] = []
  for (const part of parts) {
    const port = Number(part)
    if (!/^\d{1,5}$/.test(part) || port < 1 || port > 65535) {
      return { ok: false, error: `"${part}" is not a port number. Ports run from 1 to 65535.` }
    }
    ports.push(port)
  }
  return { ok: true, ports }
}

export default function IPRangeScannerPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Host>()

  const [rangeText, setRangeText] = useState('')
  const [rangeError, setRangeError] = useState<string | null>(null)
  const [portsText, setPortsText] = useState('')
  const [portsError, setPortsError] = useState<string | null>(null)
  const [timeoutMs, setTimeoutMs] = useState('1000')
  const [workers, setWorkers] = useState('64')
  const [probeTCP, setProbeTCP] = useState(true)
  const [resolveNames, setResolveNames] = useState(true)

  const [defaults, setDefaults] = useState<ScanDefaults | null>(null)
  const [scanned, setScanned] = useState<IPRange | null>(null)
  const [selected, setSelected] = useState<string | null>(null)
  const [view, setView] = useState<'grid' | 'table'>('grid')
  const [inUseOnly, setInUseOnly] = useState(true)

  useEffect(() => {
    void scanDefaults().then((found) => {
      if (found === null) return
      setDefaults(found)
      setRangeText((current) => (current === '' ? found.range : current))
    })
  }, [])

  const runScan = useCallback(
    async (text: string) => {
      const parsed = parseRange(text)
      if (!parsed.ok) {
        setRangeError(parsed.error)
        return
      }
      const ports = parsePorts(portsText)
      if (!ports.ok) {
        setPortsError(ports.error)
        return
      }

      setRangeError(null)
      setPortsError(null)
      setSelected(null)
      setScanned(parsed.range)

      await start(() =>
        startScan({
          range: text.trim(),
          timeoutMs: Number(timeoutMs),
          workers: Number(workers),
          ports: ports.ports,
          skipTcp: !probeTCP,
          skipHostnames: !resolveNames,
        }),
      )
    },
    [portsText, timeoutMs, workers, probeTCP, resolveNames, start],
  )

  const byIP = useMemo(() => mergeHosts(results), [results])
  const states = useMemo(
    () => (scanned === null ? null : cellStates(scanned, byIP)),
    [scanned, byIP],
  )
  const counts = useMemo(
    () => (scanned === null || states === null ? null : tally(scanned, states, byIP)),
    [scanned, states, byIP],
  )
  const freeBlock = useMemo(
    () => (scanned === null || states === null ? null : largestFreeBlock(scanned, states)),
    [scanned, states],
  )
  const firstFree = useMemo(
    () => (scanned === null || states === null ? null : firstFreeAddress(scanned, states)),
    [scanned, states],
  )

  const tableRows = useMemo(() => {
    const all = Array.from(byIP.values())
    return inUseOnly ? all.filter((host) => host.status === 'up') : all
  }, [byIP, inUseOnly])

  const columns = useMemo<Column<Host>[]>(
    () => [
      { key: 'ip', header: 'IP', width: '9rem' },
      {
        key: 'status',
        header: 'Status',
        width: '7rem',
        value: (host) => (host.status === 'up' ? 'In use' : 'Free'),
        render: (host) => (
          <span className="inline-flex items-center gap-1.5">
            <StateSwatch state={host.status === 'up' ? USED : FREE} />
            {host.status === 'up' ? 'In use' : 'Free'}
          </span>
        ),
      },
      { key: 'hostname', header: 'Hostname' },
      { key: 'mac', header: 'MAC', width: '11rem' },
      { key: 'vendor', header: 'Vendor' },
      {
        key: 'latencyMs',
        header: 'Latency',
        align: 'right',
        width: '6rem',
        value: (host) => (host.status === 'up' ? host.latencyMs : null),
        render: (host) => (host.status === 'up' ? `${host.latencyMs} ms` : ''),
      },
      {
        key: 'respondedVia',
        header: 'Responded Via',
        width: '8rem',
        value: (host) => VIA_LABELS[host.respondedVia] ?? '',
      },
      {
        key: 'openPorts',
        header: 'Open Ports',
        value: (host) => (host.openPorts ?? []).join(' '),
      },
    ],
    [],
  )

  const gridTooBig = scanned !== null && scanned.count > MAX_GRID_ADDRESSES
  const showGrid = view === 'grid' && !gridTooBig

  return (
    <ToolShell
      title="IP Range Scanner"
      description="Sweep a range and see at a glance which addresses are in use and which are free."
      help={
        <>
          <p>
            Enter a range as <code>192.168.1.0/24</code>, <code>192.168.1.1-254</code>,{' '}
            <code>192.168.1.1-192.168.1.99</code> or a single address, then press Scan. The grid
            paints one cell per address so the free block you can take a static IP from is obvious.
          </p>
          <p className="mt-1.5">
            Plenty of devices are told to ignore ping, so a silent address is not proof that it is
            free. Each address gets three chances: a ping, then a connection attempt on common
            ports (80, 443, 445, 22, 3389, 8080, 631), then a read of this computer's ARP cache,
            which also supplies MAC addresses and the manufacturer behind them. The ARP cache only
            covers your own network, so scanning through a router gives no MACs.
          </p>
          <p className="mt-1.5">
            Free means nothing answered just now. A device that is switched off still holds its
            DHCP lease, so check the DHCP server before handing an address out permanently, and
            leave the DHCP pool itself alone.
          </p>
        </>
      }
      actions={
        <div className="flex items-center gap-1 rounded border border-border bg-surface-2 p-0.5">
          <Button
            size="sm"
            variant={view === 'grid' ? 'primary' : 'ghost'}
            onClick={() => setView('grid')}
            icon={<LayoutGrid size={14} aria-hidden />}
            aria-pressed={view === 'grid'}
          >
            Grid
          </Button>
          <Button
            size="sm"
            variant={view === 'table' ? 'primary' : 'ghost'}
            onClick={() => setView('table')}
            icon={<List size={14} aria-hidden />}
            aria-pressed={view === 'table'}
          >
            Table
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void runScan(rangeText)
          }}
        >
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1">
              <TextInput
                label="Range to scan"
                value={rangeText}
                onChange={(event) => setRangeText(event.target.value)}
                placeholder="192.168.1.0/24"
                spellCheck={false}
                autoComplete="off"
                error={rangeError ?? undefined}
                hint="CIDR, a start and end, or a single address."
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={running}
              icon={<Radar size={14} aria-hidden />}
            >
              Scan
            </Button>
            {running && (
              <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
                Cancel
              </Button>
            )}
            {defaults !== null && (
              <Button
                onClick={() => {
                  setRangeText(defaults.range)
                  if (!running) void runScan(defaults.range)
                }}
                disabled={running}
                icon={<Wifi size={14} aria-hidden />}
                title={`${defaults.adapter}, this computer is ${defaults.ip}`}
              >
                Scan my subnet ({defaults.range})
              </Button>
            )}
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Scan options
            </summary>
            <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
              <Select
                label="Wait per device"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
                hint="Longer finds slow devices, shorter finishes sooner."
              />
              <Select
                label="Addresses checked at once"
                options={WORKERS}
                value={workers}
                onChange={(event) => setWorkers(event.target.value)}
                hint="Lower this on a busy or fragile network."
              />
              <TextInput
                label="TCP ports to try"
                value={portsText}
                onChange={(event) => setPortsText(event.target.value)}
                placeholder="80, 443, 445, 22, 3389, 8080, 631"
                spellCheck={false}
                autoComplete="off"
                error={portsError ?? undefined}
                hint="Leave empty for the common ports above."
              />
              <div className="flex flex-col justify-center gap-2 text-sm">
                <label className="flex items-center gap-2 text-fg">
                  <input
                    type="checkbox"
                    checked={probeTCP}
                    onChange={(event) => setProbeTCP(event.target.checked)}
                    className="size-4 accent-[var(--accent)]"
                  />
                  Try TCP ports when a device ignores ping
                </label>
                <label className="flex items-center gap-2 text-fg">
                  <input
                    type="checkbox"
                    checked={resolveNames}
                    onChange={(event) => setResolveNames(event.target.checked)}
                    className="size-4 accent-[var(--accent)]"
                  />
                  Look up device names
                </label>
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
          <p className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg">
            {error}
          </p>
        )}

        {scanned !== null && counts !== null && (
          <Summary
            range={scanned}
            counts={counts}
            running={running}
            done={done}
            freeBlock={freeBlock}
            firstFree={firstFree}
          />
        )}

        {scanned === null ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Press Scan to check every address in the range.
          </p>
        ) : showGrid && states !== null ? (
          <div className="flex flex-col gap-3">
            <HostGrid
              range={scanned}
              states={states}
              byIP={byIP}
              selected={selected}
              onSelect={setSelected}
              myIP={defaults?.ip}
              gateway={defaults?.gateway}
            />
            <Detail
              ip={selected}
              host={selected === null ? undefined : byIP.get(selected)}
              defaults={defaults}
            />
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {gridTooBig && view === 'grid' && (
              <p className="text-xs text-fg-muted">
                {scanned.count} addresses is too many to draw as a grid, so here is the table.
                Scan up to {MAX_GRID_ADDRESSES} addresses at a time for the grid view.
              </p>
            )}
            <label className="flex items-center gap-2 text-sm text-fg">
              <input
                type="checkbox"
                checked={!inUseOnly}
                onChange={(event) => setInUseOnly(!event.target.checked)}
                className="size-4 accent-[var(--accent)]"
              />
              Include free addresses in the table and the CSV
            </label>
            <ResultsTable
              columns={columns}
              rows={tableRows}
              getRowId={(host) => host.ip}
              csvName={`ip-scan-${scanned.text.replace(/[./]/g, '-')}`}
              emptyMessage={
                running ? 'Scanning, results appear as devices answer.' : 'Nothing answered in this range.'
              }
            />
          </div>
        )}
      </div>
    </ToolShell>
  )
}

interface SummaryProps {
  range: IPRange
  counts: Tally
  running: boolean
  done: JobDone | null
  freeBlock: FreeBlock | null
  firstFree: string | null
}

function Summary({ range, counts, running, done, freeBlock, firstFree }: SummaryProps) {
  const headline = running
    ? `Checking ${counts.total} addresses in ${range.text}: ${counts.used} in use, ${counts.free} free so far`
    : `Scanned ${counts.total} addresses in ${range.text}: ${counts.used} in use, ${counts.free} free`

  // The backend explains here when it had to fall back, for example because
  // this computer is not allowed to send ping packets.
  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''

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

      <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-fg-muted">
        <span>Answered ping: {counts.viaIcmp}</span>
        <span>Answered a TCP port: {counts.viaTcp}</span>
        <span>Found in the ARP cache only: {counts.viaArp}</span>
        <span>MAC address known: {counts.withMac}</span>
      </div>

      {freeBlock !== null && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-border pt-2 text-sm">
          <span className="text-fg">
            Largest free block:{' '}
            <span className="font-medium text-ok">
              {freeBlock.start} to {freeBlock.end}
            </span>{' '}
            <span className="text-fg-muted">
              ({freeBlock.count} {freeBlock.count === 1 ? 'address' : 'addresses'})
            </span>
          </span>
          {firstFree !== null && (
            <span className="inline-flex items-center gap-1 text-fg-muted">
              First free address: <span className="text-fg">{firstFree}</span>
              <CopyButton value={firstFree} />
            </span>
          )}
        </div>
      )}
    </div>
  )
}

interface DetailProps {
  ip: string | null
  host: Host | undefined
  defaults: ScanDefaults | null
}

function Detail({ ip, host, defaults }: DetailProps) {
  if (ip === null) {
    return (
      <p className="text-xs text-fg-muted">
        Hover a cell for a summary, or click one for the full details.
      </p>
    )
  }

  const inUse = host?.status === 'up'
  const marker =
    ip === defaults?.ip
      ? 'This is the computer you are scanning from.'
      : ip === defaults?.gateway
        ? 'This is the gateway for this network. Leave it alone.'
        : ''

  return (
    <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold text-fg tabular-nums">{ip}</span>
        <CopyButton value={ip} />
        <span
          className={cn(
            'rounded px-1.5 py-0.5 text-xs font-medium',
            inUse ? 'bg-accent text-accent-fg' : 'bg-ok/20 text-fg',
          )}
        >
          {host === undefined ? 'Not checked yet' : inUse ? 'In use' : 'Free'}
        </span>
      </div>

      {marker !== '' && <p className="text-xs text-warn">{marker}</p>}

      {inUse && host !== undefined ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <Field label="Answered" value={VIA_LABELS[host.respondedVia] ?? ''} />
          <Field label="Response time" value={`${host.latencyMs} ms`} />
          <Field label="Hostname" value={host.hostname} />
          <Field label="MAC" value={host.mac} />
          <Field label="Manufacturer" value={host.vendor} />
          <Field label="Open ports" value={(host.openPorts ?? []).join(', ')} />
        </dl>
      ) : (
        host !== undefined && (
          <p className="text-xs text-fg-muted">
            Nothing answered on this address, so it is a candidate for a static IP. Check your DHCP
            server first in case a device that is switched off still holds the lease.
          </p>
        )
      )}
    </div>
  )
}

function Field({ label, value }: { label: string; value: string }) {
  if (value === '') return null
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className="text-fg">{value}</dd>
    </>
  )
}
