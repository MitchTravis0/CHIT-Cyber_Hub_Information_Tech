import type { ComponentPropsWithRef } from 'react'
import { useId } from 'react'
import { cn } from '../lib/format'
import { focusRing } from './Button'

export interface TextInputProps extends ComponentPropsWithRef<'input'> {
  label?: string
  hint?: string
  /** When set the field is marked invalid and the message replaces the hint. */
  error?: string
}

export const fieldClass =
  'h-8 w-full rounded border border-border bg-surface px-2 text-sm text-fg placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-50'

export function TextInput({ label, hint, error, className, id, ...rest }: TextInputProps) {
  const autoId = useId()
  const inputId = id ?? autoId
  const noteId = `${inputId}-note`
  const note = error ?? hint

  return (
    <div className="flex flex-col gap-1">
      {label !== undefined && (
        <label htmlFor={inputId} className="text-xs font-medium text-fg-muted">
          {label}
        </label>
      )}
      <input
        id={inputId}
        type="text"
        aria-invalid={error !== undefined || undefined}
        aria-describedby={note !== undefined ? noteId : undefined}
        className={cn(fieldClass, focusRing, error !== undefined && 'border-danger', className)}
        {...rest}
      />
      {note !== undefined && (
        <p id={noteId} className={cn('text-xs', error !== undefined ? 'text-danger' : 'text-fg-muted')}>
          {note}
        </p>
      )}
    </div>
  )
}
