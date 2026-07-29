import type { FormEvent } from 'react'
import { useMemo, useState } from 'react'
import { CircleCheck, Clock, ShieldAlert, TriangleAlert } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  Spinner,
  StatusDot,
  Textarea,
  ToolShell,
  type Column,
} from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { checkNtpTime, type NtpReport, type NtpServer } from './api'
import { describeOffset, resultLabel, resultTone, summaryText } from './offset'

const HELP = (
  <>
    <p>
      This asks a time server what time it is and tells you how far this computer's clock is from
      that answer. A clock more than five minutes out is the usual reason a correct password is
      refused on a domain: Kerberos will not accept a ticket that far from the server's own time,
      and Windows reports it as a wrong username or password.
    </p>
    <p className="mt-2">
      Type one server per line. pool.ntp.org is filled in for you and is the right answer off a
      domain. <strong>On a domain, check your domain controller instead</strong>, because what
      matters is the gap between this computer and the machine handing out logins, not the gap to
      the internet. A name or an IP address both work, and you can add :123 if the server listens
      somewhere unusual.
    </p>
    <p className="mt-2">
      A positive number means this computer is ahead of the server, negative means behind. Under a
      second is normal and needs no action. "No answer" usually means UDP port 123 is blocked
      between here and there, which is common on guest networks and is not itself proof the clock is
      wrong: try another server.
    </p>
    <p className="mt-2">
      Nothing here changes the clock. To fix it: <code>w32tm /resync</code> on Windows (as
      administrator), <code>sudo chronyc makestep</code> or{' '}
      <code>sudo systemctl restart systemd-timesyncd</code> on Linux,{' '}
      <code>sudo sntp -sS time.apple.com</code> on macOS.
    </p>
  </>
)

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
  return Number.isNaN(when.getTime()) ? stamp : when.toLocaleString()
}

export default function NtpCheckPage() {
  const [servers, setServers] = useState('pool.ntp.org')
  const [timeoutMs, setTimeoutMs] = useState('3000')
  const [report, setReport] = useState<NtpReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  const columns = useMemo<Column<NtpServer>[]>(
    () => [
      { key: 'server', header: 'Server', width: '12rem' },
      {
        key: 'status',
        header: 'Result',
        width: '9rem',
        value: (row) => resultLabel(row.status),
        render: (row) => <StatusDot status={resultTone(row.status)} label={resultLabel(row.status)} />,
      },
      {
        key: 'offsetMs',
        header: 'Difference',
        align: 'right',
        width: '10rem',
        value: (row) => (row.status === 'unreachable' ? null : row.offsetMs),
        render: (row) => (row.status === 'unreachable' ? '' : describeOffset(row.offsetMs)),
      },
      {
        key: 'delayMs',
        header: 'Round trip',
        align: 'right',
        width: '8rem',
        value: (row) => (row.status === 'unreachable' ? null : row.delayMs),
        render: (row) => (row.status === 'unreachable' ? '' : `${Math.round(row.delayMs)} ms`),
      },
      {
        key: 'stratum',
        header: 'Stratum',
        align: 'right',
        width: '6rem',
        value: (row) => (row.status === 'unreachable' ? null : row.stratum),
      },
      {
        key: 'serverTime',
        header: 'Server said',
        width: '14rem',
        value: (row) => row.serverTime,
        render: (row) => localTime(row.serverTime),
      },
    ],
    [],
  )

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (running) return
    setRunning(true)
    try {
      setReport(
        await checkNtpTime({
          servers: servers.split('\n'),
          timeoutMs: Number(timeoutMs),
        }),
      )
      setError(null)
    } catch (err) {
      setReport(null)
      setError(errorMessage(err))
    } finally {
      setRunning(false)
    }
  }

  const problems = (report?.servers ?? []).filter((row) => row.status !== 'ok')
  const tone = bannerTone(report?.level ?? 'ok')

  return (
    <ToolShell
      title="NTP Time Check"
      description="Compare this computer's clock against a time server and say whether the difference will break logins."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={onSubmit}>
          <div className="min-w-56 flex-1">
            <Textarea
              label="Time servers"
              value={servers}
              onChange={(event) => setServers(event.target.value)}
              placeholder="pool.ntp.org"
              rows={2}
              spellCheck={false}
              autoComplete="off"
              hint="One server per line, or separated by commas. On a domain, use your domain controller. Press Check to run."
            />
          </div>
          <Button type="submit" variant="primary" disabled={running} icon={<Clock size={14} aria-hidden />}>
            Check
          </Button>
        </form>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Check options</summary>
          <div className="mt-3 max-w-56">
            <Select
              label="Wait for an answer"
              options={TIMEOUTS}
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </div>
        </details>

        {running && <Spinner label="Asking the time servers" />}

        {error !== null && (
          <p role="alert" className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg">
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
              </div>
              <CopyButton value={summaryText(report.servers, report.checkedAt)} />
            </div>

            <ResultsTable
              columns={columns}
              rows={report.servers}
              getRowId={(row) => row.server}
              csvName="ntp-check"
              rowStatus={(row) => {
                if (row.status === 'ok') return 'ok'
                if (row.status === 'warn') return 'warn'
                if (row.status === 'error') return 'danger'
                return undefined
              }}
              emptyMessage="No servers were checked."
            />

            {problems.length > 0 && (
              <ul className="flex list-disc flex-col gap-1 pl-5 text-sm text-fg-muted">
                {problems.map((row) => (
                  <li key={row.server}>{row.message}</li>
                ))}
              </ul>
            )}

            <p className="text-sm text-fg-muted">
              Checked {report.servers.length} {report.servers.length === 1 ? 'server' : 'servers'} at{' '}
              {localTime(report.checkedAt)}.
            </p>
          </>
        )}

        {report === null && error === null && !running && (
          <p className="text-sm text-fg-muted">
            Press Check to compare this computer's clock against a time server.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
