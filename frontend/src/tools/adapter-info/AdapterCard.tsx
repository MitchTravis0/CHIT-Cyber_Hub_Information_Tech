import type { ReactNode } from 'react'
import { CopyButton, StatusDot } from '../../components'
import { cn } from '../../lib/format'
import type { Adapter, AdapterReport } from './api'

interface AdapterCardProps {
  adapter: Adapter
  report: AdapterReport
}

const BADGE = 'rounded border border-border px-1.5 py-0.5 text-[11px] leading-none text-fg-muted'

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-[11px] tracking-wide text-fg-muted uppercase">{label}</dt>
      <dd className="mt-0.5 text-sm break-words text-fg">{children}</dd>
    </div>
  )
}

/** A value worth retyping somewhere else, so it gets a copy button. */
function Copyable({ value, suffix }: { value: string; suffix?: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className="font-mono">{value}</span>
      {suffix !== undefined && <span className="text-xs text-fg-muted">{suffix}</span>}
      <CopyButton value={value} className="h-5 px-1" />
    </span>
  )
}

function Missing({ text }: { text: string }) {
  return <span className="text-sm text-fg-muted italic">{text}</span>
}

const DHCP_LABELS: Record<string, string> = {
  dhcp: 'DHCP (automatic)',
  static: 'Static (manual)',
}

export function AdapterCard({ adapter, report }: AdapterCardProps) {
  const unsupported = new Set(report.unsupported ?? [])
  const ipv4 = adapter.ipv4 ?? []
  const ipv6 = adapter.ipv6 ?? []
  const adapterDNS = adapter.dns ?? []
  const systemDNS = report.dns ?? []
  // Some systems only report resolvers system-wide. Offering them against a
  // disconnected or loopback adapter would just be noise.
  const usesSystemDNS =
    adapterDNS.length === 0 && systemDNS.length > 0 && adapter.up && !adapter.loopback
  const dns = usesSystemDNS ? systemDNS : adapterDNS
  const title = adapter.friendlyName !== '' ? adapter.friendlyName : adapter.name

  return (
    <article
      className={cn(
        'rounded border bg-surface-2',
        adapter.primary ? 'border-accent shadow-sm' : 'border-border',
      )}
    >
      <header
        className={cn(
          'flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-border px-3 py-2',
          adapter.primary && 'bg-accent/10',
        )}
      >
        <StatusDot
          status={adapter.up ? 'ok' : 'idle'}
          label={title}
          className="min-w-0 text-sm font-semibold text-fg"
        />
        {adapter.friendlyName !== '' && adapter.friendlyName !== adapter.name && (
          <span className="font-mono text-xs text-fg-muted">{adapter.name}</span>
        )}
        <div className="ml-auto flex flex-wrap items-center gap-1.5">
          {adapter.primary && (
            <span className="rounded bg-accent px-1.5 py-0.5 text-[11px] leading-none font-medium text-accent-fg">
              Primary
            </span>
          )}
          {!adapter.up && <span className={BADGE}>Disconnected</span>}
          {adapter.virtual && <span className={BADGE}>Virtual</span>}
          {adapter.loopback && <span className={BADGE}>Loopback</span>}
        </div>
      </header>

      <dl className="grid gap-x-6 gap-y-3 p-3 sm:grid-cols-2 lg:grid-cols-3">
        <Field label="IPv4 address">
          {ipv4.length === 0 ? (
            <Missing text="None" />
          ) : (
            <div className="space-y-1">
              {ipv4.map((a) => (
                <div key={a.cidr}>
                  <Copyable value={a.ip} suffix={`/${a.prefix}`} />
                </div>
              ))}
            </div>
          )}
        </Field>

        <Field label="Subnet mask">
          {ipv4.length === 0 ? <Missing text="None" /> : <span className="font-mono">{ipv4[0].mask}</span>}
        </Field>

        <Field label="Network">
          {ipv4.length === 0 ? (
            <Missing text="None" />
          ) : (
            <span className="font-mono">
              {ipv4[0].network}
              {ipv4[0].broadcast !== '' && (
                <span className="text-fg-muted"> (broadcast {ipv4[0].broadcast})</span>
              )}
            </span>
          )}
        </Field>

        <Field label="Default gateway">
          {adapter.gateway !== '' ? (
            <Copyable value={adapter.gateway} />
          ) : unsupported.has('gateway') ? (
            <Missing text="Not available on this OS" />
          ) : (
            <Missing text="None" />
          )}
        </Field>

        <Field label={usesSystemDNS ? 'DNS servers (system-wide)' : 'DNS servers'}>
          {dns.length === 0 ? (
            unsupported.has('dns') ? (
              <Missing text="Not available on this OS" />
            ) : (
              <Missing text="None" />
            )
          ) : (
            <div className="space-y-1">
              {dns.map((server) => (
                <div key={server}>
                  <Copyable value={server} />
                </div>
              ))}
            </div>
          )}
        </Field>

        <Field label="Address assignment">
          {DHCP_LABELS[adapter.dhcp] !== undefined ? (
            DHCP_LABELS[adapter.dhcp]
          ) : unsupported.has('dhcp') ? (
            <Missing text="Not available on this OS" />
          ) : (
            <Missing text="Unknown" />
          )}
        </Field>

        <Field label="MAC address">
          {adapter.mac !== '' ? <Copyable value={adapter.mac} /> : <Missing text="None" />}
        </Field>

        <Field label="MTU">{adapter.mtu > 0 ? adapter.mtu : <Missing text="Unknown" />}</Field>

        <Field label="IPv6 addresses">
          {ipv6.length === 0 ? (
            <Missing text="None" />
          ) : (
            <div className="space-y-1">
              {ipv6.map((a) => (
                <div key={a.cidr} className="flex flex-wrap items-center gap-1">
                  <Copyable value={a.ip} suffix={`/${a.prefix}`} />
                  <span className={BADGE}>{a.scope}</span>
                </div>
              ))}
            </div>
          )}
        </Field>

        {adapter.description !== '' && (
          <Field label="Adapter">
            <span className="text-fg-muted">{adapter.description}</span>
          </Field>
        )}
      </dl>
    </article>
  )
}
