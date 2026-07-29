import { useMemo, useState } from 'react'
import { Play, Square } from 'lucide-react'
import { Button, ProgressBar, Select, ToolShell } from '../../components'
import { formatDuration } from '../../lib/format'
import type { JobDone } from '../../lib/types'
import { useJob } from '../../lib/useJob'
import { startSpeedTest, type Sample } from './api'
import { formatMbps, niceMax, polylinePoints, toMBps } from './chart'

const DURATIONS = [
  { value: '3', label: 'Quick (3 seconds each way)' },
  { value: '8', label: 'Normal (8 seconds each way)' },
  { value: '15', label: 'Thorough (15 seconds each way)' },
]

const STREAMS = [
  { value: '1', label: '1 (strict single stream)' },
  { value: '4', label: '4 (normal)' },
  { value: '8', label: '8 (fills a fast line)' },
]

// A long test would otherwise grow the drawn path forever.
const MAX_PLOTTED = 300

const CHART_WIDTH = 600
const CHART_HEIGHT = 120

/** The live figure while a phase runs, frozen on the headline once it has ended. */
function headline(samples: Sample[]): number | null {
  if (samples.length === 0) return null
  const last = samples[samples.length - 1]
  return last.final ? last.mbps : last.avgMbps
}

export default function SpeedTestPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Sample>()
  const [durationSec, setDurationSec] = useState('8')
  const [streams, setStreams] = useState('4')
  const [skipUpload, setSkipUpload] = useState(false)

  const latency = useMemo(() => results.filter((s) => s.phase === 'latency'), [results])
  const download = useMemo(() => results.filter((s) => s.phase === 'download'), [results])
  const upload = useMemo(() => results.filter((s) => s.phase === 'upload'), [results])

  const downloadMbps = headline(download)
  const uploadMbps = headline(upload)
  const latencyMs = latency.length === 0 ? null : latency[latency.length - 1].latencyMs
  const jitterMs = typeof done?.summary.jitterMs === 'number' ? done.summary.jitterMs : null

  const onStart = () =>
    start(() =>
      startSpeedTest({
        durationSec: Number(durationSec),
        streams: Number(streams),
        skipUpload,
      }),
    )

  return (
    <ToolShell
      title="Speed Test"
      description="Rough download, upload and latency check against Cloudflare."
      help={
        <>
          <p>
            Press Start test and wait. Download and upload are in megabits per second (Mbps), which
            is how a line is sold, while a browser shows a download in megabytes per second, so
            100 Mbps arrives at about 12 MB/s. Latency is the time to reach Cloudflare and back, and
            jitter is how much that time wobbles. The test moves real data, roughly 25 MB for every
            second on a 200 Mbps line, so do not run it on a metered connection such as a 4G dongle.
          </p>
          <p className="mt-1.5">
            A low number is not proof the line is at fault. Wi-Fi, a VPN, an old network card, a busy
            machine or something else already downloading will all cap the result well below what the
            line can really do. Take a slow result as "look closer": run it again wired, with nothing
            else running, before quoting a number to an ISP.
          </p>
          <p className="mt-1.5">
            Upload is usually far smaller than download on a consumer line, and that is normal. It
            still matters: one PC saturating the upload makes everything on the site feel slow, which
            is the first thing to check when a user says video calls break up or freeze.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void onStart()
          }}
        >
          <div className="flex flex-wrap items-end gap-2">
            <Button
              type="submit"
              variant="primary"
              disabled={running}
              icon={<Play size={14} aria-hidden />}
            >
              Start test
            </Button>
            {running && (
              <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
                Stop
              </Button>
            )}
          </div>

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Test options
            </summary>
            <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
              <Select
                label="How long each part runs"
                options={DURATIONS}
                value={durationSec}
                onChange={(event) => setDurationSec(event.target.value)}
                hint="Longer is steadier on a busy line."
              />
              <Select
                label="Connections at once"
                options={STREAMS}
                value={streams}
                onChange={(event) => setStreams(event.target.value)}
                hint="A single connection often under-reads a fast line. Four is what most speed tests use."
              />
              <label className="flex items-center gap-2 text-sm text-fg">
                <input
                  type="checkbox"
                  checked={skipUpload}
                  onChange={(event) => setSkipUpload(event.target.checked)}
                  className="size-4 accent-[var(--accent)]"
                />
                Skip the upload test
              </label>
            </div>
          </details>
        </form>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={progress.message === '' ? 'Starting the test' : progress.message}
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

        <div className="grid gap-2 sm:grid-cols-3">
          <Tile
            label="Download"
            value={downloadMbps === null ? '-' : formatMbps(downloadMbps)}
            unit="Mbps"
            sub={downloadMbps === null ? '' : `${toMBps(downloadMbps).toFixed(1)} MB/s`}
          />
          <Tile
            label="Upload"
            value={uploadMbps === null ? '-' : formatMbps(uploadMbps)}
            unit="Mbps"
            sub={uploadMbps === null ? '' : `${toMBps(uploadMbps).toFixed(1)} MB/s`}
          />
          <Tile
            label="Latency"
            value={latencyMs === null ? '-' : String(Math.round(latencyMs))}
            unit="ms"
            sub={jitterMs === null ? '' : `jitter ${jitterMs.toFixed(1)} ms`}
          />
        </div>

        <p className="text-xs text-warn">
          Approximate. This measures this computer's path to Cloudflare right now, including Wi-Fi,
          VPN and anything else running. Test again wired and with nothing else downloading before
          quoting a number to an ISP.
        </p>

        {download.length > 0 && <PhaseChart title="Download" samples={download} />}
        {upload.length > 0 && <PhaseChart title="Upload" samples={upload} />}

        {done !== null && <Summary done={done} />}

        {results.length === 0 && !running && (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Press Start test to measure this connection. It takes about twenty seconds.
          </p>
        )}
      </div>
    </ToolShell>
  )
}

interface TileProps {
  label: string
  value: string
  unit: string
  sub: string
}

function Tile({ label, value, unit, sub }: TileProps) {
  return (
    <div className="flex flex-col rounded border border-border bg-surface-2 px-3 py-2">
      <span className="text-xs font-medium text-fg-muted">{label}</span>
      <span className="text-3xl font-semibold text-fg tabular-nums">{value}</span>
      <span className="text-sm text-fg-muted">{unit}</span>
      <span className="text-xs text-fg-muted">{sub === '' ? ' ' : sub}</span>
    </div>
  )
}

function PhaseChart({ title, samples }: { title: string; samples: Sample[] }) {
  const values = samples.slice(-MAX_PLOTTED).map((s) => s.mbps)
  const peak = values.reduce((highest, value) => (value > highest ? value : highest), 0)
  const max = niceMax(peak)
  const average = headline(samples) ?? 0
  const averageY = CHART_HEIGHT - Math.min(1, Math.max(0, average / max)) * CHART_HEIGHT

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between">
        <span className="text-xs font-medium text-fg-muted">{title} over time</span>
        <span className="text-xs text-fg-muted">{formatMbps(max)} Mbps</span>
      </div>
      <svg
        viewBox={`0 0 ${CHART_WIDTH} ${CHART_HEIGHT}`}
        preserveAspectRatio="none"
        className="h-28 w-full"
        role="img"
        aria-label={`${title} speed over time, peaking at ${formatMbps(peak)} Mbps`}
      >
        <line
          x1="0"
          y1={averageY}
          x2={CHART_WIDTH}
          y2={averageY}
          stroke="currentColor"
          strokeWidth="1"
          strokeDasharray="4 4"
          className="text-fg-muted"
        />
        <polyline
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          className="text-accent"
          points={polylinePoints(values, CHART_WIDTH, CHART_HEIGHT, max)}
        />
      </svg>
    </div>
  )
}

function Summary({ done }: { done: JobDone }) {
  const num = (key: string): number | null => {
    const value = done.summary[key]
    return typeof value === 'number' ? value : null
  }
  const server = typeof done.summary.server === 'string' ? done.summary.server : ''
  const note = typeof done.summary.note === 'string' ? done.summary.note : ''

  const parts: string[] = []
  const downloadMbps = num('downloadMbps')
  if (downloadMbps !== null && downloadMbps > 0) parts.push(`${formatMbps(downloadMbps)} Mbps down`)
  const uploadMbps = num('uploadMbps')
  if (uploadMbps !== null && uploadMbps > 0) parts.push(`${formatMbps(uploadMbps)} Mbps up`)
  const latencyMs = num('latencyMs')
  if (latencyMs !== null && latencyMs > 0) parts.push(`${Math.round(latencyMs)} ms latency`)
  const jitterMs = num('jitterMs')
  if (jitterMs !== null) parts.push(`${Math.round(jitterMs)} ms jitter`)

  return (
    <div className="flex flex-col gap-1 rounded border border-border bg-surface-2 px-3 py-2">
      <p className="text-sm text-fg">
        Finished in {formatDuration(done.durationMs)}
        {server !== '' && ` against ${server}`}
        {parts.length > 0 && `: ${parts.join(', ')}`}.
        {done.cancelled && <span className="text-fg-muted"> (stopped early)</span>}
      </p>
      {note !== '' && <p className="text-xs text-fg-muted">{note}</p>}
    </div>
  )
}
