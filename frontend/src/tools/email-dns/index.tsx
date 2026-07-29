import type { FormEvent, ReactNode } from 'react'
import { useMemo, useState } from 'react'
import { CircleCheck, MailCheck, ShieldAlert, TriangleAlert } from 'lucide-react'
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
  checkEmailDns,
  type DkimKey,
  type Finding,
  type MailReport,
  type MXHost,
  type SpfTerm,
} from './api'
import {
  lookupTone,
  selectorSentence,
  SPF_LOOKUP_LIMIT,
  verdictLabel,
  verdictTone,
} from './format'

const HELP = (
  <>
    <p>
      This reads the four DNS records that decide whether a domain's email is trusted, and says what
      each one actually allows. It is the first thing to check for "our email goes to junk" and for
      "someone is spoofing our domain", both of which are usually settled in these records rather
      than at the mail server.
    </p>
    <p className="mt-2">
      Type just the domain, not a full email address. MX says where mail for the domain is
      delivered. SPF lists who may send as the domain and what a receiving server should do about
      anyone else. DKIM is the signing key. DMARC ties SPF and DKIM together and says what to do
      when they fail: it is the only one of the four that actually blocks anything.
    </p>
    <p className="mt-2">
      The wording to watch for: SPF ending in <code>~all</code> (soft fail) means a spoofed message
      is marked but usually still delivered, and <code>-all</code> (hard fail) means it should be
      rejected. DMARC <code>p=none</code> means nothing is enforced at all, which is very common and
      is usually the answer to "why is nobody stopping this".
    </p>
    <p className="mt-2">
      DKIM cannot be proved absent. A domain can name its key anything, so CHIT tries a list of
      common selectors and tells you which ones it tried. If none is found, open a message from that
      domain and look at the <code>DKIM-Signature</code> header for <code>s=</code>, then type that
      name into the extra selector box.
    </p>
  </>
)

const TIMEOUTS = [
  { value: '2000', label: '2 seconds' },
  { value: '5000', label: '5 seconds' },
  { value: '20000', label: '20 seconds' },
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

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border border-border bg-surface-2 p-3">
      <h3 className="text-sm font-medium text-fg">{title}</h3>
      <div className="mt-2 flex flex-col gap-2">{children}</div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-xs text-fg-muted">{label}</dt>
      <dd className="min-w-0 text-fg">{value}</dd>
    </div>
  )
}

function orNotSet(value: string): ReactNode {
  return value === '' ? <span className="text-fg-muted">not set</span> : value
}

export default function EmailDnsPage() {
  const [domain, setDomain] = useState('')
  const [domainError, setDomainError] = useState<string | null>(null)
  const [selector, setSelector] = useState('')
  const [server, setServer] = useState('')
  const [timeoutMs, setTimeoutMs] = useState('5000')
  const [report, setReport] = useState<MailReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  const findingColumns = useMemo<Column<Finding>[]>(
    () => [
      { key: 'area', header: 'Area', width: '6rem' },
      {
        key: 'level',
        header: 'Verdict',
        width: '7rem',
        value: (row) => verdictLabel(row.level),
        render: (row) => <StatusDot status={verdictTone(row.level)} label={verdictLabel(row.level)} />,
      },
      { key: 'title', header: 'What', width: '16rem' },
      { key: 'detail', header: 'Meaning' },
    ],
    [],
  )

  const mxColumns = useMemo<Column<MXHost>[]>(
    () => [
      { key: 'preference', header: 'Preference', align: 'right', width: '8rem' },
      { key: 'host', header: 'Host' },
    ],
    [],
  )

  const spfColumns = useMemo<Column<SpfTerm>[]>(
    () => [
      { key: 'raw', header: 'Term', width: '14rem' },
      { key: 'mechanism', header: 'Kind', width: '8rem' },
      { key: 'value', header: 'Value' },
      {
        key: 'costsLookup',
        header: 'Costs a lookup',
        width: '9rem',
        value: (row) => (row.costsLookup ? 'yes' : ''),
      },
    ],
    [],
  )

  const dkimColumns = useMemo<Column<DkimKey>[]>(
    () => [
      { key: 'selector', header: 'Selector', width: '12rem' },
      {
        key: 'hasKey',
        header: 'Key',
        width: '10rem',
        value: (row) => (row.hasKey ? 'Published' : 'Revoked (empty)'),
      },
      { key: 'keyType', header: 'Type', width: '6rem' },
    ],
    [],
  )

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    if (running) return
    const text = domain.trim()
    if (text === '') {
      setDomainError('Type a domain to check, for example example.com.')
      return
    }
    setDomainError(null)
    setRunning(true)
    try {
      setReport(
        await checkEmailDns({
          domain: text,
          selector: selector.trim(),
          server: server.trim(),
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

  const tone = bannerTone(report?.level ?? 'ok')

  return (
    <ToolShell
      title="Email DNS Checker"
      description="Check a domain's MX, SPF, DKIM and DMARC records and say in plain English what they allow."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={onSubmit}>
          <div className="min-w-56 flex-1">
            <TextInput
              label="Domain"
              value={domain}
              onChange={(event) => setDomain(event.target.value)}
              placeholder="example.com"
              spellCheck={false}
              autoComplete="off"
              error={domainError ?? undefined}
              hint="Just the domain, not an email address."
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={running || domain.trim() === ''}
            icon={<MailCheck size={14} aria-hidden />}
          >
            Check
          </Button>
        </form>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Check options</summary>
          <div className="mt-3 flex flex-wrap gap-3">
            <div className="min-w-48 flex-1">
              <TextInput
                label="Extra DKIM selector"
                value={selector}
                onChange={(event) => setSelector(event.target.value)}
                placeholder="selector1"
                spellCheck={false}
                autoComplete="off"
                hint="Checked as well as the common ones. Leave empty if you do not know it."
              />
            </div>
            <div className="min-w-48 flex-1">
              <TextInput
                label="Ask a specific DNS server"
                value={server}
                onChange={(event) => setServer(event.target.value)}
                placeholder="8.8.8.8"
                spellCheck={false}
                autoComplete="off"
                hint="Leave empty to use this computer's own resolver."
              />
            </div>
            <div className="w-44">
              <Select
                label="Wait for an answer"
                options={TIMEOUTS}
                value={timeoutMs}
                onChange={(event) => setTimeoutMs(event.target.value)}
              />
            </div>
          </div>
        </details>

        {running && <Spinner label="Reading the mail records" />}

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
                <p className="mt-1 text-xs text-fg-muted">Answers came from {report.server}.</p>
              </div>
            </div>

            <ResultsTable
              columns={findingColumns}
              rows={report.findings}
              getRowId={(row) => `${row.area}-${row.title}`}
              csvName="email-dns-findings"
              rowStatus={(row) => verdictTone(row.level)}
              emptyMessage="Nothing to report."
            />

            <div>
              <h3 className="mb-2 text-sm font-medium text-fg">Mail servers (MX)</h3>
              {report.nullMx ? (
                <p className="text-sm text-fg-muted">
                  {report.domain} publishes an empty MX record, which is the standard way for a
                  domain to say it accepts no email at all.
                </p>
              ) : (
                <ResultsTable
                  columns={mxColumns}
                  rows={report.mx}
                  getRowId={(row) => row.host}
                  csvName={`mx-${report.domain.replace(/\./g, '-')}`}
                  emptyMessage="No MX records."
                />
              )}
            </div>

            <Panel title="SPF">
              {report.spf.found ? (
                <>
                  <div className="flex items-start gap-2">
                    <code className="min-w-0 flex-1 overflow-x-auto rounded bg-surface-3 px-2 py-1 text-xs text-fg">
                      {report.spf.record}
                    </code>
                    <CopyButton value={report.spf.record} />
                  </div>
                  <p className="text-sm text-fg-muted">{report.spf.verdict}</p>
                  <ResultsTable
                    columns={spfColumns}
                    rows={report.spf.terms}
                    getRowId={(row) => row.raw}
                    csvName={`spf-${report.domain.replace(/\./g, '-')}`}
                    emptyMessage="The record has no terms."
                  />
                  <p className={cn('text-xs', lookupTone(report.spf.lookups))}>
                    {report.spf.lookups} of the {SPF_LOOKUP_LIMIT} allowed DNS lookups used.
                  </p>
                </>
              ) : (
                <p className="text-sm text-fg-muted">No SPF record.</p>
              )}
            </Panel>

            <Panel title="DMARC">
              {report.dmarc.found ? (
                <>
                  <div className="flex items-start gap-2">
                    <code className="min-w-0 flex-1 overflow-x-auto rounded bg-surface-3 px-2 py-1 text-xs text-fg">
                      {report.dmarc.record}
                    </code>
                    <CopyButton value={report.dmarc.record} />
                  </div>
                  <p className="text-sm text-fg-muted">{report.dmarc.verdict}</p>
                  <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-1 text-sm">
                    <Row label="Policy" value={orNotSet(report.dmarc.policy)} />
                    <Row label="Subdomain policy" value={orNotSet(report.dmarc.subdomain)} />
                    <Row label="Applies to" value={`${report.dmarc.pct}% of mail`} />
                    <Row label="Aggregate reports" value={orNotSet(report.dmarc.rua.join(', '))} />
                    <Row label="Failure reports" value={orNotSet(report.dmarc.ruf.join(', '))} />
                  </dl>
                </>
              ) : (
                <p className="text-sm text-fg-muted">No DMARC record.</p>
              )}
            </Panel>

            <Panel title="DKIM">
              {report.dkim.length > 0 ? (
                <ResultsTable
                  columns={dkimColumns}
                  rows={report.dkim}
                  getRowId={(row) => row.selector}
                  csvName={`dkim-${report.domain.replace(/\./g, '-')}`}
                  emptyMessage="No DKIM keys."
                />
              ) : (
                <p className="text-sm text-fg-muted">
                  No DKIM key found at any of the {report.selectorsTried.length} selectors CHIT
                  tried. A domain can use any selector name, so this is not proof there is none.
                  Look at a message header from this domain for the s= value and put it in the extra
                  selector box.
                </p>
              )}
              <p className="text-xs text-fg-muted">{selectorSentence(report.selectorsTried)}</p>
            </Panel>

            <p className="text-sm text-fg-muted">Checked at {localTime(report.checkedAt)}.</p>
          </>
        )}

        {report === null && error === null && !running && (
          <p className="text-sm text-fg-muted">
            Type a domain and press Check to read its mail records.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
