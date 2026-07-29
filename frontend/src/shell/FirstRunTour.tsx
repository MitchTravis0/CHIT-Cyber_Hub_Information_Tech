import { useEffect, useState } from 'react'
import { Button } from '../components'
import { cn } from '../lib/format'
import { Icon } from './Icon'
import { TOUR_STEPS } from './tour'

interface FirstRunTourProps {
  open: boolean
  onClose: () => void
}

/**
 * Shown once on the first launch, and again from Settings. Deliberately a plain
 * overlay rather than pointers at parts of the screen: those need the tools to be
 * on screen already, and the fourth step is about a tool that is not.
 */
export function FirstRunTour({ open, onClose }: FirstRunTourProps) {
  const [index, setIndex] = useState(0)

  useEffect(() => {
    if (open) setIndex(0)
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  if (!open) return null

  const step = TOUR_STEPS[index]
  const last = index === TOUR_STEPS.length - 1

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="tour-title"
        className="corner-cut w-[min(34rem,100%)] rounded border border-t-2 border-border border-t-accent bg-surface shadow-2xl"
      >
        <div className="flex items-start gap-3 px-5 pt-5">
          <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded bg-accent-dim text-accent">
            <Icon name="Compass" size={17} aria-hidden />
          </span>
          <div className="min-w-0">
            <p className="font-display text-[11px] tracking-wide text-fg-muted uppercase">
              Welcome to CHIT, {index + 1} of {TOUR_STEPS.length}
            </p>
            <h2 id="tour-title" className="mt-0.5 text-base font-semibold">
              {step.title}
            </h2>
          </div>
        </div>

        <p className="px-5 pt-3 text-sm leading-relaxed text-fg-muted">{step.body}</p>

        <div className="mt-5 flex items-center gap-2 border-t border-border px-5 py-3">
          <div className="flex flex-1 items-center gap-1.5">
            {TOUR_STEPS.map((each, at) => (
              <button
                key={each.id}
                type="button"
                onClick={() => setIndex(at)}
                aria-label={`Step ${at + 1}: ${each.title}`}
                aria-current={at === index}
                className={cn(
                  'focus-glow size-2 rounded-full transition-colors',
                  at === index ? 'bg-accent' : 'bg-surface-3 hover:bg-fg-muted',
                )}
              />
            ))}
          </div>
          <Button variant="ghost" onClick={onClose}>
            {last ? 'Close' : 'Skip'}
          </Button>
          {index > 0 && <Button onClick={() => setIndex(index - 1)}>Back</Button>}
          {!last && (
            <Button variant="primary" onClick={() => setIndex(index + 1)}>
              Next
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
