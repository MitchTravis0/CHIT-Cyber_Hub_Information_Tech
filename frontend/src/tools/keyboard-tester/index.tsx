import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RotateCcw, TriangleAlert } from 'lucide-react'
import { Button, CopyButton, ToolShell } from '../../components'
import { cn } from '../../lib/format'
import {
  buttonName,
  coverageLine,
  describeKey,
  logText,
  modifierList,
  stuckModifiers,
  stuckSentence,
  wheelLabel,
  type KeyInfo,
  type LogEntry,
} from './events'
import { LAYOUT, OS_COMBINATIONS, allCodes, swallowedKeys, type KeyDef } from './layout'

const LOG_MAX = 50
const TOTAL_KEYS = allCodes().length

// Keys the page intercepts so that testing them does not scroll, navigate back
// or reload the app. Ctrl, Alt and Meta combinations are deliberately left
// alone: swallowing those could trap a user inside the tool.
const PREVENT = new Set([
  'Tab', 'Space', 'Backspace', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight',
  'Slash', 'F5', 'F11',
])

export default function KeyboardTesterPage() {
  const [seen, setSeen] = useState<Set<string>>(new Set())
  const [held, setHeld] = useState<Set<string>>(new Set())
  const [heldModifiers, setHeldModifiers] = useState<Set<string>>(new Set())
  const [last, setLast] = useState<KeyInfo | null>(null)
  const [log, setLog] = useState<LogEntry[]>([])

  const [buttonsSeen, setButtonsSeen] = useState<Set<number>>(new Set())
  const [buttonsHeld, setButtonsHeld] = useState<Set<number>>(new Set())
  const [clicks, setClicks] = useState(0)
  const [doubles, setDoubles] = useState(0)
  const [position, setPosition] = useState<{ x: number; y: number } | null>(null)
  const [wheel, setWheel] = useState({ x: 0, y: 0, mode: 0 })

  const lastClick = useRef<{ button: number; at: number } | null>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)

  const codes = useMemo(() => allCodes(), [])
  const dashed = useMemo(() => new Set(swallowedKeys().map((key) => key.code)), [])

  const push = useCallback((kind: string, code: string, detail: string) => {
    setLog((all) => [{ kind, code, detail, atMs: performance.now() }, ...all].slice(0, LOG_MAX))
  }, [])

  // Focus on mount: a keyboard event goes to whatever is focused, and a tech
  // who has clicked a browser control would otherwise see nothing happen.
  useEffect(() => {
    surfaceRef.current?.focus()
  }, [])

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (PREVENT.has(event.code) && !event.ctrlKey && !event.altKey && !event.metaKey) {
      event.preventDefault()
    }
    const info = describeKey(event)
    setLast(info)
    setSeen((all) => new Set(all).add(event.code))
    setHeld((all) => new Set(all).add(event.code))
    setHeldModifiers(new Set(modifierList(event)))
    if (!event.repeat) push('keydown', event.code, `"${event.key}"`)
  }

  const onKeyUp = (event: React.KeyboardEvent) => {
    setLast(describeKey(event))
    setHeld((all) => {
      const next = new Set(all)
      next.delete(event.code)
      return next
    })
    setHeldModifiers(new Set(modifierList(event)))
    push('keyup', event.code, `"${event.key}"`)
  }

  // Returning from Alt+Tab would otherwise leave a modifier looking stuck.
  const onBlur = () => {
    setHeld(new Set())
    setHeldModifiers(new Set())
  }

  const reset = () => {
    setSeen(new Set())
    setHeld(new Set())
    setHeldModifiers(new Set())
    setLast(null)
    setLog([])
  }

  const resetMouse = () => {
    setButtonsSeen(new Set())
    setButtonsHeld(new Set())
    setClicks(0)
    setDoubles(0)
    setWheel({ x: 0, y: 0, mode: 0 })
    setPosition(null)
  }

  const stuck = stuckModifiers(heldModifiers, held.size > 0)

  return (
    <ToolShell
      title="Keyboard and Input Tester"
      description="Press every key and click every button to prove what works and what does not."
      help={
        <>
          <p>
            Click anywhere on this page first, then press every key on the keyboard once. A key
            turns blue while you hold it and stays green afterwards, so you can hand the laptop to
            the user, ask them to go along every row, and read the result yourself. The counter at
            the top says how many are left.
          </p>
          <p className="mt-1.5">
            If a key stays grey, look at the panel underneath before condemning it. A key that
            reports the right <code>code</code> but the wrong character is a keyboard layout
            problem, not a hardware fault: the key works and the operating system is mapping it
            somewhere else. A modifier that shows as held with nothing pressed is either physically
            stuck or Sticky Keys, which users turn on by accident by pressing Shift five times.
          </p>
          <p className="mt-1.5">
            A few keys never reach any application, on any operating system: the operating system
            takes them first. Those are drawn with a dashed outline and listed at the bottom of the
            page, so a dashed key that stays grey tells you nothing either way. The keyboard drawn
            here is the standard 104-key US layout; on a laptop or a European keyboard some keys
            will be missing from the picture, and pressing them still shows up in the event panel
            and the log.
          </p>
        </>
      }
      actions={
        <Button onClick={reset} variant="ghost" icon={<RotateCcw size={14} aria-hidden />}>
          Reset
        </Button>
      }
    >
      <div
        ref={surfaceRef}
        tabIndex={0}
        onKeyDown={onKeyDown}
        onKeyUp={onKeyUp}
        onBlur={onBlur}
        className="flex flex-col gap-4 rounded outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        <p className="text-sm text-fg">
          Click here first, then press every key once. Keys stay green after you have pressed them.
        </p>

        <p className="text-sm font-medium text-fg">{coverageLine(seen, codes, TOTAL_KEYS)}</p>

        {stuck.length > 0 && (
          <p className="flex items-start gap-1.5 text-sm text-warn">
            <TriangleAlert size={16} aria-hidden className="mt-0.5 shrink-0" />
            {stuckSentence(stuck)}
          </p>
        )}

        <div className="flex flex-wrap items-start gap-3 overflow-x-auto">
          {LAYOUT.map((block) => (
            <div key={block.name} className="flex flex-col gap-1">
              {block.rows.map((row, index) => (
                <div key={index} className="flex gap-1">
                  {row.keys.map((key) => (
                    <Cap
                      key={key.code}
                      def={key}
                      down={held.has(key.code)}
                      found={seen.has(key.code)}
                      dashed={dashed.has(key.code)}
                    />
                  ))}
                </div>
              ))}
            </div>
          ))}
        </div>

        <div className="rounded border border-border bg-surface-2 px-3 py-2">
          <h2 className="mb-1.5 text-xs font-semibold tracking-wide text-fg-muted uppercase">
            Last key
          </h2>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-0.5 font-mono text-xs">
            <Row label="code" value={last?.code ?? ''} />
            <Row label="key" value={last === null ? '' : `"${last.key}"`} />
            <Row label="keyCode" value={last === null ? '' : String(last.keyCode)} />
            <Row label="location" value={last?.location ?? ''} />
            <Row label="repeat" value={last === null ? '' : last.repeat ? 'yes' : 'no'} />
            <Row
              label="modifiers"
              value={last === null ? '' : last.modifiers.length === 0 ? 'none' : last.modifiers.join(' ')}
            />
          </dl>
        </div>

        <div className="rounded border border-border bg-surface-2 px-3 py-2">
          <h2 className="mb-1.5 text-xs font-semibold tracking-wide text-fg-muted uppercase">
            Mouse
          </h2>
          <div
            className="h-48 cursor-crosshair rounded border border-border bg-surface"
            onContextMenu={(event) => event.preventDefault()}
            onPointerDown={(event) => {
              setButtonsHeld((all) => new Set(all).add(event.button))
              setButtonsSeen((all) => new Set(all).add(event.button))
              push('mousedown', buttonName(event.button), '')
              const now = performance.now()
              const previous = lastClick.current
              if (previous !== null && previous.button === event.button && now - previous.at < 500) {
                setDoubles((n) => n + 1)
              }
              lastClick.current = { button: event.button, at: now }
              setClicks((n) => n + 1)
            }}
            onPointerUp={(event) => {
              setButtonsHeld((all) => {
                const next = new Set(all)
                next.delete(event.button)
                return next
              })
              push('mouseup', buttonName(event.button), '')
            }}
            onPointerMove={(event) => {
              const box = event.currentTarget.getBoundingClientRect()
              setPosition({
                x: Math.round(event.clientX - box.left),
                y: Math.round(event.clientY - box.top),
              })
            }}
            onWheel={(event) => {
              setWheel((w) => ({
                x: w.x + event.deltaX,
                y: w.y + event.deltaY,
                mode: event.deltaMode,
              }))
              push('wheel', 'wheel', wheelLabel(event.deltaX, event.deltaY, event.deltaMode))
            }}
          >
            <div className="flex flex-wrap gap-1 p-2">
              {[0, 1, 2, 3, 4].map((button) => (
                <span
                  key={button}
                  className={cn(
                    'rounded border px-2 py-1 text-xs',
                    buttonsHeld.has(button)
                      ? 'border-accent bg-accent text-accent-fg'
                      : buttonsSeen.has(button)
                        ? 'border-ok bg-ok/20 text-fg'
                        : 'border-border bg-surface-2 text-fg-muted',
                  )}
                >
                  {buttonName(button)}
                  {buttonsSeen.has(button) && ' ✓'}
                </span>
              ))}
            </div>
            <p className="px-2 text-xs text-fg-muted">
              Click, right-click and scroll in here. Move the pointer to see its position.
            </p>
          </div>
          <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-0.5 font-mono text-xs">
            <Row label="Clicks" value={`${clicks} (${doubles} double)`} />
            <Row
              label="Position"
              value={position === null ? '' : `${position.x}, ${position.y}`}
            />
            <Row label="Wheel" value={wheelLabel(wheel.x, wheel.y, wheel.mode)} />
          </dl>
          <Button size="sm" variant="ghost" className="mt-2" onClick={resetMouse}>
            Reset mouse
          </Button>
        </div>

        <div className="rounded border border-border bg-surface-2 px-3 py-2">
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <h2 className="text-xs font-semibold tracking-wide text-fg-muted uppercase">
              Event log
            </h2>
            <CopyButton value={logText(log)} label="Copy log" />
          </div>
          {log.length === 0 ? (
            <p className="py-4 text-center text-xs text-fg-muted">
              Nothing yet. Press a key or click in the mouse box.
            </p>
          ) : (
            <ol className="max-h-64 overflow-auto font-mono text-xs text-fg">
              {log.map((entry, index) => (
                <li key={`${entry.atMs}-${index}`} className="whitespace-pre">
                  {index + 1 < log.length
                    ? logLineFor(entry, log[index + 1].atMs)
                    : logLineFor(entry, null)}
                </li>
              ))}
            </ol>
          )}
        </div>

        <div className="rounded border border-border bg-surface-2 px-3 py-2 text-xs text-fg-muted">
          <h2 className="mb-1.5 text-xs font-semibold tracking-wide uppercase">
            Keys the operating system keeps
          </h2>
          <ul className="flex flex-col gap-1">
            {swallowedKeys().map((key) => (
              <li key={key.code}>
                <span className="text-fg">{key.label}</span> ({key.code}): {key.swallowed}
              </li>
            ))}
            {OS_COMBINATIONS.map((entry) => (
              <li key={entry.combination}>
                <span className="text-fg">{entry.combination}</span>: {entry.reason}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </ToolShell>
  )
}

function logLineFor(entry: LogEntry, previousAtMs: number | null): string {
  const gap = previousAtMs === null ? 0 : Math.max(0, Math.round(entry.atMs - previousAtMs))
  const head = `+${gap} ms  ${entry.kind}  ${entry.code}`
  return entry.detail === '' ? head : `${head}  ${entry.detail}`
}

function Cap({
  def,
  down,
  found,
  dashed,
}: {
  def: KeyDef
  down: boolean
  found: boolean
  dashed: boolean
}) {
  return (
    <span
      title={def.swallowed ?? def.code}
      aria-label={`${def.label}${found ? ', reported' : ', not yet pressed'}`}
      className={cn(
        'flex h-8 shrink-0 items-center justify-center rounded border px-1 text-[10px] leading-none',
        dashed ? 'border-dashed' : '',
        down
          ? 'border-accent bg-accent text-accent-fg'
          : found
            ? 'border-ok bg-ok/20 text-fg'
            : 'border-border bg-surface-2 text-fg-muted',
      )}
      style={{ width: `${def.width * 2.1}rem` }}
    >
      {def.label}
      {found && !down && ' ✓'}
    </span>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className="text-fg">{value === '' ? '-' : value}</dd>
    </>
  )
}
