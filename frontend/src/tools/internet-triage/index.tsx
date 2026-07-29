import { useMemo, useState } from 'react'
import { Square, Stethoscope } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  Select,
  StatusDot,
  ToolShell,
} from '../../components'
import { cn, formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { startInternetTriage, type Rung } from './api'
import { ladderText, statusLabel, statusTone } from './ladder'

const HELP = (
  <>
    <p>
      This runs the whole "is the internet down" checklist in order and stops at the first thing
      that is actually broken. Each step tells you what it tried and what to do about it, so you can
      read the answer straight down the phone without knowing the chain yourself.
    </p>
    <p className="mt-2">
      Press Run. Nothing needs filling in. The steps are: does this computer have an address, does
      the gateway answer, does a name resolve, does a raw IP address answer, does HTTPS work, and is
      a login page in the way. A red step stops the run and everything after it is marked "not
      checked", because those answers would not mean anything until the red one is fixed.
    </p>
    <p className="mt-2">
      An amber step is worth knowing about but is not a fault on its own. The commonest one is a
      gateway that does not answer ping: a lot of business firewalls are set up that way on purpose,
      so the run carries on and uses a TCP connection instead.
    </p>
    <p className="mt-2">
      The last step is the one people miss. Hotel, airport and guest Wi-Fi hand out a perfectly good
      address and then intercept everything until you accept their terms. Every earlier step passes
      and nothing works. If that step is red, open a browser and go to any plain <code>http://</code>{' '}
      page, and the login screen will appear.
    </p>
  </>
)

const TIMEOUTS = [
  { value: '2000', label: '2 seconds' },
  { value: '4000', label: '4 seconds' },
  { value: '10000', label: '10 seconds' },
]

export default function InternetTriagePage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Rung>()
  const [timeoutMs, setTimeoutMs] = useState('4000')

  // The backend emits each rung exactly once, but the UI keys by id anyway so a
  // future change cannot produce duplicate rows.
  const rungs = useMemo(() => {
    const map = new Map<string, Rung>()
    for (const rung of results) map.set(rung.id, rung)
    return Array.from(map.values()).sort((a, b) => a.step - b.step)
  }, [results])

  const headline = typeof done?.summary.headline === 'string' ? done.summary.headline : ''
  const failed = typeof done?.summary.failed === 'number' ? done.summary.failed : 0

  const onRun = async () => {
    await start(() => startInternetTriage({ timeoutMs: Number(timeoutMs) }))
  }

  return (
    <ToolShell
      title="Internet Triage"
      description='Run the whole "is the internet down?" checklist in order and show exactly where it breaks.'
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <Button
            variant="primary"
            disabled={running}
            onClick={() => void onRun()}
            icon={<Stethoscope size={14} aria-hidden />}
          >
            Run
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Cancel
            </Button>
          )}
        </div>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Triage options</summary>
          <div className="mt-3 max-w-56">
            <Select
              label="Wait per step"
              options={TIMEOUTS}
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </div>
        </details>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Starting the checks' : progress.message}
          />
        )}

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {rungs.length > 0 && (
          <ol className="flex flex-col gap-2">
            {rungs.map((rung) => (
              <li
                key={rung.id}
                className="rounded border border-border bg-surface-2 px-3 py-2"
              >
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <StatusDot status={statusTone(rung.status)} label={statusLabel(rung.status)} />
                  <span className="font-medium text-fg">
                    {rung.step}. {rung.name}
                  </span>
                  {rung.target !== '' && (
                    <span className="text-xs text-fg-muted">{rung.target}</span>
                  )}
                  {rung.ms > 0 && (
                    <span className="ml-auto text-xs tabular-nums text-fg-muted">
                      {Math.round(rung.ms)} ms
                    </span>
                  )}
                </div>
                <p className="mt-1 text-sm text-fg">{rung.detail}</p>
                <p className="mt-0.5 text-sm text-fg-muted">{rung.advice}</p>
              </li>
            ))}
          </ol>
        )}

        {done !== null && (
          <>
            <div className="flex items-start gap-2">
              <p className="flex-1 text-sm text-fg">
                Checked {rungs.length} steps in {formatDuration(done.durationMs)}
                {done.cancelled && <span className="text-fg-muted"> (stopped early)</span>}
              </p>
              <CopyButton value={ladderText(rungs)} />
            </div>
            {headline !== '' && (
              <p
                className={cn(
                  'rounded border px-3 py-2 text-sm text-fg',
                  failed > 0 ? 'border-danger bg-danger/10' : 'border-ok bg-ok/10',
                )}
              >
                {headline}
              </p>
            )}
          </>
        )}

        {rungs.length === 0 && !running && error === null && (
          <>
            <p className="text-sm text-fg-muted">Press Run. Nothing needs filling in.</p>
            <p className="text-xs text-fg-muted">
              This checks your adapter and gateway, then example.com, 1.1.1.1, 8.8.8.8,
              www.google.com and detectportal.firefox.com.
            </p>
          </>
        )}
      </div>
    </ToolShell>
  )
}
