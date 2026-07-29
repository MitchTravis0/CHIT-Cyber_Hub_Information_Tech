import type { ReactNode } from 'react'
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { CircleAlert, CircleCheck, Info, X } from 'lucide-react'
import { cn } from '../lib/format'
import { focusRing } from './Button'

export type ToastKind = 'info' | 'error' | 'success'

export interface ToastApi {
  push: (kind: ToastKind, msg: string) => void
}

interface Toast {
  id: number
  kind: ToastKind
  msg: string
}

const ToastContext = createContext<ToastApi | null>(null)

const LIFETIME_MS: Record<ToastKind, number> = {
  info: 4000,
  success: 3000,
  error: 8000,
}

const TONES: Record<ToastKind, string> = {
  info: 'border-border text-fg',
  success: 'border-ok text-fg',
  error: 'border-danger text-fg',
}

const ICONS: Record<ToastKind, ReactNode> = {
  info: <Info size={16} className="mt-px shrink-0 text-accent" />,
  success: <CircleCheck size={16} className="mt-px shrink-0 text-ok" />,
  error: <CircleAlert size={16} className="mt-px shrink-0 text-danger" />,
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)
  const timers = useRef<Array<ReturnType<typeof setTimeout>>>([])

  useEffect(() => () => timers.current.forEach(clearTimeout), [])

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id))
  }, [])

  const push = useCallback(
    (kind: ToastKind, msg: string) => {
      const id = nextId.current++
      setToasts((prev) => prev.concat({ id, kind, msg }).slice(-4))
      timers.current.push(setTimeout(() => dismiss(id), LIFETIME_MS[kind]))
    },
    [dismiss],
  )

  const api = useMemo(() => ({ push }), [push])

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        aria-live="polite"
        className="pointer-events-none fixed right-3 bottom-3 z-50 flex w-80 max-w-[calc(100vw-1.5rem)] flex-col gap-2"
      >
        {toasts.map((toast) => (
          <div
            key={toast.id}
            role={toast.kind === 'error' ? 'alert' : 'status'}
            className={cn(
              'pointer-events-auto flex items-start gap-2 rounded border bg-surface-2 p-2.5 text-sm shadow-lg',
              TONES[toast.kind],
            )}
          >
            {ICONS[toast.kind]}
            <p className="min-w-0 flex-1 break-words">{toast.msg}</p>
            <button
              type="button"
              aria-label="Dismiss notification"
              onClick={() => dismiss(toast.id)}
              className={cn('shrink-0 rounded text-fg-muted hover:text-fg', focusRing)}
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastApi {
  const api = useContext(ToastContext)
  if (api === null) throw new Error('useToast must be used inside a ToastProvider')
  return api
}
