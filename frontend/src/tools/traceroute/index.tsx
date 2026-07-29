import { useEffect, useMemo, useState } from 'react'
import { List, Route, Square } from 'lucide-react'
import {
  Button,
  ProgressBar,
  ResultsTable,
  Select,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { formatDuration } from '../../lib/format'
import type { JobDone } from '../../lib/types'
import { useJob } from '../../lib/useJob'
import { platform, startTrace, type Hop } from './api'
import { HopList } from './HopList'
import { biggestJump, hopLabel, hopTone, mergeHops } from './hops'

const MAX_HOPS = [
  { value: '15', label: '15' },
  { value: '30', label: '30' },
  { value: '64', label: '64' },
]

const TIMEOUTS = [
  { value: '1000', label: '1 second' },
  { value: '2000', label: '2 seconds' },
  { value: '4000', label: '4 seconds' },
]

const QUERIES = [
  { value: '1', label: '1' },
  { value: '3', label: '3' },
  { value: '5', label: '5' },
]

export default function TraceroutePage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Hop>()

  const [host, setHost] = useState('')
  const [traced, setTraced] = useState<string | null>(null)
  const [maxHops, setMaxHops] = useState('30')
  const [timeoutMs, setTimeoutMs] = useState('2000')
  const [queries, setQueries] = useState('3')
  const [resolveNames, setResolveNames] = useState(true)
  const [view, setView] = useState<'path' | 'table'>('path')
  const [onWindows, setOnWindows] = useState(false)

  useEffect(() => {
    void platform().then((name) => {
      if (name !== null) setOnWindows(name.startsWith('windows'))
    })
  }, [])

  const hops = useMemo(() => mergeHops(results), [results])
  const jump = useMemo(() => biggestJump(hops), [hops])

  const columns = useMemo<Column<Hop>[]>(
    () => [
      { key: 'number', header: 'Hop', align: 'right', width: '4rem' },
      { key: 'ip', header: 'IP', width: '11rem' },
      { key: 'hostname', header: 'Name' },
      {
        key: 'bestMs',
        header: 'Best',
        align: 'right',
        width: '6rem',
        value: (hop) => (hop.timesMs.length === 0 ? null : hop.bestMs),
        render: (hop) => (hop.timesMs.length === 0 ? '' : `${hop.bestMs} ms`),
      },
      {
        key: 'avgMs',
        header: 'Average',
        align: 'right',
        width: '6rem',
        value: (hop) => (hop.timesMs.length === 0 ? null : hop.avgMs),
        render: (hop) => (hop.timesMs.length === 0 ? '' : `${hop.avgMs} ms`),
      },
      {
        key: 'worstMs',
        header: 'Worst',
        align: 'right',
        width: '6rem',
        value: (hop) => (hop.timesMs.length === 0 ? null : hop.worstMs),
        render: (hop) => (hop.timesMs.length === 0 ? '' : `${hop.worstMs} ms`),
      },
      { key: 'lost', header: 'Lost', align: 'right', width: '5rem' },
      { key: 'note', header: 'Note', value: (hop) => hopLabel(hop) },
    ],
    [],
  )

  const runTrace = async () => {
    const text = host.trim()
    if (text === '') return
    setTraced(text)
    await start(() =>
      startTrace({
        host: text,
        maxHops: Number(maxHops),
        queries: Number(queries),
        timeoutMs: Number(timeoutMs),
        noNames: !resolveNames,
      }),
    )
  }

  return (
    <ToolShell
      title="Traceroute"
      description="Follow the path to a host hop by hop and see where it slows down or stops."
      help={
        <>
          <p>
            Type a host and press Trace. Each row is one router between this computer and the
            destination, in order, with how long it took to answer. The bar makes the slow hop
            obvious at a glance, and the hop where the time jumps the most is flagged for you.
          </p>
          <p className="mt-1.5">
            Rows saying "No reply" are normal, especially in the middle of a path. Many routers are
            configured not to answer traceroute while still forwarding traffic perfectly, so a gap
            in the middle followed by working hops afterwards means nothing is wrong. What matters
            is where the path stops for good, and where the time takes a big step up.
          </p>
          <p className="mt-1.5">
            The first hop is your gateway and should be a millisecond or two. The next hop is
            usually your internet connection: if that one is already slow, the fault is your circuit
            and not the far end. If the trace never reaches the destination but the website works
            anyway, the destination is probably just ignoring traceroute.
          </p>
          <p className="mt-1.5">
            CHIT uses the traceroute command that comes with your operating system, so the numbers
            match what you would get from a command prompt. It needs no administrator rights. On a
            Linux machine where the command is not installed, CHIT tells you the one line to run to
            install it.
          </p>
        </>
      }
      actions={
        <div className="flex items-center gap-1 rounded border border-border bg-surface-2 p-0.5">
          <Button
            size="sm"
            variant={view === 'path' ? 'primary' : 'ghost'}
            onClick={() => setView('path')}
            icon={<Route size={14} aria-hidden />}
            aria-pressed={view === 'path'}
          >
            Path
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
            if (!running) void runTrace()
          }}
        >
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1">
              <TextInput
                label="Host to trace"
                value={host}
                onChange={(event) => setHost(event.target.value)}
                placeholder="8.8.8.8"
                spellCheck={false}
                autoComplete="off"
                hint="An IP address or a name."
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={running}
              icon={<Route size={14} aria-hidden />}
            >
              Trace
            </Button>
            {running && (
              <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
                Cancel
              </Button>
            )}
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Trace options
            </summary>
            <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
              <Select
                label="Maximum hops"
                options={MAX_HOPS}
                value={maxHops}
                onChange={(event) => setMaxHops(event.target.value)}
                hint="Stop after this many routers even if the destination has not answered."
              />
              <Select
                label="Wait per probe"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
                hint="Longer waits find slow routers, shorter ones finish sooner."
              />
              {onWindows ? (
                <p className="self-center text-xs text-fg-muted">
                  Windows always sends three probes per hop.
                </p>
              ) : (
                <Select
                  label="Probes per hop"
                  options={QUERIES}
                  value={queries}
                  onChange={(event) => setQueries(event.target.value)}
                  hint="More probes give a fairer average, fewer finish sooner."
                />
              )}
              <label className="flex flex-col justify-center gap-1 text-sm text-fg">
                <span className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={resolveNames}
                    onChange={(event) => setResolveNames(event.target.checked)}
                    className="size-4 accent-[var(--accent)]"
                  />
                  Look up router names
                </span>
                <span className="text-xs text-fg-muted">
                  Turning this off makes a trace noticeably faster on a slow DNS server.
                </span>
              </label>
            </div>
          </details>
        </form>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Starting the trace' : progress.message}
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

        {traced !== null && (
          <Summary host={traced} count={hops.length} running={running} done={done} />
        )}

        {traced === null ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Enter a host and press Trace.
          </p>
        ) : view === 'path' ? (
          <HopList hops={hops} jump={jump} />
        ) : (
          <ResultsTable
            columns={columns}
            rows={hops}
            getRowId={(hop) => String(hop.number)}
            csvName={`traceroute-${host.trim().replace(/[.:/]/g, '-')}`}
            rowStatus={hopTone}
            emptyMessage={
              running ? 'Tracing, hops appear as routers answer.' : 'No hops were returned.'
            }
          />
        )}
      </div>
    </ToolShell>
  )
}

interface SummaryProps {
  host: string
  count: number
  running: boolean
  done: JobDone | null
}

function Summary({ host, count, running, done }: SummaryProps) {
  // The backend explains here when the path stopped short or when nothing on
  // it answered at all, both of which look like a broken tool otherwise.
  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''
  const tool = typeof done?.summary.tool === 'string' ? done.summary.tool : ''
  const ip = typeof done?.summary.ip === 'string' ? done.summary.ip : ''
  const reached = done?.summary.reached === true

  const headline = running
    ? `Following the path to ${host}: ${count} hops so far`
    : `Path to ${host}${ip === '' ? '' : ` (${ip})`}: ${count} hops, ${
        reached ? 'destination reached' : 'destination did not answer'
      }`

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

      {tool !== '' && <p className="text-xs text-fg-muted">Measured with {tool}.</p>}
    </div>
  )
}
