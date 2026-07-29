import type { FormEvent, ReactNode } from 'react'
import { useMemo, useState } from 'react'
import {
  CircleCheck,
  CircleHelp,
  Info,
  Link2Off,
  Search,
  ShieldAlert,
  TriangleAlert,
} from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  Spinner,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { inspectLink, type Finding, type Hop, type Report } from './api'
import { hostSlug, levelTone, severityTone } from './display'

const HELP = (
  <>
    <p>
      Paste a link somebody was sent and this shows where it actually goes. Each address in the
      chain is asked, one at a time, where it points next. The page itself is never loaded, never
      rendered and never run, so it is safe to inspect a link you would not click.
    </p>
    <p className="mt-2">
      Mail security products wrap outside links, so a link that starts with
      safelinks.protection.outlook.com or urldefense.proofpoint.com is unwrapped first and the real
      address inside it is shown. Shorteners like bit.ly cannot be unwrapped that way, so those are
      followed instead, which is what the hop table is showing you.
    </p>
    <p className="mt-2">
      Watch for three things in particular. An address written in xn-- form is punycode, and the
      "Really reads as" line shows what it actually spells: a Cyrillic letter that looks exactly like
      a Latin one is how a convincing fake domain is built. A domain registered a few days ago on a
      link claiming to be from a bank is almost always a campaign. And a chain that starts at one
      company and ends at a completely different one is normal for a newsletter and a red flag on
      anything asking for a password.
    </p>
    <p className="mt-2">
      A green banner means none of these checks fired. It does not mean the link is safe: it cannot
      see the page contents, it cannot follow a JavaScript redirect, and a brand new phishing site
      with a clean domain will look fine here. Check that the address is one the user was expecting,
      and when in doubt tell them not to click it.
    </p>
  </>
)

const TIMEOUTS = [
  { value: '10000', label: '10 seconds' },
  { value: '15000', label: '15 seconds' },
  { value: '30000', label: '30 seconds' },
]

// The one message the backend produces about the pasted address itself, which
// belongs beside the field rather than in the page.
const ADDRESS_PROBLEM = 'is not a web address CHIT can follow'

function bannerTone(level: string): { box: string; icon: string; Icon: typeof CircleCheck } {
  switch (levelTone(level)) {
    case 'ok':
      return { box: 'border-ok bg-ok/10', icon: 'text-ok', Icon: CircleCheck }
    case 'warn':
      return { box: 'border-warn bg-warn/10', icon: 'text-warn', Icon: TriangleAlert }
    case 'danger':
      return { box: 'border-danger bg-danger/10', icon: 'text-danger', Icon: ShieldAlert }
    default:
      return { box: 'border-border bg-surface-2', icon: 'text-fg-muted', Icon: CircleHelp }
  }
}

function findingTone(severity: string): { box: string; icon: string; Icon: typeof CircleCheck } {
  const tone = severityTone(severity)
  if (tone === 'danger')
    return { box: 'border-danger bg-danger/10 text-fg', icon: 'text-danger', Icon: ShieldAlert }
  if (tone === 'warn')
    return { box: 'border-warn bg-warn/10 text-fg', icon: 'text-warn', Icon: TriangleAlert }
  return { box: 'border-border bg-surface-2 text-fg', icon: 'text-accent', Icon: Info }
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-xs text-fg-muted sm:pt-0.5">{label}</dt>
      <dd className="min-w-0 text-fg">{children}</dd>
    </div>
  )
}

function FindingItem({ finding }: { finding: Finding }) {
  const { box, icon, Icon } = findingTone(finding.severity)
  return (
    <li className={cn('flex items-start gap-2 rounded border px-3 py-2 text-sm', box)}>
      <Icon size={16} className={cn('mt-0.5 shrink-0', icon)} aria-hidden />
      <span className="min-w-0">{finding.text}</span>
    </li>
  )
}

export default function PhishingInspectorPage() {
  // Nothing is prefilled. There is no link that could sensibly be filled in, and
  // an example would make the tool's first act a request to a host the user
  // never asked about.
  const [url, setUrl] = useState('')
  const [skipAge, setSkipAge] = useState(false)
  const [timeoutMs, setTimeoutMs] = useState('15000')
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string>()
  const [fieldError, setFieldError] = useState<string>()
  const [loading, setLoading] = useState(false)

  const columns = useMemo<Column<Hop>[]>(
    () => [
      { key: 'n', header: '#', align: 'right', width: '3rem', value: (row) => row.n },
      {
        key: 'status',
        header: 'Status',
        align: 'right',
        width: '5.5rem',
        value: (row) => (row.status === 0 ? null : row.status),
        render: (row) => (row.status === 0 ? 'no answer' : String(row.status)),
      },
      {
        key: 'method',
        header: 'Method',
        width: '6rem',
        value: (row) => (row.headRejected ? `${row.method} (HEAD refused)` : row.method),
      },
      {
        key: 'host',
        header: 'Host',
        width: '14rem',
        value: (row) => row.host,
        render: (row) => <span className="font-mono">{row.host}</span>,
      },
      {
        key: 'url',
        header: 'Address',
        value: (row) => row.url,
        render: (row) => <span className="font-mono text-xs break-all">{row.url}</span>,
      },
      {
        key: 'next',
        header: 'Redirects to',
        value: (row) => row.next,
        render: (row) =>
          row.next === '' ? null : <span className="font-mono text-xs break-all">{row.next}</span>,
      },
      {
        key: 'tookMs',
        header: 'Took',
        align: 'right',
        width: '5rem',
        value: (row) => row.tookMs,
        render: (row) => `${row.tookMs} ms`,
      },
      { key: 'error', header: 'Problem', value: (row) => row.error },
    ],
    [],
  )

  const onChangeUrl = (next: string) => {
    setUrl(next)
    // A report from the previous link must never sit under a different one.
    setReport(null)
    setError(undefined)
    setFieldError(undefined)
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true)
    try {
      setReport(
        await inspectLink({ url: url.trim(), timeoutMs: Number(timeoutMs), skipAge }),
      )
      setError(undefined)
      setFieldError(undefined)
    } catch (err) {
      const message = errorMessage(err)
      setReport(null)
      setError(message.includes(ADDRESS_PROBLEM) ? undefined : message)
      setFieldError(message.includes(ADDRESS_PROBLEM) ? message : undefined)
    } finally {
      setLoading(false)
    }
  }

  const findings = report === null ? [] : (report.findings ?? [])
  const unwrapped = report === null ? [] : (report.unwrapped ?? [])
  const banner = bannerTone(report?.level ?? 'ok')

  return (
    <ToolShell
      title="Phishing Link Inspector"
      description="Paste a suspicious link and see where it really ends up before anyone clicks it."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <form onSubmit={onSubmit} className="flex flex-col gap-3">
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1">
              <TextInput
                label="Link to inspect"
                className="font-mono"
                placeholder="https://example.com/click?id=..."
                spellCheck={false}
                autoComplete="off"
                autoFocus
                value={url}
                onChange={(event) => onChangeUrl(event.target.value)}
                error={fieldError ?? undefined}
                hint="Paste the link exactly as it appears. CHIT never opens the page, it only asks each address where it points next."
              />
            </div>
            <Button
              type="submit"
              variant="primary"
              disabled={loading || url.trim() === ''}
              icon={<Search size={14} aria-hidden />}
            >
              Inspect
            </Button>
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer px-3 py-2 text-sm text-fg">Options</summary>
            <div className="flex flex-col gap-3 px-3 pb-3">
              <div>
                <label className="flex items-center gap-2 text-sm text-fg">
                  <input
                    type="checkbox"
                    checked={!skipAge}
                    onChange={(event) => setSkipAge(!event.target.checked)}
                    className="size-4 accent-[var(--accent)]"
                  />
                  Look up how old the destination domain is
                </label>
                <p className="mt-1 text-xs text-fg-muted">
                  Asks rdap.org, which is the only third party contacted apart from the link itself.
                </p>
              </div>
              <Select
                label="Wait before giving up"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
              />
            </div>
          </details>
        </form>

        {error !== undefined && (
          <p role="alert" className="mt-2 text-xs text-danger">
            {error}
          </p>
        )}

        {loading && <Spinner label="Following the link, this takes a few seconds." />}

        {report !== null && !loading && (
          <div className="flex flex-col gap-4">
            <div
              className={cn(
                'flex items-start gap-2 rounded border px-3 py-2 text-sm text-fg',
                banner.box,
              )}
            >
              <banner.Icon size={16} className={cn('mt-0.5 shrink-0', banner.icon)} aria-hidden />
              <p>{report.headline}</p>
            </div>

            {unwrapped.length > 0 && (
              <div className="flex flex-col gap-1">
                {unwrapped.map((step) => (
                  <p
                    key={`${step.wrapper}-${step.to}`}
                    className="flex items-start gap-2 text-xs text-fg-muted"
                  >
                    <Link2Off size={14} className="mt-0.5 shrink-0" aria-hidden />
                    <span className="min-w-0 break-all">
                      Unwrapped {step.wrapper}. The real address inside it is {step.to}.
                    </span>
                  </p>
                ))}
              </div>
            )}

            <div className="rounded border border-border bg-surface-2 px-3 py-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-lg font-semibold break-all text-fg">
                  {report.finalHost.raw}
                </span>
                <CopyButton value={report.final} label="Copy address" />
              </div>
              <dl className="mt-2 grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[9rem_1fr]">
                {report.finalHost.punycode && (
                  <Field label="Really reads as">
                    <span className="font-mono text-warn">{report.finalHost.decoded}</span>
                  </Field>
                )}
                <Field label="Domain">
                  {report.finalHost.isIp ? report.finalHost.raw : report.finalHost.registrable}
                </Field>
                <Field label="Registered">
                  {report.age.known ? (
                    `${report.age.registered} (${report.age.human} ago)`
                  ) : (
                    <span className="text-fg-muted">{report.age.note}</span>
                  )}
                </Field>
                <Field label="Connection">
                  {report.final.startsWith('https://') ? (
                    'Encrypted (https)'
                  ) : (
                    <span className="text-warn">Not encrypted (plain http)</span>
                  )}
                </Field>
              </dl>
            </div>

            {findings.length > 0 ? (
              <ul className="flex flex-col gap-1.5">
                {findings.map((finding) => (
                  <FindingItem key={finding.id} finding={finding} />
                ))}
              </ul>
            ) : (
              <p className="text-sm text-fg-muted">
                Nothing was flagged. That does not make the link safe: it means none of the checks
                below fired.
              </p>
            )}

            {report.stopped !== '' && (
              <p className="flex items-start gap-2 text-xs text-warn">
                <TriangleAlert size={14} className="mt-0.5 shrink-0" aria-hidden />
                <span className="min-w-0">{report.stopped}</span>
              </p>
            )}

            <ResultsTable
              columns={columns}
              rows={report.hops ?? []}
              getRowId={(row) => String(row.n)}
              csvName={'link-hops-' + hostSlug(report.finalHost.raw)}
              rowStatus={(row) => (row.error !== '' ? 'danger' : undefined)}
              emptyMessage="Nothing to show yet. Paste a link and press Inspect."
            />

            <p className="text-xs text-fg-muted">
              CHIT only follows the redirects a server sends in its headers. A page can also
              redirect using JavaScript or a meta refresh tag, and that will not show up here.
            </p>
          </div>
        )}

        {report === null && !loading && (
          <p className="text-sm text-fg-muted">
            Paste a link from a suspicious email and press Inspect. Nothing is opened or run: CHIT
            only asks each address where it points next.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
