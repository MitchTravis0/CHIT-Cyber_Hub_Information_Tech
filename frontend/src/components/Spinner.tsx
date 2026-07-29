import { cn } from '../lib/format'

export interface SpinnerProps {
  /** Diameter in pixels. */
  size?: number
  label?: string
  className?: string
}

export function Spinner({ size = 14, label = 'Working', className }: SpinnerProps) {
  return (
    <span
      role="status"
      aria-label={label}
      style={{ width: size, height: size }}
      className={cn(
        'inline-block animate-spin rounded-full border-2 border-current border-t-transparent align-[-2px]',
        className,
      )}
    />
  )
}
