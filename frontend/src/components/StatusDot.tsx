import { cn } from '../lib/format'

export type StatusTone = 'ok' | 'warn' | 'danger' | 'idle' | 'busy'

export interface StatusDotProps {
  status: StatusTone
  /** Shown next to the dot. When omitted the dot is labelled for screen readers only. */
  label?: string
  className?: string
}

// Only the two "live" tones glow, so a lit dot reads as something happening
// rather than as decoration on every state.
const TONES: Record<StatusTone, string> = {
  ok: 'bg-ok shadow-[0_0_6px_var(--ok)]',
  warn: 'bg-warn',
  danger: 'bg-danger',
  idle: 'bg-fg-muted/50',
  busy: 'bg-accent animate-pulse shadow-[0_0_6px_var(--accent)]',
}

export function StatusDot({ status, label, className }: StatusDotProps) {
  return (
    <span className={cn('inline-flex items-center gap-1.5', className)}>
      <span
        aria-hidden={label !== undefined}
        aria-label={label === undefined ? status : undefined}
        role={label === undefined ? 'img' : undefined}
        className={cn('size-2 shrink-0 rounded-full', TONES[status])}
      />
      {label !== undefined && <span className="truncate">{label}</span>}
    </span>
  )
}
