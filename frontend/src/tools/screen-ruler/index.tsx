import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Grid3x3, Image as ImageIcon, Palette, Ruler, Trash2 } from 'lucide-react'
import { Button, CopyButton, TextInput, ToolShell } from '../../components'
import { cn } from '../../lib/format'
import {
  BLACK,
  WHITE,
  contrastRatio,
  formatRatio,
  overWhite,
  parseColor,
  passes,
  toHex,
  toHslString,
  toRgbString,
  type RGB,
} from './color'
import { clampToBox, formatInches, formatMm, lockAxis, measure, scalingLabel, type Point } from './ruler'

const AREA_HEIGHT = 420
const RECENT_MAX = 8
const CANVAS_MAX_WIDTH = 720

export default function ScreenRulerPage() {
  const [tab, setTab] = useState<'ruler' | 'colour'>('ruler')

  return (
    <ToolShell
      title="Screen Ruler and Color Picker"
      description="Measure in pixels, read this display's real resolution and scaling, and pick colours out of an image."
      help={
        <>
          <p>
            The ruler measures inside this window. Drag across the measuring area and it gives you
            the size in CSS pixels, in the real pixels your display actually has, and in
            millimetres. Hold Shift while dragging to keep the line straight, and use the arrow keys
            to nudge the end by exactly one pixel.
          </p>
          <p className="mt-1.5">
            The display panel underneath is usually the more useful half. When somebody says
            "everything is huge on the new monitor" or "text looks blurry", the answer is almost
            always the Scaling figure: Windows calls it "Scale and layout", macOS calls it a scaled
            resolution, and an app that ignores it renders at the wrong size. Read those numbers out
            and you have settled it.
          </p>
          <p className="mt-1.5">
            The colour picker works on an image, not on the screen. No operating system lets an app
            read pixels outside its own window without a screen-recording permission, so CHIT does
            not pretend it can. Take a screenshot with your operating system (Win+Shift+S on
            Windows, Cmd+Shift+4 on a Mac), paste it in with Ctrl+V, and click the colour you want.
            The contrast figures underneath tell you whether text in that colour is readable, using
            the same thresholds accessibility audits use.
          </p>
        </>
      }
      actions={
        <div className="flex items-center gap-1 rounded border border-border bg-surface-2 p-0.5">
          <Button
            size="sm"
            variant={tab === 'ruler' ? 'primary' : 'ghost'}
            onClick={() => setTab('ruler')}
            icon={<Ruler size={14} aria-hidden />}
            aria-pressed={tab === 'ruler'}
          >
            Ruler
          </Button>
          <Button
            size="sm"
            variant={tab === 'colour' ? 'primary' : 'ghost'}
            onClick={() => setTab('colour')}
            icon={<Palette size={14} aria-hidden />}
            aria-pressed={tab === 'colour'}
          >
            Colour
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <p className="rounded border border-border bg-surface-2 px-3 py-2 text-xs text-fg-muted">
          Operating systems do not let an app read pixels outside its own window, so this measures
          and picks inside CHIT and inside an image you open. To sample a colour from somewhere
          else, take a screenshot with your operating system and paste it in with Ctrl+V.
        </p>

        {tab === 'ruler' ? <RulerTab /> : <ColourTab active={tab === 'colour'} />}
      </div>
    </ToolShell>
  )
}

function RulerTab() {
  const areaRef = useRef<HTMLDivElement>(null)
  const [from, setFrom] = useState<Point | null>(null)
  const [to, setTo] = useState<Point | null>(null)
  const [dragging, setDragging] = useState(false)
  const [grid, setGrid] = useState(false)
  const [display, setDisplay] = useState(readDisplay())

  useEffect(() => {
    const onResize = () => setDisplay(readDisplay())
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  const pointIn = (event: React.PointerEvent): Point => {
    const box = event.currentTarget.getBoundingClientRect()
    return clampToBox(
      { x: Math.round(event.clientX - box.left), y: Math.round(event.clientY - box.top) },
      Math.round(box.width),
      Math.round(box.height),
    )
  }

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (to === null || from === null) return
    const step = event.shiftKey ? 10 : 1
    const moves: Record<string, Point> = {
      ArrowLeft: { x: -step, y: 0 },
      ArrowRight: { x: step, y: 0 },
      ArrowUp: { x: 0, y: -step },
      ArrowDown: { x: 0, y: step },
    }
    const move = moves[event.key]
    if (move) {
      event.preventDefault()
      setTo({ x: to.x + move.x, y: to.y + move.y })
      return
    }
    if (event.key === 'Escape') {
      setFrom(null)
      setTo(null)
    }
  }

  const m = from !== null && to !== null ? measure(from, to, display.dpr) : null

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant={grid ? 'primary' : 'secondary'}
          aria-pressed={grid}
          onClick={() => setGrid((on) => !on)}
          icon={<Grid3x3 size={14} aria-hidden />}
        >
          Grid
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            setFrom(null)
            setTo(null)
          }}
          icon={<Trash2 size={14} aria-hidden />}
        >
          Clear
        </Button>
        <span className="text-xs text-fg-muted">
          Drag to measure. Hold Shift to keep it straight, arrow keys to nudge, Escape to clear.
        </span>
      </div>

      <div
        ref={areaRef}
        tabIndex={0}
        role="application"
        aria-label="Measuring area"
        onKeyDown={onKeyDown}
        onPointerDown={(event) => {
          event.currentTarget.setPointerCapture(event.pointerId)
          const point = pointIn(event)
          setFrom(point)
          setTo(point)
          setDragging(true)
        }}
        onPointerMove={(event) => {
          if (!dragging || from === null) return
          const point = pointIn(event)
          setTo(event.shiftKey ? lockAxis(from, point) : point)
        }}
        onPointerUp={() => setDragging(false)}
        onPointerCancel={() => setDragging(false)}
        className={cn(
          'relative w-full cursor-crosshair touch-none rounded border border-border bg-surface-2 select-none',
          'focus-visible:border-accent focus-visible:outline-none',
        )}
        style={{
          height: AREA_HEIGHT,
          backgroundImage: grid
            ? 'repeating-linear-gradient(to right, var(--border) 0 1px, transparent 1px 10px),' +
              'repeating-linear-gradient(to bottom, var(--border) 0 1px, transparent 1px 10px)'
            : undefined,
        }}
      >
        {from !== null && to !== null && (
          <svg className="pointer-events-none absolute inset-0 h-full w-full text-accent">
            <rect
              x={Math.min(from.x, to.x)}
              y={Math.min(from.y, to.y)}
              width={Math.abs(to.x - from.x)}
              height={Math.abs(to.y - from.y)}
              fill="none"
              stroke="currentColor"
              strokeDasharray="4 3"
              opacity={0.5}
            />
            <line
              x1={from.x}
              y1={from.y}
              x2={to.x}
              y2={to.y}
              stroke="currentColor"
              strokeWidth={2}
            />
            <circle cx={from.x} cy={from.y} r={4} fill="currentColor" />
            <circle cx={to.x} cy={to.y} r={4} fill="currentColor" />
          </svg>
        )}
      </div>

      <div className="rounded border border-border bg-surface-2 px-3 py-2">
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <Row label="Width" value={`${m?.width ?? 0} px`} mono />
          <Row label="Height" value={`${m?.height ?? 0} px`} mono />
          <Row label="Diagonal" value={`${m?.diagonal ?? 0} px`} mono />
          <Row
            label="Device pixels"
            value={`${m?.deviceWidth ?? 0} x ${m?.deviceHeight ?? 0}`}
            mono
          />
          <Row
            label="At 96 dpi"
            value={`${formatMm(m?.width ?? 0)} x ${formatMm(m?.height ?? 0)} (${formatInches(m?.width ?? 0)} x ${formatInches(m?.height ?? 0)})`}
            mono
          />
        </dl>
        <p className="mt-1.5 text-xs text-fg-muted">
          Millimetres assume the CSS standard of 96 pixels per inch. Your monitor's real dots per
          inch will differ, so treat these as nominal.
        </p>
      </div>

      <div className="rounded border border-border bg-surface-2 px-3 py-2">
        <h2 className="mb-1.5 text-xs font-semibold tracking-wide text-fg-muted uppercase">
          This display
        </h2>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <Fact
            label="Screen resolution"
            value={`${display.width} x ${display.height}`}
            note="What the operating system reports, before scaling."
          />
          <Fact
            label="Usable area"
            value={`${display.availWidth} x ${display.availHeight}`}
            note="The screen minus the taskbar or dock."
          />
          <Fact
            label="CHIT window"
            value={`${display.windowWidth} x ${display.windowHeight}`}
            note="The size of this window in CSS pixels."
          />
          <Fact
            label="Scaling"
            value={scalingLabel(display.dpr)}
            note='Windows calls this "Scale and layout". Text looks blurry when an app does not follow it.'
          />
          <Fact
            label="Real pixels"
            value={`${Math.round(display.width * display.dpr)} x ${Math.round(display.height * display.dpr)}`}
            note="Resolution times scaling: the actual pixels on the panel."
          />
          <Fact
            label="Colour depth"
            value={`${display.colorDepth}-bit`}
            note="How many colours the display can show."
          />
        </dl>
      </div>
    </>
  )
}

function ColourTab({ active }: { active: boolean }) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [colour, setColour] = useState<RGB>({ r: 30, g: 144, b: 255 })
  const [text, setText] = useState('#1e90ff')
  const [textError, setTextError] = useState<string | null>(null)
  const [recent, setRecent] = useState<string[]>([])
  const [imageError, setImageError] = useState<string | null>(null)
  const [transparent, setTransparent] = useState(false)
  const [hasImage, setHasImage] = useState(false)

  const remember = useCallback((next: RGB) => {
    setColour(next)
    setText(toHex(next))
    setTextError(null)
    setRecent((all) => [toHex(next), ...all.filter((hex) => hex !== toHex(next))].slice(0, RECENT_MAX))
  }, [])

  const drawImage = useCallback((source: Blob) => {
    const url = URL.createObjectURL(source)
    const img = new Image()
    img.onload = () => {
      const canvas = canvasRef.current
      if (canvas) {
        const scale = Math.min(1, CANVAS_MAX_WIDTH / img.naturalWidth)
        canvas.width = Math.max(1, Math.round(img.naturalWidth * scale))
        canvas.height = Math.max(1, Math.round(img.naturalHeight * scale))
        const ctx = canvas.getContext('2d')
        ctx?.drawImage(img, 0, 0, canvas.width, canvas.height)
        setHasImage(true)
        setImageError(null)
      }
      URL.revokeObjectURL(url)
    }
    img.onerror = () => {
      setImageError('That file could not be read as an image. PNG, JPEG, GIF and WebP all work.')
      URL.revokeObjectURL(url)
    }
    img.src = url
  }, [])

  useEffect(() => {
    if (!active) return
    const onPaste = (event: ClipboardEvent) => {
      for (const item of event.clipboardData?.items ?? []) {
        if (!item.type.startsWith('image/')) continue
        const file = item.getAsFile()
        if (file) drawImage(file)
        return
      }
    }
    window.addEventListener('paste', onPaste)
    return () => window.removeEventListener('paste', onPaste)
  }, [active, drawImage])

  const sample = (event: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current
    if (!canvas || !hasImage) return
    const box = canvas.getBoundingClientRect()
    const x = Math.floor(((event.clientX - box.left) / box.width) * canvas.width)
    const y = Math.floor(((event.clientY - box.top) / box.height) * canvas.height)
    const data = canvas.getContext('2d')?.getImageData(x, y, 1, 1).data
    if (!data) return

    const alpha = data[3] / 255
    setTransparent(alpha < 1)
    remember(alpha < 1 ? overWhite({ r: data[0], g: data[1], b: data[2] }, alpha) : { r: data[0], g: data[1], b: data[2] })
  }

  const onText = (next: string) => {
    setText(next)
    const parsed = parseColor(next)
    if (parsed === null) {
      setTextError(
        'That is not a colour CHIT recognises. Try a hex value like #1e90ff, an rgb(30, 144, 255) or an hsl(210, 100%, 56%).',
      )
      return
    }
    setTextError(null)
    setColour(parsed)
  }

  const onWhite = useMemo(() => contrastRatio(colour, WHITE), [colour])
  const onBlack = useMemo(() => contrastRatio(colour, BLACK), [colour])

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <label className="inline-flex">
          <span className="sr-only">Open an image</span>
          <input
            type="file"
            accept="image/*"
            className="sr-only"
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) drawImage(file)
            }}
          />
          <span className="inline-flex cursor-pointer items-center gap-1.5 rounded border border-border bg-surface-2 px-3 py-1.5 text-sm text-fg hover:bg-surface-3">
            <ImageIcon size={14} aria-hidden />
            Open an image
          </span>
        </label>
        <span className="text-xs text-fg-muted">
          or paste a screenshot with Ctrl+V, or drop an image here
        </span>
      </div>

      {imageError !== null && (
        <p role="alert" className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg">
          {imageError}
        </p>
      )}

      <div
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          // Without this the webview navigates away from the app entirely.
          event.preventDefault()
          const file = event.dataTransfer.files?.[0]
          if (file) drawImage(file)
        }}
        className="rounded border border-border bg-surface-2 p-2"
      >
        {!hasImage && (
          <p className="px-3 py-8 text-center text-sm text-fg-muted">
            Open, paste or drop an image to pick a colour out of it.
          </p>
        )}
        <canvas
          ref={canvasRef}
          onClick={sample}
          className={cn('max-w-full cursor-crosshair rounded', !hasImage && 'hidden')}
        />
      </div>

      <div className="rounded border border-border bg-surface-2 px-3 py-2">
        <div className="flex flex-wrap items-center gap-3">
          <span
            className="size-16 shrink-0 rounded border border-border"
            style={{ backgroundColor: toHex(colour) }}
            aria-hidden
          />
          <dl className="grid grid-cols-[auto_1fr_auto] items-center gap-x-3 gap-y-1 text-sm">
            <Swatch label="Hex" value={toHex(colour)} />
            <Swatch label="RGB" value={toRgbString(colour)} />
            <Swatch label="HSL" value={toHslString(colour)} />
          </dl>
        </div>

        {transparent && (
          <p className="mt-2 text-xs text-warn">
            That pixel is partly transparent, so the colour shown is what it looks like over white.
          </p>
        )}

        <h3 className="mt-3 mb-1 text-xs font-semibold tracking-wide text-fg-muted uppercase">
          Contrast
        </h3>
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-fg-muted">
              <th className="py-1 pr-3 font-medium">Against</th>
              <th className="py-1 pr-3 font-medium">Ratio</th>
              <th className="py-1 pr-3 font-medium">AA normal</th>
              <th className="py-1 pr-3 font-medium">AA large</th>
              <th className="py-1 font-medium">AAA normal</th>
            </tr>
          </thead>
          <tbody>
            <ContrastRow label="White" ratio={onWhite} />
            <ContrastRow label="Black" ratio={onBlack} />
          </tbody>
        </table>
      </div>

      <div className="max-w-md">
        <TextInput
          label="Colour"
          value={text}
          onChange={(event) => onText(event.target.value)}
          spellCheck={false}
          autoComplete="off"
          error={textError ?? undefined}
          hint="Hex, rgb() or hsl(). For example #1e90ff, rgb(30, 144, 255) or hsl(210, 100%, 56%)."
        />
      </div>

      {recent.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-fg-muted">Recent</span>
          {recent.map((hex) => (
            <button
              key={hex}
              type="button"
              aria-label={`Use ${hex}`}
              title={hex}
              onClick={() => {
                const parsed = parseColor(hex)
                if (parsed) remember(parsed)
              }}
              className="size-7 rounded border border-border"
              style={{ backgroundColor: hex }}
            />
          ))}
        </div>
      )}
    </>
  )
}

function ContrastRow({ label, ratio }: { label: string; ratio: number }) {
  return (
    <tr className="border-t border-border">
      <td className="py-1 pr-3 text-fg">{label}</td>
      <td className="py-1 pr-3 font-mono text-fg tabular-nums">{formatRatio(ratio)}</td>
      <td className="py-1 pr-3">
        <Verdict ok={passes(ratio, 'aa')} />
      </td>
      <td className="py-1 pr-3">
        <Verdict ok={passes(ratio, 'aaLarge')} />
      </td>
      <td className="py-1">
        <Verdict ok={passes(ratio, 'aaa')} />
      </td>
    </tr>
  )
}

// The word is the signal, not the colour.
function Verdict({ ok }: { ok: boolean }) {
  return <span className={ok ? 'text-ok' : 'text-danger'}>{ok ? 'Pass' : 'Fail'}</span>
}

function Swatch({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className="font-mono text-fg">{value}</dd>
      <CopyButton value={value} />
    </>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className={cn('text-fg', mono && 'font-mono tabular-nums')}>{value}</dd>
    </>
  )
}

function Fact({ label, value, note }: { label: string; value: string; note: string }) {
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className="text-fg">
        <span className="font-mono tabular-nums">{value}</span>
        <span className="ml-2 text-xs text-fg-muted">{note}</span>
      </dd>
    </>
  )
}

interface DisplayFacts {
  width: number
  height: number
  availWidth: number
  availHeight: number
  windowWidth: number
  windowHeight: number
  dpr: number
  colorDepth: number
}

function readDisplay(): DisplayFacts {
  return {
    width: window.screen?.width ?? 0,
    height: window.screen?.height ?? 0,
    availWidth: window.screen?.availWidth ?? 0,
    availHeight: window.screen?.availHeight ?? 0,
    windowWidth: window.innerWidth,
    windowHeight: window.innerHeight,
    dpr: window.devicePixelRatio,
    colorDepth: window.screen?.colorDepth ?? 0,
  }
}
