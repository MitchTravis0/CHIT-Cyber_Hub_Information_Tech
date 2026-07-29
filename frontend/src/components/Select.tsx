import type { ComponentPropsWithRef } from 'react'
import { useId } from 'react'
import { cn } from '../lib/format'
import { focusRing } from './Button'
import { fieldClass } from './TextInput'

export interface SelectOption {
  value: string
  label: string
}

export interface SelectProps extends ComponentPropsWithRef<'select'> {
  label?: string
  hint?: string
  options: SelectOption[]
}

export function Select({ label, hint, options, className, id, ...rest }: SelectProps) {
  const autoId = useId()
  const selectId = id ?? autoId
  const noteId = `${selectId}-note`

  return (
    <div className="flex flex-col gap-1">
      {label !== undefined && (
        <label htmlFor={selectId} className="text-xs font-medium text-fg-muted">
          {label}
        </label>
      )}
      <select
        id={selectId}
        aria-describedby={hint !== undefined ? noteId : undefined}
        className={cn(fieldClass, focusRing, 'pr-1', className)}
        {...rest}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      {hint !== undefined && (
        <p id={noteId} className="text-xs text-fg-muted">
          {hint}
        </p>
      )}
    </div>
  )
}
