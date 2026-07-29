import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  Spinner,
  StatusDot,
  ToolShell,
} from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { batteryHealth, type Battery, type Report } from './api'
import { healthText, healthTone, stateLabel, unsupportedLabel, whText } from './battery'

const TONE_CLASS: Record<string, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  danger: 'text-danger',
  muted: 'text-fg-muted',
}

export default function BatteryHealthPage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await batteryHealth())
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

  const batteries = report?.batteries ?? []

  return (
    <ToolShell
      title="Battery Health"
      description="How much of its original charge this laptop's battery still holds, and whether it is worth replacing."
      help={
        <>
          <p>
            The number that matters is the big one: how much of its original charge this battery can
            still hold. A new battery is about 100%. Below 80% the user will notice. Below 60% they
            will complain. Below 40% it is the answer to "my laptop dies after an hour" and the
            ticket can be a battery rather than a machine.
          </p>
          <p className="mt-1.5">
            Charge cycles are the other half of the picture. Most laptop batteries are rated for
            somewhere between 300 and 1,000 full cycles, so a battery at 85% health with 1,200 cycles
            behind it is about to fall away even though the health figure still looks fine.
          </p>
          <p className="mt-1.5">
            A health figure slightly above 100% is normal and not a bug. The battery is measured
            against the capacity printed on it by the manufacturer, and a good cell often beats that
            figure slightly when new.
          </p>
          <p className="mt-1.5">
            On Windows, some machines will not give an ordinary user the original capacity. If the
            health figure here is missing, run <code>powercfg /batteryreport</code> from a command
            prompt: it writes a full report to a file and does not need administrator rights either.
            Nothing on this page changes a power setting.
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
        {loading && <Spinner label="Reading the battery" />}

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
            {report.hasAc && (
              <p className="text-sm text-fg">
                {report.onAc ? 'Running on mains power.' : 'Running on battery.'}
              </p>
            )}
            <p className="text-xs text-fg-muted">{report.note}</p>

            {batteries.length === 0 && !loading && (
              <p className="rounded border border-border bg-surface-2 px-3 py-3 text-sm text-fg">
                This machine has no battery, which is normal for a desktop.
              </p>
            )}

            {batteries.map((item) => (
              <BatteryCard
                key={item.name + item.serial}
                battery={item}
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

function BatteryCard({
  battery,
  os,
  unsupported,
}: {
  battery: Battery
  os: string
  unsupported: string[] | null
}) {
  const tone = healthTone(battery.healthPercent)
  const healthGap = unsupportedLabel(os, 'health', unsupported)
  const cyclesGap = unsupportedLabel(os, 'cycles', unsupported)

  return (
    <div className="rounded border border-border bg-surface-2 p-4">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-base font-medium text-fg">
          {battery.model === '' ? battery.name : battery.model}
        </span>
        <span className="text-xs text-fg-muted">{battery.name}</span>
        <StatusDot
          status={
            battery.state === 'charging' || battery.state === 'full'
              ? 'ok'
              : battery.state === 'discharging'
                ? 'idle'
                : 'idle'
          }
          label={stateLabel(battery.state)}
        />
      </div>

      <div className="mt-3 flex flex-wrap items-baseline gap-2">
        <span className={cn('text-2xl', TONE_CLASS[tone])}>
          {healthText(battery.healthPercent)}
        </span>
        <span className="text-sm text-fg-muted">of its original charge</span>
      </div>
      <p className={cn('mt-1 text-sm', TONE_CLASS[tone])}>{battery.verdict}</p>

      {battery.chargePercent >= 0 && (
        <div className="mt-3">
          <ProgressBar
            value={battery.chargePercent}
            max={100}
            label={`Charged ${battery.chargePercent}%`}
          />
        </div>
      )}

      <dl className="mt-4 grid grid-cols-1 gap-x-6 gap-y-1.5 text-sm sm:grid-cols-2">
        <Row label="Original capacity" value={whText(battery.designWh)} gap={healthGap} />
        <Row label="Full charge now" value={whText(battery.fullWh)} gap={healthGap} />
        <Row
          label="Charge cycles"
          value={battery.cycleCount > 0 ? String(battery.cycleCount) : ''}
          gap={cyclesGap}
        />
        <Row label="Technology" value={battery.technology} />
        <Row label="Manufacturer" value={battery.manufacturer} />
        <Row
          label="Serial number"
          value={battery.serial}
          copy={battery.serial === '' ? undefined : battery.serial}
        />
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
