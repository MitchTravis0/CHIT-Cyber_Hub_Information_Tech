import type { FormEvent } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { CircleCheck, GitCompare, Plus, ShieldAlert, TriangleAlert } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  Spinner,
  StatusDot,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { cn, errorMessage } from '../../lib/format'
import {
  compareDns,
  dnsCompareServers,
  type DnsAnswer,
  type DnsComparison,
  type ServerOption,
} from './api'
import {
  agreementLabel,
  agreementTone,
  comparisonText,
  csvNameFor,
  defaultTicks,
  MAX_SERVERS,
} from './compare'

const HELP = (
  <>
    <p>
      This asks several DNS servers the same question at the same moment and shows you which ones
      disagree. It is the tool for "has the DNS change gone live yet" and for "is our domain
      controller handing out a stale answer", both of which are invisible if you only ask one
      server.
    </p>
    <p className="mt-2">
      Type a name, pick a record type, and tick the servers to ask. The system resolver is whatever
      this computer is set to use. Your own servers are filled in from this machine's adapters, and
      you can add any other address, which is how you check a domain controller. A resolver marked{' '}
      <strong>out of step</strong> returned something different from what most of the others
      returned.
    </p>
    <p className="mt-2">
      "No record" is an answer, not a failure: if four servers give an address and one says there is
      no record, that one is out of step and is probably still holding an old, empty cache. "No
      answer" means the server could not be reached at all, usually because port 53 to it is
      blocked.
    </p>
    <p className="mt-2">
      Time to live is not shown. Go's DNS client does not report it and CHIT will not add a DNS
      library just for that, so this tool tells you <strong>who disagrees</strong>, not how long the
      old answer will linger. If you need the TTL, <code>nslookup -debug</code> on the machine will
      give it.
    </p>
  </>
)

const TYPES = [
  { value: 'A', label: 'A (address)' },
  { value: 'AAAA', label: 'AAAA (IPv6 address)' },
  { value: 'CNAME', label: 'CNAME (alias)' },
  { value: 'MX', label: 'MX (mail servers)' },
  { value: 'TXT', label: 'TXT (text)' },
  { value: 'NS', label: 'NS (name servers)' },
]

const TIMEOUTS = [
  { value: '1000', label: '1 second' },
  { value: '3000', label: '3 seconds' },
  { value: '10000', label: '10 seconds' },
]

function bannerTone(level: string): { box: string; icon: string; Icon: typeof CircleCheck } {
  if (level === 'ok') return { box: 'border-ok bg-ok/10', icon: 'text-ok', Icon: CircleCheck }
  if (level === 'warn')
    return { box: 'border-warn bg-warn/10', icon: 'text-warn', Icon: TriangleAlert }
  return { box: 'border-danger bg-danger/10', icon: 'text-danger', Icon: ShieldAlert }
}

function localTime(stamp: string): string {
  if (stamp === '') return ''
  const when = new Date(stamp)
  return Number.isNaN(when.getTime()) ? stamp : when.toLocaleTimeString()
}

export default function DnsComparePage() {
  const [name, setName] = useState('')
  const [nameError, setNameError] = useState<string | null>(null)
  const [type, setType] = useState('A')
  const [timeoutMs, setTimeoutMs] = useState('3000')
  const [options, setOptions] = useState<ServerOption[]>([])
  const [ticked, setTicked] = useState<string[]>([])
  const [extra, setExtra] = useState('')
  const [extraError, setExtraError] = useState<string | null>(null)
  const [result, setResult] = useState<DnsComparison | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  useEffect(() => {
    let cancelled = false
    dnsCompareServers()
      .then((list) => {
        if (cancelled) return
        setOptions(list)
        setTicked(defaultTicks(list))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  const columns = useMemo<Column<DnsAnswer>[]>(
    () => [
      { key: 'label', header: 'Resolver', width: '12rem' },
      {
        key: 'inStep',
        header: 'Agreement',
        width: '10rem',
        value: (row) => agreementLabel(row),
        render: (row) => <StatusDot status={agreementTone(row)} label={agreementLabel(row)} />,
      },
      {
        key: 'values',
        header: 'Answer',
        value: (row) => row.values.join(', '),
        render: (row) =>
          row.values.length > 0 ? (
            row.values.join(', ')
          ) : (
            <span className="text-fg-muted">{row.status === 'error' ? '' : 'no record'}</span>
          ),
      },
      {
        key: 'queryMs',
        header: 'Answered in',
        align: 'right',
        width: '9rem',
        value: (row) => (row.status === 'error' ? null : row.queryMs),
        render: (row) => (row.status === 'error' ? '' : `${Math.round(row.queryMs)} ms`),
      },
    ],
    [],
  )

  const toggle = (id: string) => {
    setTicked((current) =>
      current.includes(id) ? current.filter((x) => x !== id) : [...current, id],
    )
  }

  const addResolver = () => {
    const text = extra.trim()
    if (text === '') return
    if (options.some((o) => o.id === text)) {
      setExtraError(null)
      setExtra('')
      if (!ticked.includes(text)) toggle(text)
      return
    }
    if (!/^(\d{1,3}\.){3}\d{1,3}(:\d+)?$|^\[[0-9a-fA-F:]+\](:\d+)?$|^[0-9a-fA-F:]+$/.test(text)) {
      setExtraError(
        `Enter the DNS server as an IP address, for example 8.8.8.8 or 192.168.1.10. "${text}" is not one.`,
      )
      return
    }
    setExtraError(null)
    setOptions((current) => [...current, { id: text, label: text, detail: 'Added by you' }])
    setTicked((current) => (current.includes(text) ? current : [...current, text]))
    setExtra('')
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (running) return
    const text = name.trim()
    if (text === '') {
      setNameError('Type a name to look up, for example example.com.')
      return
    }
    setNameError(null)
    setRunning(true)
    try {
      setResult(
        await compareDns({ name: text, type, servers: ticked, timeoutMs: Number(timeoutMs) }),
      )
      setError(null)
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    } finally {
      setRunning(false)
    }
  }

  const tone = bannerTone(result?.level ?? 'ok')
  const problems = (result?.answers ?? []).filter((row) => row.message !== '')

  return (
    <ToolShell
      title="DNS Resolver Comparer"
      description="Ask several DNS servers the same question at once and see who disagrees."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={onSubmit}>
          <div className="min-w-56 flex-1">
            <TextInput
              label="Name to look up"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="example.com"
              spellCheck={false}
              autoComplete="off"
              error={nameError ?? undefined}
              hint="The name whose answer you want to compare."
            />
          </div>
          <div className="w-48">
            <Select
              label="Record type"
              options={TYPES}
              value={type}
              onChange={(event) => setType(event.target.value)}
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={running || name.trim() === ''}
            icon={<GitCompare size={14} aria-hidden />}
          >
            Compare
          </Button>
        </form>

        <div className="rounded border border-border bg-surface-2 p-3">
          <p className="text-sm font-medium text-fg">Resolvers to ask</p>
          <div className="mt-2 flex flex-col gap-1.5">
            {options.map((option) => (
              <label key={option.id} className="flex items-center gap-2 text-sm text-fg">
                <input
                  type="checkbox"
                  checked={ticked.includes(option.id)}
                  onChange={() => toggle(option.id)}
                  className="size-4 accent-[var(--accent)]"
                />
                <span>{option.label}</span>
                <span className="text-xs text-fg-muted">{option.detail}</span>
              </label>
            ))}
          </div>
          <div className="mt-3 flex flex-wrap items-end gap-2">
            <div className="min-w-48 flex-1">
              <TextInput
                label="Another resolver"
                value={extra}
                onChange={(event) => setExtra(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    addResolver()
                  }
                }}
                placeholder="192.168.1.10"
                spellCheck={false}
                autoComplete="off"
                error={extraError ?? undefined}
                hint="Your domain controller, for example. Press Add."
              />
            </div>
            <Button onClick={addResolver} icon={<Plus size={14} aria-hidden />}>
              Add
            </Button>
          </div>
          <p className="mt-2 text-xs text-fg-muted">
            {ticked.length} of at most {MAX_SERVERS} ticked.
          </p>
        </div>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Compare options</summary>
          <div className="mt-3 max-w-56">
            <Select
              label="Wait for an answer"
              options={TIMEOUTS}
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </div>
        </details>

        {running && <Spinner label="Asking the resolvers" />}

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {result !== null && (
          <>
            <div className={cn('flex items-start gap-2 rounded border px-3 py-2', tone.box)}>
              <tone.Icon size={16} className={cn('mt-0.5 shrink-0', tone.icon)} aria-hidden />
              <div className="min-w-0 flex-1">
                <p className="text-sm text-fg">{result.headline}</p>
                {result.advice !== '' && (
                  <p className="mt-1 text-sm text-fg-muted">{result.advice}</p>
                )}
                {result.fastestLabel !== '' && (
                  <p className="mt-1 text-xs text-fg-muted">
                    Fastest to answer: {result.fastestLabel} ({Math.round(result.fastestMs)} ms)
                  </p>
                )}
              </div>
              <CopyButton value={comparisonText(result.answers)} />
            </div>

            <ResultsTable
              columns={columns}
              rows={result.answers}
              getRowId={(row) => row.label}
              csvName={csvNameFor(result.name, result.type)}
              rowStatus={(row) => (row.status !== 'error' && !row.inStep ? 'warn' : undefined)}
              emptyMessage="No resolvers were asked."
            />

            {problems.length > 0 && (
              <ul className="flex list-disc flex-col gap-1 pl-5 text-sm text-fg-muted">
                {problems.map((row) => (
                  <li key={row.label}>{row.message}</li>
                ))}
              </ul>
            )}

            <p className="text-sm text-fg-muted">
              {result.majorityCount} of {result.answered} resolvers gave the same answer, checked at{' '}
              {localTime(result.checkedAt)}.
            </p>
          </>
        )}

        {result === null && error === null && !running && (
          <p className="text-sm text-fg-muted">
            Type a name and press Compare to see whether every DNS server agrees.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
