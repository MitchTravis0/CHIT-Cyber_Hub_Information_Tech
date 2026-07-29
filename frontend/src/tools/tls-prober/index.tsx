import type { FormEvent } from 'react'
import { useMemo, useState } from 'react'
import { CircleCheck, Handshake, ShieldAlert, TriangleAlert } from 'lucide-react'
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
import { probeTls, type TlsAttempt, type TlsReport } from './api'
import { csvNameFor, reportText, resultLabel, resultTone } from './format'

const HELP = (
  <>
    <p>
      This tries a TLS connection to the server once for each protocol version and reports which
      ones it will actually accept. It answers the question behind most "it worked last week" faults
      on old equipment: a server was hardened, the old device can only speak TLS 1.0, and nothing on
      the device says so.
    </p>
    <p className="mt-2">
      Type a name or an IP address. Port 443 is assumed, so add <code>:8443</code> or{' '}
      <code>:993</code> if the service is somewhere else. This tool speaks plain TLS only, so ports
      that need a conversation first (STARTTLS on 25, 587, 110 and 143) will not answer here.
    </p>
    <p className="mt-2">
      "Accepted" means the handshake completed at exactly that version, and the cipher column shows
      what the two sides settled on. "Refused" means the server would not do that version. If every
      row is refused, the problem is usually not TLS at all: nothing answered on that port, so check
      the address and the firewall first.
    </p>
    <p className="mt-2">
      The certificate is deliberately ignored here, so a self-signed or expired certificate still
      gets you an honest answer about protocol versions. Use the Certificate Decoder or the Website
      / Service Up Checker to look at the certificate itself. SSL 3.0 is listed but cannot be
      tested: Go's TLS library does not implement it, and CHIT will not guess an answer it did not
      measure.
    </p>
  </>
)

const TIMEOUTS = [
  { value: '2000', label: '2 seconds' },
  { value: '5000', label: '5 seconds' },
  { value: '15000', label: '15 seconds' },
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

export default function TlsProberPage() {
  const [target, setTarget] = useState('')
  const [targetError, setTargetError] = useState<string | null>(null)
  const [timeoutMs, setTimeoutMs] = useState('5000')
  const [report, setReport] = useState<TlsReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  const columns = useMemo<Column<TlsAttempt>[]>(
    () => [
      { key: 'version', header: 'Version', width: '8rem' },
      {
        key: 'accepted',
        header: 'Result',
        width: '9rem',
        value: (row) => resultLabel(row),
        render: (row) => <StatusDot status={resultTone(row)} label={resultLabel(row)} />,
      },
      { key: 'cipher', header: 'Cipher' },
      { key: 'alpn', header: 'App protocol', width: '8rem' },
      {
        key: 'handshakeMs',
        header: 'Handshake',
        align: 'right',
        width: '8rem',
        value: (row) => (row.accepted ? row.handshakeMs : null),
        render: (row) => (row.accepted ? `${Math.round(row.handshakeMs)} ms` : ''),
      },
    ],
    [],
  )

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (running) return
    const text = target.trim()
    if (text === '') {
      setTargetError('Type the server to probe, for example mail.example.com.')
      return
    }
    setTargetError(null)
    setRunning(true)
    try {
      setReport(await probeTls({ target: text, timeoutMs: Number(timeoutMs) }))
      setError(null)
    } catch (err) {
      setReport(null)
      setError(errorMessage(err))
    } finally {
      setRunning(false)
    }
  }

  const tone = bannerTone(report?.level ?? 'ok')

  return (
    <ToolShell
      title="TLS Handshake Prober"
      description="Find out which TLS versions a server will actually accept, and which it refuses."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={onSubmit}>
          <div className="min-w-56 flex-1">
            <TextInput
              label="Server to probe"
              value={target}
              onChange={(event) => setTarget(event.target.value)}
              placeholder="mail.example.com:443"
              spellCheck={false}
              autoComplete="off"
              error={targetError ?? undefined}
              hint="A name or IP address. Add :port if it is not 443."
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={running || target.trim() === ''}
            icon={<Handshake size={14} aria-hidden />}
          >
            Probe
          </Button>
        </form>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Probe options</summary>
          <div className="mt-3 max-w-56">
            <Select
              label="Wait per handshake"
              options={TIMEOUTS}
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </div>
        </details>

        {running && <Spinner label="Trying each TLS version" />}

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
            <div className={cn('flex items-start gap-2 rounded border px-3 py-2', tone.box)}>
              <tone.Icon size={16} className={cn('mt-0.5 shrink-0', tone.icon)} aria-hidden />
              <div className="min-w-0 flex-1">
                <p className="text-sm text-fg">{report.headline}</p>
                {report.advice !== '' && (
                  <p className="mt-1 text-sm text-fg-muted">{report.advice}</p>
                )}
                <p className="mt-1 text-xs text-fg-muted">
                  {report.host}:{report.port}
                  {report.ip !== '' && ` resolved to ${report.ip}`}
                </p>
              </div>
              <CopyButton value={reportText(report.attempts)} />
            </div>

            <ResultsTable
              columns={columns}
              rows={report.attempts}
              getRowId={(row) => row.version}
              csvName={csvNameFor(report.host, report.port)}
              rowStatus={(row) => {
                if (!row.testable) return undefined
                return row.accepted ? 'ok' : 'danger'
              }}
              emptyMessage="Nothing was probed."
            />

            <ul className="flex list-disc flex-col gap-1 pl-5 text-sm text-fg-muted">
              {report.attempts.map((row) => (
                <li key={row.version}>
                  <span className="text-fg">{row.version}:</span> {row.message}
                </li>
              ))}
            </ul>

            <p className="text-sm text-fg-muted">Probed at {localTime(report.checkedAt)}.</p>
          </>
        )}

        {report === null && error === null && !running && (
          <p className="text-sm text-fg-muted">
            Type a server and press Probe to see which TLS versions it accepts.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
