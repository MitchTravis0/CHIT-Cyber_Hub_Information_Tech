import { useEffect, useMemo, useState } from 'react'
import { Play, Square, Wifi } from 'lucide-react'
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
import { cn, formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { myGateway, startPingMonitor, type Sample } from './api'
import { SERIES } from './chart'
import { LatencyChart } from './LatencyChart'
import { dropRows, formatClock, targetStats, type TargetStats } from './stats'

const INTERVALS = [
  { value: '500', label: 'Half a second' },
  { value: '1000', label: '1 second' },
  { value: '2000', label: '2 seconds' },
  { value: '5000', label: '5 seconds' },
]

const TIMEOUTS = [
  { value: '1000', label: '1 second' },
  { value: '2000', label: '2 seconds' },
  { value: '4000', label: '4 seconds' },
]

const ROUNDS = [
  { value: '0', label: 'I press Stop' },
  { value: '60', label: '60 pings' },
  { value: '300', label: '300 pings' },
  { value: '3600', label: '3600 pings' },
]

const VIA_LABELS: Record<string, string> = {
  icmp: 'Ping',
  tcp: 'TCP connect',
}

/** How many rows the table holds. A twelve hour run emits far more than that. */
const TABLE_LIMIT = 1000

function splitHosts(text: string): string[] {
  const parts = text
    .split(/[\s,]+/)
    .map((part) => part.trim())
    .filter((part) => part !== '')

  // The backend de-duplicates case-insensitively too, so the cards line up with
  // the samples that come back.
  const unique = new Map<string, string>()
  for (const part of parts) {
    if (!unique.has(part.toLowerCase())) unique.set(part.toLowerCase(), part)
  }
  return Array.from(unique.values())
}

export default function PingMonitorPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Sample>()

  const [hosts, setHosts] = useState('')
  const [intervalMs, setIntervalMs] = useState('1000')
  const [timeoutMs, setTimeoutMs] = useState('1000')
  const [rounds, setRounds] = useState('0')
  const [tcpFallback, setTcpFallback] = useState(true)
  const [tcpPort, setTcpPort] = useState('443')

  const [gateway, setGateway] = useState<string | null>(null)
  const [targets, setTargets] = useState<string[]>([])
  const [showEveryPing, setShowEveryPing] = useState(false)

  useEffect(() => {
    void myGateway().then((found) => {
      if (found === null) return
      setGateway(found)
      setHosts((current) => (current === '' ? found : current))
    })
  }, [])

  const stats = useMemo(() => targetStats(results), [results])
  const byTarget = useMemo(() => {
    const map = new Map<string, TargetStats>()
    for (const entry of stats) map.set(entry.target, entry)
    return map
  }, [stats])

  const totals = useMemo(() => {
    let sent = 0
    let lost = 0
    for (const entry of stats) {
      sent += entry.sent
      lost += entry.lost
    }
    return { sent, lost, lossPct: sent === 0 ? 0 : Math.round((lost / sent) * 1000) / 10 }
  }, [stats])

  const round = results.length === 0 ? 0 : results[results.length - 1].round

  const rows = useMemo(
    () =>
      showEveryPing
        ? results.slice(Math.max(0, results.length - TABLE_LIMIT))
        : dropRows(results, TABLE_LIMIT),
    [results, showEveryPing],
  )

  const columns = useMemo<Column<Sample>[]>(
    () => [
      { key: 'at', header: 'Time', width: '7rem', value: (row) => formatClock(row.at) },
      { key: 'target', header: 'Host' },
      { key: 'ip', header: 'IP', width: '10rem' },
      {
        key: 'ok',
        header: 'Result',
        width: '8rem',
        value: (row) => (row.ok ? 'Reply' : 'No reply'),
        render: (row) => (
          <StatusDot status={row.ok ? 'ok' : 'danger'} label={row.ok ? 'Reply' : 'No reply'} />
        ),
      },
      {
        key: 'latencyMs',
        header: 'Time taken',
        align: 'right',
        width: '7rem',
        value: (row) => (row.ok ? row.latencyMs : null),
        render: (row) => (row.ok ? `${row.latencyMs} ms` : ''),
      },
      {
        key: 'via',
        header: 'Measured by',
        width: '8rem',
        value: (row) => VIA_LABELS[row.via] ?? '',
      },
      { key: 'reason', header: 'Why not' },
    ],
    [],
  )

  const onSubmit = async () => {
    const list = splitHosts(hosts)
    if (list.length === 0) return
    setTargets(list)
    await start(() =>
      startPingMonitor({
        targets: list,
        intervalMs: Number(intervalMs),
        timeoutMs: Number(timeoutMs),
        rounds: Number(rounds),
        tcpPort: Number(tcpPort),
        skipTcp: !tcpFallback,
      }),
    )
  }

  const started = targets.length > 0
  const canAddGateway = gateway !== null && !splitHosts(hosts).includes(gateway)
  // The backend explains here when it had to measure with TCP because this
  // computer is not allowed to send ping packets.
  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''

  return (
    <ToolShell
      title="Ping Monitor"
      description="Ping one or more hosts continuously with a live latency graph and a packet-loss count."
      help={
        <>
          <p>
            Enter up to four hosts separated by commas and press Start. A good pair is your gateway
            and something on the internet: if both start dropping at the same moment the fault is
            inside the building, and if only the internet one drops it is upstream of your router.
          </p>
          <p className="mt-1.5">
            The graph shows one line per host over the last 300 pings, and every lost ping puts a
            red mark in the strip along the bottom, so a gap is visible instead of being smoothed
            over. The cards above give you the numbers to put in a ticket: how many were sent, how
            many came back, the loss percentage, the average and worst response time, jitter (how
            much the time jumps around), and the longest run of pings that went missing in a row.
          </p>
          <p className="mt-1.5">
            Plenty of devices and most Windows firewalls are set to ignore ping, so a host that
            never answers is not necessarily down. If this computer is not allowed to send ping
            packets at all, CHIT times a TCP connection instead and says so under the summary.
            Those numbers are slightly higher than a real ping because a connection is more work
            than an echo.
          </p>
          <p className="mt-1.5">
            Leave it running while you wiggle the cable. It stops on its own after twelve hours.
            Nothing is saved when you close the tool, so export the CSV if you need it for the
            ticket.
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
            <div className="min-w-64 flex-1">
              <TextInput
                label="Hosts to watch"
                value={hosts}
                onChange={(event) => setHosts(event.target.value)}
                placeholder="192.168.1.1, 8.8.8.8"
                spellCheck={false}
                autoComplete="off"
                hint="Up to four, separated by commas."
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={running}
              icon={<Play size={14} aria-hidden />}
            >
              Start
            </Button>
            {running && (
              <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
                Stop
              </Button>
            )}
            {canAddGateway && (
              <Button
                onClick={() => setHosts((current) => (current === '' ? gateway : `${current}, ${gateway}`))}
                icon={<Wifi size={14} aria-hidden />}
              >
                Add my gateway ({gateway})
              </Button>
            )}
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Monitor options
            </summary>
            <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
              <Select
                label="Ping every"
                options={INTERVALS}
                value={intervalMs}
                onChange={(event) => setIntervalMs(event.target.value)}
                hint="Faster spots a short glitch, slower is kinder to the network."
              />
              <Select
                label="Wait for a reply"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
              />
              <Select
                label="Stop after"
                options={ROUNDS}
                value={rounds}
                onChange={(event) => setRounds(event.target.value)}
                hint="A run stops on its own after twelve hours either way."
              />
              <div className="flex flex-col justify-center gap-2 text-sm">
                <label className="flex items-center gap-2 text-fg">
                  <input
                    type="checkbox"
                    checked={tcpFallback}
                    onChange={(event) => setTcpFallback(event.target.checked)}
                    className="size-4 accent-[var(--accent)]"
                  />
                  Measure with a TCP connection when ping is blocked
                </label>
                <div className="w-28">
                  <TextInput
                    label="TCP port"
                    value={tcpPort}
                    onChange={(event) => setTcpPort(event.target.value)}
                    spellCheck={false}
                    autoComplete="off"
                  />
                </div>
              </div>
            </div>
          </details>
        </form>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Starting' : progress.message}
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

        {!started ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Enter one or more hosts and press Start.
          </p>
        ) : (
          <>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {targets.map((target, index) => {
                const entry = byTarget.get(target)
                if (entry === undefined) return null
                return <StatCard key={target} stats={entry} colour={SERIES[index]} />
              })}
            </div>

            <LatencyChart samples={results} targets={targets} />

            <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
              <p className="text-sm font-medium text-fg">
                {running
                  ? `Watching ${targets.length} hosts, round ${round}: ${totals.lossPct}% of pings lost so far`
                  : `Watched ${targets.length} hosts for ${round} rounds: ${totals.lossPct}% of pings lost`}
                {!running && done !== null && (
                  <span className="font-normal text-fg-muted">
                    {' '}
                    in {formatDuration(done.durationMs)}
                  </span>
                )}
                {!running && done?.cancelled === true && (
                  <span className="font-normal text-fg-muted"> (stopped early)</span>
                )}
              </p>
              {note !== '' && <p className="text-xs text-warn">{note}</p>}
            </div>

            <div className="flex flex-col gap-2">
              <label className="flex items-center gap-2 text-sm text-fg">
                <input
                  type="checkbox"
                  checked={showEveryPing}
                  onChange={(event) => setShowEveryPing(event.target.checked)}
                  className="size-4 accent-[var(--accent)]"
                />
                Show every ping, not just the drops
              </label>
              <p className="text-xs text-fg-muted">
                Only the last 1000 pings are kept in the table. The CSV contains whatever the table
                shows.
              </p>
              <ResultsTable
                columns={columns}
                rows={rows}
                getRowId={(row) => `${row.round}-${row.target}`}
                csvName={`ping-${targets[0] ?? 'run'}`.replace(/[.:/]/g, '-')}
                rowStatus={(row) => (row.ok ? undefined : 'danger')}
                emptyMessage={
                  running ? 'No drops yet. Every ping has answered.' : 'Press Start to begin.'
                }
              />
            </div>
          </>
        )}
      </div>
    </ToolShell>
  )
}

function StatCard({ stats, colour }: { stats: TargetStats; colour: string }) {
  return (
    <div className="rounded border border-border bg-surface-2 px-3 py-2">
      <div className="flex items-center gap-2">
        <span aria-hidden className="size-2.5 rounded-full" style={{ backgroundColor: colour }} />
        <span className="truncate text-sm font-medium text-fg">{stats.target}</span>
        <StatusDot
          status={stats.up ? 'ok' : 'danger'}
          label={stats.up ? 'Up' : 'No reply'}
          className="ml-auto text-xs text-fg-muted"
        />
      </div>
      <p className="text-xs text-fg-muted">{stats.ip === '' ? 'name not found' : stats.ip}</p>
      <p className="text-sm tabular-nums">
        <span className="text-base font-semibold text-fg">
          {stats.lastMs < 0 ? 'no reply' : `${stats.lastMs} ms`}
        </span>{' '}
        <span className="text-fg-muted">
          avg {stats.avgMs} / min {stats.minMs} / max {stats.maxMs}
        </span>
      </p>
      <p className={cn('text-xs', stats.lossPct > 0 ? 'text-danger' : 'text-fg-muted')}>
        {stats.lossPct}% lost ({stats.lost} of {stats.sent})
      </p>
      <p className="text-xs text-fg-muted">
        jitter {stats.jitterMs} ms
        {stats.longestOutage > 1 && `, longest gap: ${stats.longestOutage} in a row`}
      </p>
    </div>
  )
}
