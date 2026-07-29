import { useEffect, useMemo, useState } from 'react'
import { Search, Square } from 'lucide-react'
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
import { useJob } from '../../lib/useJob'
import { dnsServers, startLookup, type DnsRecord, type ServerOption } from './api'
import { answerCount, sortRecords, visibleRecords, TYPE_ORDER } from './records'

const DEFAULT_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'TXT']

const TIMEOUTS = [
  { value: '1000', label: '1 second' },
  { value: '3000', label: '3 seconds' },
  { value: '8000', label: '8 seconds' },
]

const SYSTEM_RESOLVER: ServerOption = {
  id: '',
  label: 'System resolver',
  detail: 'Whatever this computer is set to use',
}

/** Only good enough to warn the user before they press the button: the backend
 *  decides for real. */
function looksLikeAddress(text: string): boolean {
  return /^[0-9.]+$/.test(text) || text.includes(':')
}

export default function DnsLookupPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<DnsRecord>()

  const [name, setName] = useState('')
  const [types, setTypes] = useState<string[]>(DEFAULT_TYPES)
  const [options, setOptions] = useState<ServerOption[]>([SYSTEM_RESOLVER])
  const [picked, setPicked] = useState<string[]>([''])
  const [extraServer, setExtraServer] = useState('')
  const [timeoutMs, setTimeoutMs] = useState('3000')
  const [showEmpty, setShowEmpty] = useState(true)
  const [asked, setAsked] = useState<{ name: string; servers: number; types: number } | null>(null)

  useEffect(() => {
    void dnsServers().then(setOptions)
  }, [])

  const rows = useMemo(
    () => visibleRecords(sortRecords(results), showEmpty),
    [results, showEmpty],
  )
  const answers = useMemo(() => answerCount(results), [results])

  const columns = useMemo<Column<DnsRecord>[]>(
    () => [
      { key: 'server', header: 'Server', width: '12rem' },
      { key: 'type', header: 'Type', width: '5rem' },
      { key: 'name', header: 'Name' },
      {
        key: 'value',
        header: 'Answer',
        value: (row) => (row.status === 'ok' ? row.value : row.message),
        render: (row) => (
          <span className={row.status === 'ok' ? 'text-fg' : 'text-fg-muted'}>
            {row.status === 'ok' ? row.value : row.message}
          </span>
        ),
      },
      {
        key: 'priority',
        header: 'Priority',
        align: 'right',
        width: '6rem',
        value: (row) => (row.priority === 0 ? null : row.priority),
      },
      {
        key: 'queryMs',
        header: 'Time',
        align: 'right',
        width: '6rem',
        render: (row) => `${row.queryMs} ms`,
      },
    ],
    [],
  )

  const toggleType = (type: string) => {
    setTypes((current) =>
      current.includes(type) ? current.filter((item) => item !== type) : current.concat(type),
    )
  }

  const toggleServer = (id: string) => {
    setPicked((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : current.concat(id),
    )
  }

  const onSubmit = async () => {
    const text = name.trim()
    if (text === '' || types.length === 0) return

    const servers = picked.slice()
    const extra = extraServer.trim()
    if (extra !== '' && !servers.includes(extra)) servers.push(extra)
    // Unticking everything still asks something, the same way the backend does.
    if (servers.length === 0) servers.push('')

    setAsked({ name: text, servers: servers.length, types: types.length })
    await start(() => startLookup({ name: text, types, servers, timeoutMs: Number(timeoutMs) }))
  }

  // The backend works out whether the servers agree and explains it here. The
  // page never recomputes it.
  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''

  return (
    <ToolShell
      title="DNS Lookup"
      description="Look up A, AAAA, CNAME, MX, TXT, NS, SRV and PTR records against one or more DNS servers."
      help={
        <>
          <p>
            Type a name and press Look up. Tick more than one server to ask them the same question
            at once, which is how you catch a stale record: if your domain controller and a public
            resolver disagree about an address, a warning appears above the table.
          </p>
          <p className="mt-1.5">
            A stands for the IPv4 address and AAAA for the IPv6 one. CNAME is an alias pointing at
            another name. MX is where mail for the domain goes, lowest priority number first. TXT
            holds the SPF, DKIM and verification lines. NS lists the servers that are authoritative
            for the domain, and SRV is how things like SIP and Active Directory advertise a port.
          </p>
          <p className="mt-1.5">
            Type an IP address instead of a name and CHIT does a reverse lookup, which turns the
            address back into a name if one has been published. Plenty of addresses have no reverse
            record and that is normal, especially inside a company network.
          </p>
          <p className="mt-1.5">
            {'"No MX record for example.com" is a real answer, not a failure. A row in red means ' +
              'the server could not be reached or refused to answer, which is a different problem: ' +
              'check that this computer can reach it on port 53. There is no TTL column because ' +
              'the lookup method built into Go does not report one.'}
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
                label="Name or IP address"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="example.com"
                spellCheck={false}
                autoComplete="off"
                hint="A name for a forward lookup, an IP address for a reverse one."
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={running}
              icon={<Search size={14} aria-hidden />}
            >
              Look up
            </Button>
            {running && (
              <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
                Cancel
              </Button>
            )}
          </div>

          <div className="flex flex-col gap-1">
            <div className="flex flex-wrap items-center gap-1">
              <span className="text-xs text-fg-muted">Record types:</span>
              {TYPE_ORDER.map((type) => (
                <Button
                  key={type}
                  size="sm"
                  variant={types.includes(type) ? 'primary' : 'ghost'}
                  aria-pressed={types.includes(type)}
                  onClick={() => toggleType(type)}
                >
                  {type}
                </Button>
              ))}
            </div>
            {looksLikeAddress(name.trim()) && (
              <p className="text-xs text-fg-muted">
                That looks like an IP address, so CHIT will do a reverse (PTR) lookup instead.
              </p>
            )}
          </div>

          <div className="flex flex-col gap-1 rounded border border-border bg-surface-2 px-3 py-2">
            <span className="text-xs font-medium text-fg-muted">Ask these servers</span>
            {options.map((option) => (
              <label key={option.id} className="flex items-center gap-2 text-sm text-fg">
                <input
                  type="checkbox"
                  checked={picked.includes(option.id)}
                  onChange={() => toggleServer(option.id)}
                  className="size-4 accent-[var(--accent)]"
                />
                {option.label}
                <span className="text-xs text-fg-muted">{option.detail}</span>
              </label>
            ))}
            <div className="max-w-64 pt-1">
              <TextInput
                label="Another server"
                value={extraServer}
                onChange={(event) => setExtraServer(event.target.value)}
                placeholder="192.168.1.10"
                spellCheck={false}
                autoComplete="off"
              />
            </div>
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Lookup options
            </summary>
            <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
              <Select
                label="Wait for an answer"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
                hint="A local server answers in milliseconds. Raise this only when a server is far away."
              />
            </div>
          </details>
        </form>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Asking' : progress.message}
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

        {asked !== null && (
          <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-sm font-medium text-fg">
              {running
                ? `Asking ${asked.servers} servers about ${asked.name}`
                : `Asked ${asked.servers} servers for ${asked.types} record types about ${asked.name}: ${answers} answers`}
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
        )}

        <div className="flex flex-col gap-1">
          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={showEmpty}
              onChange={(event) => setShowEmpty(event.target.checked)}
              className="size-4 accent-[var(--accent)]"
            />
            Show questions that had no answer
          </label>
          <p className="text-xs text-fg-muted">
            {'"No MX record" is an answer too. The CSV contains whatever the table shows.'}
          </p>
        </div>

        {asked === null ? (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Enter a name or an IP address and press Look up.
          </p>
        ) : (
          <ResultsTable
            columns={columns}
            rows={rows}
            getRowId={(row) => `${row.server}|${row.type}|${row.value}|${row.message}`}
            csvName={`dns-${name.trim().replace(/[.:/]/g, '-')}`}
            rowStatus={(row) => (row.status === 'error' ? 'danger' : undefined)}
            emptyMessage={
              running ? 'Asking, answers appear as servers reply.' : 'No answers came back.'
            }
          />
        )}
      </div>
    </ToolShell>
  )
}
