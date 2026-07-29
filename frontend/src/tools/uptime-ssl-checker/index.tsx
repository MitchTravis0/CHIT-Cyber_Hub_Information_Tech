import type { FormEvent, ReactNode } from 'react'
import { useState } from 'react'
import { ArrowRight, CircleCheck, Search, ShieldAlert, TriangleAlert } from 'lucide-react'
import { Button, CopyButton, Select, Spinner, TextInput, ToolShell } from '../../components'
import { cn, errorMessage, formatBytes } from '../../lib/format'
import { checkSite, type SiteResult, type TlsInfo } from './api'
import { reusedConnection, timingSegments } from './timing'

const HELP = (
  <>
    <p>
      Type an address and press Check. You get the HTTP status, every redirect on the way, where the
      time went, and the certificate. A red banner means the site itself is misbehaving, not that
      this tool failed: read the sentence in the banner and it tells you which part broke.
    </p>
    <p className="mt-2">
      A certificate has to be renewed <em>and</em> reinstalled on the server before the expiry date,
      and that is a job somebody has to book in. The warning starts at 30 days left, which is the
      point to raise a ticket, not the point to panic. Under 7 days it says renew it this week.
    </p>
    <p className="mt-2">
      "Not trusted on this machine" is normal on an internal device. A printer, a NAS or a firewall
      admin page signs its own certificate, so no browser will ever trust it and nobody needs to fix
      it. The same message on a public website is a real fault worth chasing.
    </p>
  </>
)

const TIMEOUTS = [
  { value: '5000', label: '5 seconds' },
  { value: '10000', label: '10 seconds' },
  { value: '30000', label: '30 seconds' },
]

const CAP_BYTES = 262144

function bannerTone(level: string): { box: string; icon: string; Icon: typeof CircleCheck } {
  if (level === 'ok') return { box: 'border-ok bg-ok/10', icon: 'text-ok', Icon: CircleCheck }
  if (level === 'warn')
    return { box: 'border-warn bg-warn/10', icon: 'text-warn', Icon: TriangleAlert }
  return { box: 'border-danger bg-danger/10', icon: 'text-danger', Icon: ShieldAlert }
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-xs text-fg-muted sm:pt-0.5">{label}</dt>
      <dd className="min-w-0 text-fg">{children}</dd>
    </div>
  )
}

function localTime(stamp: string): string {
  if (stamp === '') return 'Not given'
  const when = new Date(stamp)
  return Number.isNaN(when.getTime()) ? stamp : when.toLocaleString()
}

function TimingPanel({ result }: { result: SiteResult }) {
  const segments = timingSegments(result.timing)
  const widths = ['bg-accent', 'bg-accent/70', 'bg-accent/50', 'bg-accent/30']

  return (
    <div className="rounded border border-border bg-surface-2 px-4 py-3">
      <p className="text-xs font-medium text-fg-muted">Where the time went</p>
      {reusedConnection(result.timing) ? (
        <p className="mt-2 text-xs text-fg-muted">
          Connection was reused, so there is no lookup or handshake to break down.
        </p>
      ) : (
        <div className="mt-2 flex h-2 w-full overflow-hidden rounded bg-surface-3">
          {segments.map((segment, index) => (
            <div
              key={segment.key}
              className={widths[index]}
              style={{ width: `${segment.percent}%` }}
              aria-hidden
            />
          ))}
        </div>
      )}
      <dl className="mt-3 flex flex-wrap gap-x-6 gap-y-2 text-sm">
        {segments.map((segment) => (
          <div key={segment.key}>
            <dt className="text-xs text-fg-muted">{segment.label}</dt>
            <dd className="tabular-nums text-fg">{segment.ms} ms</dd>
          </div>
        ))}
        <div>
          <dt className="text-xs text-fg-muted">Total</dt>
          <dd className="tabular-nums text-fg">{result.timing.totalMs} ms</dd>
        </div>
      </dl>
    </div>
  )
}

function Redirects({ result }: { result: SiteResult }) {
  const hops = result.redirects ?? []
  if (hops.length === 0) return null

  return (
    <div className="rounded border border-border bg-surface-2 px-4 py-3">
      <p className="text-xs font-medium text-fg-muted">Redirects</p>
      <ol className="mt-2 space-y-1 text-sm">
        {hops.map((hop, index) => (
          <li key={`${index}-${hop.url}`} className="flex items-start gap-2 text-fg-muted">
            <ArrowRight size={14} className="mt-1 shrink-0" aria-hidden />
            <span className="min-w-0 break-all">
              <span className="tabular-nums">{hop.status}</span> {hop.url}
            </span>
          </li>
        ))}
        <li className="flex items-start gap-2 text-fg">
          <ArrowRight size={14} className="mt-1 shrink-0" aria-hidden />
          <span className="min-w-0 break-all">{result.finalUrl}</span>
        </li>
      </ol>
    </div>
  )
}

function CertificatePanel({ tls }: { tls: TlsInfo }) {
  const days = tls.daysRemaining
  const tone = days < 0 ? 'text-danger' : days <= 30 ? 'text-warn' : 'text-ok'
  const sans = (tls.sans ?? []).join(', ')
  const chainSubjects = tls.chainSubjects ?? []

  return (
    <div className="rounded border border-border bg-surface-2 px-4 py-3">
      <p className="text-xs font-medium text-fg-muted">Certificate</p>
      <p className={cn('mt-1 text-2xl font-semibold tabular-nums', tone)}>{Math.abs(days)}</p>
      <p className="text-xs text-fg-muted">
        {days < 0 ? 'days ago it expired' : 'days until it expires'}
      </p>

      <dl className="mt-3 grid gap-x-4 gap-y-1.5 text-sm sm:grid-cols-[9rem_1fr]">
        <Field label="Issued to">{tls.commonName === '' ? 'Not given' : tls.commonName}</Field>
        <Field label="Issued by">{tls.issuer === '' ? 'Not given' : tls.issuer}</Field>
        <Field label="Valid from">{localTime(tls.notBefore)}</Field>
        <Field label="Valid until">{localTime(tls.notAfter)}</Field>
        <Field label="Covers">
          <span className="flex min-w-0 items-start gap-1">
            <span className="min-w-0 break-words">
              {sans === '' ? 'No names listed on the certificate' : sans}
            </span>
            {sans !== '' && <CopyButton value={sans} />}
          </span>
        </Field>
        <Field label="Chain">
          <span className={tls.chainValid ? 'text-ok' : 'text-warn'}>
            {tls.chainValid ? 'Trusted by this machine' : tls.chainError}
          </span>
        </Field>
        <Field label="Protocol">
          {tls.version}
          {tls.cipherSuite !== '' && `, ${tls.cipherSuite}`}
        </Field>
        <Field label="Key">{tls.keyType}</Field>
        <Field label="Signature">{tls.signatureAlgorithm}</Field>
        <Field label="Serial">
          <span className="font-mono text-xs break-all">{tls.serialNumber}</span>
        </Field>
        <Field label="SHA-256 fingerprint">
          <span className="flex min-w-0 items-start gap-1">
            <span className="min-w-0 font-mono text-xs break-all">{tls.sha256Fingerprint}</span>
            <CopyButton value={tls.sha256Fingerprint} />
          </span>
        </Field>
      </dl>

      {chainSubjects.length > 0 && (
        <div className="mt-3">
          <p className="text-xs font-medium text-fg-muted">What the server sent</p>
          <ol className="mt-1 ml-4 list-decimal space-y-0.5 text-xs text-fg-muted">
            {chainSubjects.map((subject, index) => (
              <li key={`${index}-${subject}`} className="break-words">
                {subject}
              </li>
            ))}
          </ol>
        </div>
      )}
    </div>
  )
}

function Verdict({ result }: { result: SiteResult }) {
  const { box, icon, Icon } = bannerTone(result.level)
  const errors = result.errors ?? []
  const warnings = result.warnings ?? []
  const capped = result.bodyBytes >= CAP_BYTES

  return (
    <div className="mt-4 flex flex-col gap-4">
      <div className={cn('flex items-start gap-2 rounded border px-3 py-2 text-sm text-fg', box)}>
        <Icon size={16} className={cn('mt-0.5 shrink-0', icon)} aria-hidden />
        <p>{result.headline}</p>
      </div>

      {(errors.length > 0 || warnings.length > 0) && (
        <ul className="ml-4 list-disc space-y-1 text-xs">
          {errors.map((sentence, index) => (
            <li key={`e${index}`} className="text-danger">
              {sentence}
            </li>
          ))}
          {warnings.map((sentence, index) => (
            <li key={`w${index}`} className="text-warn">
              {sentence}
            </li>
          ))}
        </ul>
      )}

      <div className="flex flex-wrap items-start gap-x-8 gap-y-3 rounded border border-border bg-surface-2 px-4 py-3">
        <div>
          <p className="text-xs text-fg-muted">Status</p>
          <p className="text-2xl font-semibold tabular-nums text-fg">
            {result.status === 0 ? 'No reply' : result.status}
            {result.statusText !== '' && (
              <span className="ml-2 text-sm font-normal text-fg-muted">
                {result.statusText.replace(`${result.status} `, '')}
              </span>
            )}
          </p>
        </div>
        <div className="min-w-0">
          <p className="text-xs text-fg-muted">Answered at</p>
          <p className="flex min-w-0 items-center gap-1 text-sm">
            <span className="min-w-0 break-all text-fg">{result.finalUrl}</span>
            <CopyButton value={result.finalUrl} />
          </p>
        </div>
        <div>
          <p className="text-xs text-fg-muted">Connected to</p>
          <p className="text-sm text-fg">{result.ip === '' ? 'Nothing answered' : result.ip}</p>
        </div>
        {result.serverHeader !== '' && (
          <div>
            <p className="text-xs text-fg-muted">Server says</p>
            <p className="text-sm text-fg">{result.serverHeader}</p>
          </div>
        )}
        {result.bodyBytes > 0 && (
          <div>
            <p className="text-xs text-fg-muted">Page size</p>
            <p className="text-sm text-fg">
              {formatBytes(result.bodyBytes)}
              {capped && <span className="text-fg-muted"> (first 256 KB)</span>}
            </p>
          </div>
        )}
      </div>

      <TimingPanel result={result} />
      <Redirects result={result} />
      {result.tls !== null && <CertificatePanel tls={result.tls} />}
    </div>
  )
}

export default function UptimeSSLCheckerPage() {
  const [url, setUrl] = useState('')
  const [follow, setFollow] = useState(true)
  const [timeoutMs, setTimeoutMs] = useState('10000')
  const [result, setResult] = useState<SiteResult | null>(null)
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true)
    try {
      setResult(
        await checkSite({
          url: url.trim(),
          timeoutMs: Number(timeoutMs),
          followRedirects: follow,
        }),
      )
      setError(undefined)
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <ToolShell
      title="Website / Service Up Checker"
      description="Check a URL: HTTP status, redirects, response time and certificate expiry."
      help={HELP}
    >
      <form onSubmit={onSubmit} className="flex flex-wrap items-end gap-2">
        <div className="min-w-56 flex-1">
          <TextInput
            label="Address to check"
            placeholder="https://example.com"
            spellCheck={false}
            autoComplete="off"
            autoFocus
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            hint="No https:// needed, it is added for you."
          />
        </div>
        <Select
          label="Wait before giving up"
          options={TIMEOUTS}
          value={timeoutMs}
          onChange={(event) => setTimeoutMs(event.target.value)}
        />
        <Button
          type="submit"
          variant="primary"
          disabled={loading}
          icon={<Search size={14} aria-hidden />}
        >
          Check
        </Button>
      </form>

      <label className="mt-2 flex items-center gap-2 text-sm text-fg">
        <input
          type="checkbox"
          checked={follow}
          onChange={(event) => setFollow(event.target.checked)}
          className="size-4 accent-[var(--accent)]"
        />
        Follow redirects
      </label>

      {error !== undefined && (
        <p role="alert" className="mt-2 text-xs text-danger">
          {error}
        </p>
      )}

      {loading && (
        <div className="mt-4">
          <Spinner label="Checking the address" />
        </div>
      )}

      {result !== null && !loading && <Verdict result={result} />}

      {result === null && !loading && error === undefined && (
        <p className="mt-4 text-sm text-fg-muted">
          Type an address above and press Check. Anything works: a public site, an internal portal,
          or a device on a port such as 192.168.1.10:8443.
        </p>
      )}
    </ToolShell>
  )
}
