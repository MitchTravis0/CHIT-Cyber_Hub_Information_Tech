import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { Button, CopyButton, Spinner, StatusDot, ToolShell } from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { wifiInfo, type Link, type Report } from './api'
import {
  channelText,
  rateText,
  signalText,
  signalTone,
  unsupportedLabel,
  widthText,
} from './link'

const TONE_CLASS: Record<string, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  danger: 'text-danger',
}

export default function WifiInfoPage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await wifiInfo())
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const links = report?.links ?? []

  return (
    <ToolShell
      title="Wi-Fi Connection Info"
      description="What this machine's wireless is doing right now: network, band, channel, speed and signal."
      help={
        <>
          <p>
            The Network Adapter Info tool tells you the addressing: IP, gateway, DNS. This one tells
            you the radio, which is what "the Wi-Fi is bad over here" is actually about. Walk to the
            desk in question, press Refresh, and read the signal.
          </p>
          <p className="mt-1.5">
            Signal in dBm is a negative number and closer to zero is stronger. Roughly: -50 is
            excellent, -67 is the edge of what a video call needs, and anything past -75 will drop.
            Windows reports a percentage instead of dBm, so the percentage is the real figure there
            and the two are shown on the same scale.
          </p>
          <p className="mt-1.5">
            Two things worth noticing besides the signal. A client on 2.4 GHz in a building whose
            access points are 5 GHz is the client's fault, not the network's. And a low negotiated
            rate with a strong signal means interference rather than distance, which is a different
            fix: a microwave, a cordless phone, or too many access points on the same channel.
          </p>
          <p className="mt-1.5">
            Nothing here connects, disconnects or changes a setting, and no saved password is read.
            This page only reads what the adapter is doing.
          </p>
        </>
      }
      actions={
        <Button
          onClick={() => void load()}
          disabled={loading}
          icon={<RefreshCw size={14} aria-hidden />}
        >
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {loading && <Spinner label="Reading the wireless connection" />}

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
            <p className="text-xs text-fg-muted">{report.note}</p>

            {links.length === 0 && !loading && (
              <p className="rounded border border-border bg-surface-2 px-3 py-3 text-sm text-fg">
                This machine has no wireless adapter, or it is switched off.
              </p>
            )}

            {links.map((link) => (
              <LinkCard
                key={link.interface}
                link={link}
                os={report.os}
                unsupported={report.unsupported}
              />
            ))}
          </>
        )}
      </div>
    </ToolShell>
  )
}

function LinkCard({
  link,
  os,
  unsupported,
}: {
  link: Link
  os: string
  unsupported: string[] | null
}) {
  if (!link.connected) {
    return (
      <div className="rounded border border-border bg-surface-2 p-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-base font-medium text-fg">{link.interface}</span>
          <StatusDot status="idle" label="Not connected" />
        </div>
        <p className="mt-2 text-sm text-fg-muted">
          This adapter is switched on but not joined to a network.
        </p>
      </div>
    )
  }

  const ssidGap = unsupportedLabel(os, 'ssid', unsupported)
  const tone = signalTone(link.signalPercent)
  const signal = signalText(link)

  return (
    <div className="rounded border border-border bg-surface-2 p-4">
      <div className="flex flex-wrap items-center gap-3">
        {link.ssid === '' ? (
          <span className="text-base font-medium text-fg-muted">Network name not reported</span>
        ) : (
          <span className="text-base font-medium text-fg">{link.ssid}</span>
        )}
        <span className="text-xs text-fg-muted">{link.interface}</span>
        <StatusDot status="ok" label="Connected" />
      </div>

      {signal === '' ? (
        <p className="mt-3 text-sm text-fg-muted">
          This computer did not report a signal strength for this adapter.
        </p>
      ) : (
        <>
          <div className="mt-3 flex flex-wrap items-baseline gap-2">
            <span className={cn('text-2xl', TONE_CLASS[tone])}>{signal}</span>
            {link.signalDbm !== 0 && link.signalPercent > 0 && (
              <span className="text-sm text-fg-muted">{link.signalPercent}%</span>
            )}
          </div>
          <p className={cn('mt-1 text-sm', TONE_CLASS[tone])}>{link.reading}</p>
        </>
      )}

      <dl className="mt-4 grid grid-cols-1 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
        <Row label="Band" value={link.band} />
        <Row label="Channel" value={channelText(link)} />
        <Row
          label="Width"
          value={widthText(link.widthMhz)}
          gap={unsupportedLabel(os, 'width', unsupported)}
        />
        <Row label="Receive rate" value={rateText(link.rxMbps)} />
        <Row label="Transmit rate" value={rateText(link.txMbps)} />
        <Row
          label="Security"
          value={link.security}
          gap={unsupportedLabel(os, 'security', unsupported)}
        />
        <Row
          label="Access point"
          value={link.bssid}
          copy={link.bssid === '' ? undefined : link.bssid}
        />
        {ssidGap !== null && <Row label="Network name" value="" gap={ssidGap} />}
      </dl>
    </div>
  )
}

function Row({
  label,
  value,
  gap,
  copy,
}: {
  label: string
  value: string
  gap?: string | null
  copy?: string
}) {
  const shown = value !== '' ? value : (gap ?? 'not reported')
  return (
    <div className="flex items-baseline justify-between gap-2 border-b border-border py-1">
      <dt className="text-fg-muted">{label}</dt>
      <dd className={cn('flex items-center gap-1.5 text-right', value === '' && 'text-fg-muted')}>
        {shown}
        {copy !== undefined && <CopyButton value={copy} />}
      </dd>
    </div>
  )
}
