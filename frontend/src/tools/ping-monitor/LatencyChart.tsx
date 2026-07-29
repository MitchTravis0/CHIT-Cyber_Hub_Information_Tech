import { useMemo } from 'react'
import type { Sample } from './api'
import { chartScale, pointX, pointY, segments, DASHES, PLOT, SERIES } from './chart'
import { seriesFor } from './stats'

/** How many pings the graph shows. Older samples stay in the stats and the CSV. */
export const WINDOW = 300

interface LatencyChartProps {
  samples: Sample[]
  targets: string[]
}

export function LatencyChart({ samples, targets }: LatencyChartProps) {
  const series = useMemo(
    () => targets.map((target) => seriesFor(samples, target, WINDOW)),
    [samples, targets],
  )
  const scale = useMemo(
    () => chartScale(series.flat().filter((s) => s.ok).map((s) => s.latencyMs)),
    [series],
  )

  const empty = series.every((line) => line.length === 0)

  return (
    <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
        {targets.map((target, i) => {
          const line = series[i]
          const last = line.length === 0 ? undefined : line[line.length - 1]
          return (
            <span key={target} className="inline-flex items-center gap-1.5 text-fg-muted">
              <span
                aria-hidden
                className="size-2.5 rounded-full"
                style={{ backgroundColor: SERIES[i] }}
              />
              <span className="text-fg">{target}</span>
              <span className="tabular-nums">
                {last === undefined || !last.ok ? 'last: no reply' : `last: ${last.latencyMs} ms`}
              </span>
            </span>
          )
        })}
      </div>

      <svg
        viewBox={`0 0 ${PLOT.width} ${PLOT.height}`}
        className="h-40 w-full"
        role="img"
        aria-label={`Latency over the last ${WINDOW} pings for ${targets.join(', ')}`}
      >
        {scale.ticks.map((tick, index) => {
          const y = pointY(tick, scale.max)
          return (
            <g key={tick}>
              <line
                x1={PLOT.left}
                x2={PLOT.width - PLOT.right}
                y1={y}
                y2={y}
                stroke="var(--border)"
                strokeWidth={1}
                vectorEffect="non-scaling-stroke"
              />
              <text
                x={PLOT.left - 6}
                y={y + 3}
                textAnchor="end"
                className="fill-fg-muted text-[10px]"
              >
                {index === scale.ticks.length - 1 ? `${tick} ms` : tick}
              </text>
            </g>
          )
        })}

        {series.map((line, i) =>
          segments(line, scale.max).map((run, index) => (
            <g key={`${targets[i]}-${index}`}>
              <polyline
                fill="none"
                stroke={SERIES[i]}
                strokeDasharray={DASHES[i]}
                strokeWidth={1.5}
                vectorEffect="non-scaling-stroke"
                points={run.map(([x, y]) => `${x},${y}`).join(' ')}
              />
              {run.length === 1 && (
                // A one point polyline draws nothing, so a lone reply between two
                // losses would be invisible without this.
                <circle cx={run[0][0]} cy={run[0][1]} r={1.5} fill={SERIES[i]} />
              )}
            </g>
          )),
        )}

        {series.map((line, i) =>
          line.map((sample, index) =>
            sample.ok ? null : (
              <rect
                key={`${targets[i]}-drop-${sample.round}`}
                x={pointX(index, line.length) - 1}
                y={PLOT.dropY + i * 2}
                width={2}
                height={PLOT.dropH}
                fill="var(--danger)"
              />
            ),
          ),
        )}

        {empty && (
          <text
            x={PLOT.width / 2}
            y={70}
            textAnchor="middle"
            className="fill-fg-muted text-[11px]"
          >
            No pings yet
          </text>
        )}
      </svg>
    </div>
  )
}
