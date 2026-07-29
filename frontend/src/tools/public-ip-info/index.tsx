import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { Button, CopyButton, Spinner, ToolShell } from '../../components'
import { errorMessage } from '../../lib/format'
import { publicIP, type PublicIPInfo } from './api'

const HELP = (
  <>
    <p>
      This is the address the rest of the internet sees, not the one on the adapter. Everyone in the
      office shares it, because the router translates every internal address onto this single public
      one, so it is the address to give a vendor for a firewall rule or a VPN peer.
    </p>
    <p className="mt-1.5">
      The location is wherever the ISP registered the block of addresses, so it is regularly the
      wrong city and sometimes the wrong end of the country. It is not GPS, and it is not the
      customer's address.
    </p>
    <p className="mt-1.5">
      If the router's own WAN address starts 100.64 to 100.127 and the address here is different,
      the ISP is using CGNAT. Inbound port forwarding will not work until they hand the line a real
      public address, which usually means asking them for one.
    </p>
  </>
)

// The spec gives this text with "What leaves this machine:" in front of it and
// asks for that same phrase as the heading, so the prefix lives in the heading
// rather than being printed twice.
const PRIVACY =
  'A plain web request to ipinfo.io (or ipwho.is, or cloudflare.com if those are down) with no account and no identifying details, plus a reverse DNS lookup of the address through whichever DNS server this machine already uses. Those services see the same public address that every website you visit already sees. Nothing about this computer is sent: no hostname, no MAC address, no adapter list, no scan results.'

export default function PublicIPInfoPage() {
  const [info, setInfo] = useState<PublicIPInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setInfo(await publicIP())
      setError(null)
    } catch (err) {
      setInfo(null)
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const located = info !== null && (info.latitude !== 0 || info.longitude !== 0)

  return (
    <ToolShell
      title="Public IP & Connection Info"
      description="Show this site's public IP, ISP and rough location."
      help={HELP}
      actions={
        <Button
          onClick={() => void load()}
          disabled={loading}
          icon={<RefreshCw size={14} className={loading ? 'animate-spin' : undefined} aria-hidden />}
        >
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {loading && (
          <div className="flex items-center gap-2 text-sm text-fg-muted">
            <Spinner /> Asking an outside service what address you come from
          </div>
        )}

        {error !== null && (
          <div className="flex flex-col items-start gap-2">
            <p
              role="alert"
              className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
            >
              {error}
            </p>
            <Button variant="primary" onClick={() => void load()} disabled={loading}>
              Try again
            </Button>
          </div>
        )}

        {info !== null && (
          <div className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-4 py-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-2xl font-semibold tabular-nums text-fg">{info.ipv4}</span>
              <CopyButton value={info.ipv4} label="Copy" />
            </div>

            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
              <dt className="text-fg-muted">IPv6</dt>
              <dd className={info.ipv6 === '' ? 'text-fg-muted' : 'text-fg'}>
                {info.ipv6 === ''
                  ? 'Not reachable over IPv6 (normal on most business lines)'
                  : info.ipv6}
              </dd>
              <Field label="Reverse DNS name" value={info.reverseDns} />
              <Field label="ISP" value={info.isp} />
              <Field label="AS number" value={info.asn} />
              <Field label="City" value={info.city} />
              <Field label="Region" value={info.region} />
              <Field
                label="Country"
                value={info.countryName === '' ? info.country : info.countryName}
              />
              <Field label="Time zone" value={info.timezone} />
              {located && (
                <>
                  <dt className="text-fg-muted">Approximate location</dt>
                  <dd className="flex min-w-0 items-center gap-1 text-fg">
                    <span className="tabular-nums">
                      {info.latitude}, {info.longitude}
                    </span>
                    <CopyButton value={`${info.latitude}, ${info.longitude}`} />
                  </dd>
                </>
              )}
            </dl>

            {info.note !== '' && <p className="text-xs text-warn">{info.note}</p>}

            {info.source !== '' && (
              <p className="text-xs text-fg-muted">
                Answered by {info.source} at {new Date(info.checkedAt).toLocaleTimeString()}.
              </p>
            )}
          </div>
        )}

        <div className="rounded border border-border bg-surface-2 px-3 py-2 text-xs text-fg-muted">
          <p className="font-medium text-fg">What leaves this machine</p>
          <p className="mt-1">{PRIVACY}</p>
        </div>
      </div>
    </ToolShell>
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
