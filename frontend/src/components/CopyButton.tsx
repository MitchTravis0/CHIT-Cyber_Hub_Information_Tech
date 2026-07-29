import { useEffect, useRef, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { Button, type ButtonSize } from './Button'
import { useToast } from './Toast'

export interface CopyButtonProps {
  value: string
  /** Visible text. Omit for an icon-only button (still labelled for screen readers). */
  label?: string
  size?: ButtonSize
  className?: string
}

/** The webview clipboard API is not available on every platform, so the Wails
 *  runtime is tried first and the browser API is the fallback. */
async function copy(value: string): Promise<boolean> {
  try {
    if (await ClipboardSetText(value)) return true
  } catch {
    // falls through to the browser API
  }
  try {
    await navigator.clipboard.writeText(value)
    return true
  } catch {
    return false
  }
}

export function CopyButton({ value, label, size = 'sm', className }: CopyButtonProps) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const toast = useToast()

  useEffect(() => () => clearTimeout(timer.current), [])

  const onClick = async () => {
    if (!(await copy(value))) {
      // Both clipboard routes failed, so the button showing nothing at all
      // would look like a dead control.
      toast.push('error', `${value} could not be copied. Select the text and copy it by hand.`)
      return
    }
    setCopied(true)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setCopied(false), 1200)
  }

  return (
    <Button
      size={size}
      variant="ghost"
      className={className}
      onClick={onClick}
      aria-label={label ?? `Copy ${value}`}
      title={label ?? 'Copy'}
      icon={copied ? <Check size={14} className="text-ok" /> : <Copy size={14} />}
    >
      {label !== undefined && (copied ? 'Copied' : label)}
      <span className="sr-only" aria-live="polite">
        {copied ? 'Copied' : ''}
      </span>
    </Button>
  )
}
