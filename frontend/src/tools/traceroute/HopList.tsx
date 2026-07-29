import { cn } from '../../lib/format'
import type { Hop } from './api'
import { barWidth, hopTone } from './hops'

export interface HopListProps {
  hops: Hop[]
  /** The hop flagged as the biggest step up in response time, or -1. */
  jump: number
}

/**
 * The path view: one row per router, with a bar making the slow hop obvious.
 * ResultsTable cannot draw the bar or the chips, so this view is hand built
 * while the table view keeps using the shared component.
 */
export function HopList({ hops, jump }: HopListProps) {
  const anySilent = hops.some((hop) => hop.timesMs.length === 0)

  return (
    <div className="flex flex-col gap-2">
      <ol className="flex flex-col gap-1">
        {hops.map((hop) => {
          const tone = hopTone(hop)
          return (
            <li
              key={hop.number}
              className={cn(
                'flex flex-wrap items-center gap-3 rounded border border-border bg-surface-2 px-3 py-1.5',
                tone === 'warn' && 'bg-warn/10',
                tone === 'danger' && 'bg-danger/10',
              )}
            >
              <span className="w-6 shrink-0 text-right text-xs text-fg-muted tabular-nums">
                {hop.number}
              </span>

              <div className="w-24 shrink-0">
                <div className="h-1.5 rounded-full bg-surface-3">
                  <div
                    className="h-full rounded-full bg-accent"
                    style={{ width: `${barWidth(hop, hops)}%` }}
                  />
                </div>
              </div>

              <div className="min-w-0 flex-1">
                {hop.ip === '' ? (
                  <span className="text-sm text-fg-muted">No reply</span>
                ) : (
                  <>
                    <div className="text-sm text-fg tabular-nums">{hop.ip}</div>
                    {hop.hostname !== '' && (
                      <div className="truncate text-xs text-fg-muted">{hop.hostname}</div>
                    )}
                  </>
                )}
              </div>

              {hop.timesMs.length > 0 && (
                <div className="shrink-0 text-sm text-fg tabular-nums">
                  {hop.avgMs} ms{' '}
                  <span className="text-xs text-fg-muted">
                    best {hop.bestMs} / worst {hop.worstMs}
                  </span>
                </div>
              )}

              <div className="flex shrink-0 gap-1">
                {hop.lost > 0 && (
                  <span className="rounded bg-warn/20 px-1.5 py-0.5 text-xs font-medium text-fg">
                    {hop.lost} lost
                  </span>
                )}
                {hop.number === jump && (
                  <span className="rounded bg-warn/20 px-1.5 py-0.5 text-xs font-medium text-fg">
                    biggest jump
                  </span>
                )}
                {hop.final && (
                  <span className="rounded bg-accent px-1.5 py-0.5 text-xs font-medium text-accent-fg">
                    destination
                  </span>
                )}
                {hop.note !== '' && (
                  <span className="rounded bg-danger/20 px-1.5 py-0.5 text-xs font-medium text-fg">
                    {hop.note}
                  </span>
                )}
                {hop.alsoSeen.length > 0 && (
                  <span className="rounded bg-surface-3 px-1.5 py-0.5 text-xs font-medium text-fg-muted">
                    also {hop.alsoSeen.join(', ')}
                  </span>
                )}
              </div>
            </li>
          )
        })}
      </ol>

      {anySilent && (
        <p className="text-xs text-fg-muted">
          Rows with no reply are normal in the middle of a path: plenty of routers are set not to
          answer traceroute while still passing traffic through.
        </p>
      )}
    </div>
  )
}
