import { memo, useMemo } from 'react'
import { cn } from '../../lib/format'
import { focusRing } from '../../components'
import type { Host } from './api'
import { FREE, PENDING, USED } from './hosts'
import { numberToIP, type IPRange } from './range'

/** Above this many addresses the grid stops being readable and the table wins. */
export const MAX_GRID_ADDRESSES = 4096

const COLUMNS = 16

const CELL_TONES: Record<number, string> = {
  [PENDING]: 'border-transparent bg-surface-3 text-fg-muted/60',
  [FREE]: 'border-ok/40 bg-ok/15 text-fg-muted',
  [USED]: 'border-accent bg-accent text-accent-fg font-semibold',
}

interface Cell {
  ip: string
  octet: number
  state: number
  host: Host | undefined
  marker: string
  selected: boolean
}

export interface HostGridProps {
  range: IPRange
  states: Uint8Array
  byIP: Map<string, Host>
  selected: string | null
  onSelect: (ip: string) => void
  /** Marked so nobody hands out the address they are sitting on. */
  myIP?: string
  gateway?: string
}

/** One cell per address: in use, free, or still being checked. */
export function HostGrid({
  range,
  states,
  byIP,
  selected,
  onSelect,
  myIP,
  gateway,
}: HostGridProps) {
  // Rows start on a multiple of 16 so the last octets line up in columns the
  // way a tech reads them (.0 to .15, .16 to .31).
  const rows = useMemo(() => {
    const out: { base: number; from: number; to: number }[] = []
    for (
      let base = range.start - (range.start % COLUMNS);
      base <= range.end;
      base += COLUMNS
    ) {
      out.push({
        base,
        from: Math.max(base, range.start),
        to: Math.min(base + COLUMNS - 1, range.end),
      })
    }
    return out
  }, [range])

  return (
    <div className="flex flex-col gap-2">
      <div className="max-w-[46rem] min-w-0 overflow-x-auto">
        <div className="flex min-w-[26rem] flex-col gap-1">
          {rows.map((row) => {
            const cells: Cell[] = []
            for (let addr = row.from; addr <= row.to; addr++) {
              const ip = numberToIP(addr)
              cells.push({
                ip,
                octet: addr & 255,
                state: states[addr - range.start] ?? PENDING,
                host: byIP.get(ip),
                marker: ip === myIP ? 'this computer' : ip === gateway ? 'gateway' : '',
                selected: ip === selected,
              })
            }
            return (
              <GridRow
                key={row.base}
                label={numberToIP(row.base)}
                pad={row.from - row.base}
                cells={cells}
                signature={signature(cells)}
                onSelect={onSelect}
              />
            )
          })}
        </div>
      </div>
      <Legend />
    </div>
  )
}

interface GridRowProps {
  label: string
  pad: number
  cells: Cell[]
  signature: string
  onSelect: (ip: string) => void
}

/**
 * Rows are compared by a signature rather than by their cells, so a batch of
 * results only re-renders the handful of rows it actually changed. A /24
 * arrives in ten batches a second and must not stutter.
 */
const GridRow = memo(
  function GridRow({ label, pad, cells, onSelect }: GridRowProps) {
    return (
      <div className="flex items-center gap-2">
        <span className="w-24 shrink-0 truncate text-right text-[11px] text-fg-muted tabular-nums">
          {label}
        </span>
        <div
          className="grid min-w-0 flex-1 gap-1"
          style={{ gridTemplateColumns: `repeat(${COLUMNS}, minmax(0, 1fr))` }}
        >
          {Array.from({ length: pad }, (_, i) => (
            <div key={`pad-${i}`} aria-hidden />
          ))}
          {cells.map((cell) => (
            <button
              key={cell.ip}
              type="button"
              onClick={() => onSelect(cell.ip)}
              title={describe(cell)}
              aria-label={describe(cell)}
              aria-pressed={cell.selected}
              className={cn(
                'relative h-7 rounded border text-[11px] tabular-nums transition-colors hover:brightness-110',
                focusRing,
                CELL_TONES[cell.state],
                cell.selected && 'ring-2 ring-fg ring-offset-0',
              )}
            >
              {cell.octet}
              {cell.marker !== '' && (
                <span
                  aria-hidden
                  className="absolute top-0.5 right-0.5 size-1.5 rounded-full bg-warn"
                />
              )}
            </button>
          ))}
        </div>
      </div>
    )
  },
  (prev, next) => prev.signature === next.signature && prev.onSelect === next.onSelect,
)

function Legend() {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-fg-muted">
      <LegendItem state={USED} label="In use" />
      <LegendItem state={FREE} label="Free" />
      <LegendItem state={PENDING} label="Not checked yet" />
      <span className="inline-flex items-center gap-1.5">
        <span className="size-2 shrink-0 rounded-full bg-warn" aria-hidden />
        This computer or the gateway
      </span>
    </div>
  )
}

function LegendItem({ state, label }: { state: number; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <StateSwatch state={state} />
      {label}
    </span>
  )
}

/** The same colours as the grid, for the legend and the table's status column. */
export function StateSwatch({ state }: { state: number }) {
  return (
    <span
      aria-hidden
      className={cn('size-3 shrink-0 rounded-sm border', CELL_TONES[state])}
    />
  )
}

function describe(cell: Cell): string {
  const parts = [cell.ip]
  if (cell.marker !== '') parts.push(cell.marker)
  parts.push(cell.state === USED ? 'in use' : cell.state === FREE ? 'free' : 'not checked yet')
  const host = cell.host
  if (host !== undefined && host.status === 'up') {
    if (host.hostname !== '') parts.push(host.hostname)
    if (host.vendor !== '') parts.push(host.vendor)
    else if (host.mac !== '') parts.push(host.mac)
  }
  return parts.join(', ')
}

// Everything a cell shows comes from these fields, so a row whose signature is
// unchanged has nothing new to draw.
function signature(cells: Cell[]): string {
  let out = ''
  for (const cell of cells) {
    out += `${cell.state}${cell.selected ? 's' : ''}${cell.host?.respondedVia ?? ''}${
      cell.host?.hostname ?? ''
    }${cell.host?.vendor ?? cell.host?.mac ?? ''}|`
  }
  return out
}
