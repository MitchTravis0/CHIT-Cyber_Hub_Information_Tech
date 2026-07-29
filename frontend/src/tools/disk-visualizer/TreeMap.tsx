import { useMemo } from 'react'
import { cn, formatBytes } from '../../lib/format'
import type { Entry } from './api'
import { sharePct, squarify } from './treemap'

// Six steps of the accent colour. Colour is never the only signal: every tile
// big enough to hold text also carries its name and size, and every tile has an
// aria-label saying the same thing.
const SHADES = [
  'bg-accent',
  'bg-accent/80',
  'bg-accent/65',
  'bg-accent/50',
  'bg-accent/35',
  'bg-accent/20',
]

// The layout runs in a fixed coordinate space and is placed with percentages,
// so nothing has to measure the DOM or watch for a resize.
const SPACE = { x: 0, y: 0, w: 100, h: 62 }

// A tile narrower or shorter than this cannot hold a readable label.
const LABEL_MIN = 4

interface TreeMapProps {
  entries: Entry[]
  total: number
  onOpen: (entry: Entry) => void
}

export function TreeMap({ entries, total, onOpen }: TreeMapProps) {
  const tiles = useMemo(
    () => squarify(entries, (entry) => entry.bytes, SPACE).filter((tile) => tile.rect.w > 0),
    [entries],
  )

  if (tiles.length === 0) return null

  return (
    <div
      className="relative w-full overflow-hidden rounded border border-border bg-surface-2"
      style={{ aspectRatio: `${SPACE.w} / ${SPACE.h}` }}
    >
      {tiles.map((tile, index) => {
        const share = sharePct(tile.item.bytes, total)
        const roomy = tile.rect.w >= LABEL_MIN && tile.rect.h >= LABEL_MIN
        return (
          <button
            key={tile.item.path}
            type="button"
            onClick={() => onOpen(tile.item)}
            title={`${tile.item.path}\n${formatBytes(tile.item.bytes)}, ${tile.item.files} files`}
            aria-label={`${tile.item.name}, ${formatBytes(tile.item.bytes)}, ${share} percent`}
            className={cn(
              'absolute overflow-hidden border border-surface-2 px-1 py-0.5 text-left text-accent-fg',
              'hover:border-fg focus-visible:border-fg focus-visible:outline-none',
              SHADES[Math.min(index, SHADES.length - 1)],
            )}
            style={{
              left: `${(tile.rect.x / SPACE.w) * 100}%`,
              top: `${(tile.rect.y / SPACE.h) * 100}%`,
              width: `${(tile.rect.w / SPACE.w) * 100}%`,
              height: `${(tile.rect.h / SPACE.h) * 100}%`,
            }}
          >
            {roomy && (
              <span className="block text-[11px] leading-tight font-medium break-all">
                {tile.item.name}
                <span className="block font-normal opacity-80">
                  {formatBytes(tile.item.bytes)}
                </span>
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
