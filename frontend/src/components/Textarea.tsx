import type { ComponentPropsWithRef } from 'react'
import { useId } from 'react'
import { cn } from '../lib/format'
import { focusRing } from './Button'

export interface TextareaProps extends ComponentPropsWithRef<'textarea'> {
  label?: string
  hint?: string
  /** When set the field is marked invalid and the message replaces the hint. */
  error?: string
}

/** The multi-line twin of fieldClass: no fixed height, and it grows downward. */
export const textareaClass =
  'w-full rounded border border-border bg-surface px-2 py-1.5 text-sm text-fg placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-50'

export function Textarea({ label, hint, error, className, id, rows = 6, ...rest }: TextareaProps) {
  const autoId = useId()
  const fieldId = id ?? autoId
  const noteId = `${fieldId}-note`
  const note = error ?? hint

  return (
    <div className="flex flex-col gap-1">
      {label !== undefined && (
        <label htmlFor={fieldId} className="text-xs font-medium text-fg-muted">
          {label}
        </label>
      )}
      <textarea
        id={fieldId}
        rows={rows}
        aria-invalid={error !== undefined || undefined}
        aria-describedby={note !== undefined ? noteId : undefined}
        className={cn(textareaClass, focusRing, error !== undefined && 'border-danger', className)}
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
