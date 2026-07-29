import type { ComponentPropsWithRef, ReactNode } from 'react'
import { cn } from '../lib/format'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'sm' | 'md'

export interface ButtonProps extends ComponentPropsWithRef<'button'> {
  variant?: ButtonVariant
  size?: ButtonSize
  icon?: ReactNode
}

/** The one focus treatment in the app. Defined as a class in style.css because
 *  the box-shadow behind it has a comma in it. */
export const focusRing = 'focus-glow'

const BASE =
  'inline-flex shrink-0 items-center justify-center gap-1.5 rounded border font-medium whitespace-nowrap transition-colors disabled:cursor-not-allowed disabled:opacity-50'

// primary is the only solid fill. danger is outlined rather than filled because
// the app's default accent is red: filled, the safe primary button and the
// destructive one were nearly the same colour, which was measured on the Raw
// Printer Test page where one button asks the printer and the other prints.
// Outlined, they cannot collide whichever accent the user picks.
const VARIANTS: Record<ButtonVariant, string> = {
  primary: 'border-accent bg-accent text-accent-fg hover:brightness-110',
  secondary: 'border-border bg-surface-2 text-fg hover:bg-surface-3',
  ghost: 'border-transparent text-fg-muted hover:bg-surface-2 hover:text-fg',
  danger: 'border-danger bg-transparent text-danger hover:bg-danger/10',
}

const SIZES: Record<ButtonSize, string> = {
  sm: 'h-7 px-2 text-xs',
  md: 'h-8 px-3 text-sm',
}

export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  className,
  type = 'button',
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      className={cn(BASE, focusRing, VARIANTS[variant], SIZES[size], className)}
      {...rest}
    >
      {icon}
      {children}
    </button>
  )
}
