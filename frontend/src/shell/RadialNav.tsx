import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { focusRing } from '../components'
import { cn } from '../lib/format'
import type { ToolCategory } from '../tools/registry'
import { Icon } from './Icon'
import { arcPath, layoutRing, polarToXY } from './radial'
import { toolGroups, type View } from './nav'

const MAX_BOX = 480
const MIN_BOX = 260
// Radii as a fraction of the box, so the ring is drawn at one user unit per CSS
// pixel and the SVG carries no viewBox scaling to get wrong.
const OUTER_RATIO = 200 / 480
const INNER_RATIO = 120 / 480

interface RadialNavProps {
  onNavigate: (view: View) => void
}

type Level = { kind: 'categories' } | { kind: 'tools'; category: ToolCategory }

interface Item {
  key: string
  icon: string
  /** Short name for the hub, and the accessible name of the segment. */
  name: string
  detail: string
  label: string
}

/**
 * Browsable ring of categories, then of the tools inside one. The sidebar and
 * the command palette stay the fast paths; this is the one you point at.
 *
 * The SVG is decorative and aria-hidden. Every segment's semantics and keyboard
 * handling live on a real HTML button in the overlay above it, so nothing here
 * depends on SVG elements being focusable, which is uneven across the three
 * webviews we ship to. The wedge is still the mouse target.
 */
export function RadialNav({ onNavigate }: RadialNavProps) {
  const groups = useMemo(() => toolGroups(), [])
  const [level, setLevel] = useState<Level>({ kind: 'categories' })
  const [active, setActive] = useState(0)
  const [hover, setHover] = useState<number | null>(null)
  const [focused, setFocused] = useState(false)
  const buttons = useRef<Array<HTMLButtonElement | null>>([])
  const measure = useRef<HTMLDivElement>(null)
  // The ring is sized in real pixels, measured off the row it sits in, and both
  // the box and the SVG get an explicit width and height. Nothing here leans on
  // aspect-ratio, on percentage heights or on an SVG's intrinsic viewBox ratio,
  // so there is no layout inference to differ between the three webviews.
  const [box, setBox] = useState(MIN_BOX)
  // Set when a level change came from the keyboard, so focus follows the ring
  // down and back up but a mouse user never gets focus yanked around.
  const takeFocus = useRef(false)

  const group = level.kind === 'tools' ? groups.find((g) => g.category === level.category) : undefined

  const items: Item[] = useMemo(() => {
    if (group) {
      return group.tools.map((tool) => ({
        key: tool.id,
        icon: tool.icon,
        name: tool.name,
        detail: tool.description,
        label: tool.name,
      }))
    }
    return groups.map((g) => ({
      key: g.category,
      icon: g.icon,
      name: g.label,
      detail: g.tools.length === 1 ? '1 tool' : `${g.tools.length} tools`,
      label: `${g.label}, ${g.tools.length === 1 ? '1 tool' : `${g.tools.length} tools`}`,
    }))
  }, [group, groups])

  const ring = useMemo(() => layoutRing(items.length, { startDeg: -90 }), [items.length])

  const goBack = useCallback(() => {
    takeFocus.current = true
    setLevel({ kind: 'categories' })
    setHover(null)
  }, [])

  const open = useCallback(
    (index: number) => {
      const item = items[index]
      if (!item) return
      if (level.kind === 'categories') {
        takeFocus.current = true
        setLevel({ kind: 'tools', category: item.key as ToolCategory })
        setHover(null)
        return
      }
      onNavigate({ kind: 'tool', id: item.key })
    },
    [items, level.kind, onNavigate],
  )

  useEffect(() => {
    const row = measure.current
    if (!row) return
    const fit = (width: number) => setBox(Math.max(MIN_BOX, Math.min(MAX_BOX, Math.round(width))))
    fit(row.clientWidth)
    const observer = new ResizeObserver((entries) => fit(entries[0].contentRect.width))
    observer.observe(row)
    return () => observer.disconnect()
  }, [])

  // Reset the roving index whenever the ring is rebuilt, and follow it with
  // focus only when the keyboard put us here.
  useEffect(() => {
    setActive(0)
    if (!takeFocus.current) return
    takeFocus.current = false
    buttons.current[0]?.focus()
  }, [level])

  const move = useCallback(
    (next: number) => {
      setActive(next)
      setHover(null)
      buttons.current[next]?.focus()
    },
    [],
  )

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const last = items.length - 1
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        move(active === last ? 0 : active + 1)
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        move(active === 0 ? last : active - 1)
        break
      case 'Home':
        move(0)
        break
      case 'End':
        move(last)
        break
      case 'Escape':
        if (level.kind === 'categories') return
        goBack()
        break
      default:
        return
    }
    event.preventDefault()
  }

  const highlight = hover ?? (focused ? active : null)
  const shown = highlight === null ? undefined : items[highlight]

  const centre = box / 2
  const rOuter = box * OUTER_RATIO
  const rInner = box * INNER_RATIO
  const rIcon = (rInner + rOuter) / 2

  return (
    <div ref={measure} className="w-full">
      <div
        className="relative mx-auto"
        style={{ width: box, height: box }}
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false)
        }}
        onFocus={() => setFocused(true)}
      >
        <svg
          viewBox={`0 0 ${box} ${box}`}
          width={box}
          height={box}
          className="block"
          aria-hidden
          focusable="false"
        >
          {ring.map((segment, index) => (
            <path
              key={items[index].key}
              d={arcPath(centre, centre, rInner, rOuter, segment.startDeg, segment.endDeg)}
              className={cn(
                'cursor-pointer transition-[fill,stroke] duration-150',
                highlight === index
                  ? 'fill-accent-dim stroke-accent'
                  : 'fill-surface-2 stroke-border hover:fill-surface-3',
              )}
              strokeWidth={1}
              onClick={() => open(index)}
              onMouseEnter={() => setHover(index)}
              onMouseLeave={() => setHover((current) => (current === index ? null : current))}
            />
          ))}
        </svg>

        {/* Semantics and keyboard live here, on top of the wedges. */}
        <div
          role="menu"
          aria-label={group ? `Tools in ${group.label}` : 'Tool categories'}
          onKeyDown={onKeyDown}
          className="pointer-events-none absolute inset-0"
        >
          {ring.map((segment, index) => {
            const point = polarToXY(centre, centre, rIcon, segment.midDeg)
            return (
              <button
                key={items[index].key}
                ref={(node) => {
                  buttons.current[index] = node
                }}
                type="button"
                role="menuitem"
                tabIndex={index === active ? 0 : -1}
                aria-label={items[index].label}
                onClick={() => open(index)}
                onMouseEnter={() => setHover(index)}
                onMouseLeave={() => setHover((current) => (current === index ? null : current))}
                className={cn(
                  'pointer-events-auto absolute flex size-10 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded transition-colors',
                  highlight === index ? 'text-accent' : 'text-fg-muted',
                  focusRing,
                )}
                style={{ left: point.x, top: point.y }}
              >
                <Icon name={items[index].icon} size={20} aria-hidden />
              </button>
            )
          })}
        </div>

        {/* Hub: names whatever is under the pointer, and is the way back up. */}
        <div
          className="pointer-events-none absolute top-1/2 left-1/2 flex w-[42%] -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1 text-center"
          aria-live="polite"
        >
          {group ? (
            <>
              <button
                type="button"
                onClick={goBack}
                className={cn(
                  'pointer-events-auto flex items-center gap-1 rounded px-2 py-1 font-display text-[11px] tracking-wide text-fg-muted uppercase transition-colors hover:text-fg',
                  focusRing,
                )}
              >
                <Icon name="ChevronLeft" size={13} aria-hidden />
                All categories
              </button>
              <p className="font-display text-sm font-semibold text-fg">{group.label}</p>
              <p className="text-xs leading-snug text-fg-muted">
                {shown ? shown.name : 'Pick a tool from the ring.'}
              </p>
            </>
          ) : (
            <>
              <p className="font-display text-lg font-semibold tracking-wide text-fg">
                {shown ? shown.name : 'CHIT'}
              </p>
              <p className="text-xs leading-snug text-fg-muted">
                {shown ? shown.detail : 'Pick a category from the ring.'}
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
