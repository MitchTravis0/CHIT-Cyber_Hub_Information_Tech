import { cn } from '../lib/format'

export interface ProgressBarProps {
  value: number
  /** A max of 0 or less renders the indeterminate state (total not known yet). */
  max: number
  label?: string
  className?: string
}

export function ProgressBar({ value, max, label, className }: ProgressBarProps) {
  const indeterminate = max <= 0
  const percent = indeterminate ? 0 : Math.min(100, Math.max(0, (value / max) * 100))

  return (
    <div className={cn('flex flex-col gap-1', className)}>
      {(label !== undefined || !indeterminate) && (
        <div className="flex items-baseline justify-between gap-2 text-xs text-fg-muted">
          <span className="truncate">{label}</span>
          {!indeterminate && (
            <span className="shrink-0 tabular-nums">
              {value} / {max}
            </span>
          )}
        </div>
      )}
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={indeterminate ? undefined : max}
        aria-valuenow={indeterminate ? undefined : value}
        aria-label={label ?? 'Progress'}
        className="h-1.5 w-full overflow-hidden rounded-full bg-surface-3"
      >
        <div
          className={cn('h-full rounded-full bg-accent', indeterminate && 'w-1/3 animate-pulse')}
          style={indeterminate ? undefined : { width: `${percent}%` }}
        />
      </div>
    </div>
  )
}
