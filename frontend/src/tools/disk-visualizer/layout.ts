export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface Tile<T> {
  item: T
  rect: Rect
}

/**
 * The worst (largest) aspect ratio in a row of areas laid across a side of the
 * given length. Squarified treemaps grow a row while this keeps falling.
 * From Bruls, Huizing and van Wijk, "Squarified Treemaps" (2000).
 */
function worstRatio(areas: number[], sum: number, side: number): number {
  if (sum <= 0 || side <= 0 || areas.length === 0) return Infinity
  let max = areas[0]
  let min = areas[0]
  for (const a of areas) {
    if (a > max) max = a
    if (a < min) min = a
  }
  if (min <= 0) return Infinity
  const s2 = sum * sum
  const w2 = side * side
  return Math.max((w2 * max) / s2, s2 / (w2 * min))
}

/**
 * Squarified treemap layout. Items must be sorted largest first and sizes must
 * be non-negative. Every tile lies inside `rect`, no two overlap, and together
 * they cover the whole rectangle.
 *
 * Items of size zero get a zero-area tile at the rectangle's origin and take
 * part in no row, because a zero area has no aspect ratio and would make the
 * row-growing test divide by zero.
 */
export function squarify<T>(items: T[], size: (item: T) => number, rect: Rect): Tile<T>[] {
  if (items.length === 0) return []

  const empty: Rect = { x: rect.x, y: rect.y, w: 0, h: 0 }
  const values = items.map((item) => Math.max(0, size(item)))
  const live: number[] = []
  for (let i = 0; i < items.length; i++) {
    if (values[i] > 0) live.push(i)
  }
  if (live.length === 0 || rect.w <= 0 || rect.h <= 0) {
    return items.map((item) => ({ item, rect: empty }))
  }

  const total = live.reduce((sum, i) => sum + values[i], 0)
  // Scale byte counts into area units so a row's area and the rectangle's area
  // are in the same currency.
  const scale = (rect.w * rect.h) / total

  const placed = new Map<number, Rect>()
  let free: Rect = { ...rect }
  let cursor = 0

  while (cursor < live.length) {
    const shorter = Math.min(free.w, free.h)
    const areas: number[] = []
    let rowSum = 0
    let best = Infinity
    let end = cursor

    while (end < live.length) {
      const area = values[live[end]] * scale
      const ratio = worstRatio([...areas, area], rowSum + area, shorter)
      if (end > cursor && ratio > best) break
      areas.push(area)
      rowSum += area
      best = ratio
      end++
    }

    const horizontal = free.w >= free.h
    // The final row takes whatever is left, so rounding cannot leave a sliver
    // of the rectangle uncovered.
    const thickness =
      end >= live.length ? (horizontal ? free.w : free.h) : shorter > 0 ? rowSum / shorter : 0

    let offset = 0
    for (let k = cursor; k < end; k++) {
      const share = rowSum > 0 ? areas[k - cursor] / rowSum : 0
      const length = k === end - 1 ? shorter - offset : share * shorter
      placed.set(
        live[k],
        horizontal
          ? { x: free.x, y: free.y + offset, w: thickness, h: length }
          : { x: free.x + offset, y: free.y, w: length, h: thickness },
      )
      offset += length
    }

    free = horizontal
      ? { x: free.x + thickness, y: free.y, w: free.w - thickness, h: free.h }
      : { x: free.x, y: free.y + thickness, w: free.w, h: free.h - thickness }
    cursor = end
  }

  return items.map((item, i) => ({ item, rect: placed.get(i) ?? empty }))
}

/** A child's share of the scanned total, to one decimal place. */
export function sharePct(bytes: number, total: number): number {
  if (total <= 0) return 0
  return Math.round((bytes / total) * 1000) / 10
}

/**
 * Every folder from the root down to `path`, so the breadcrumb can be built
 * from the current path alone rather than from a history stack.
 */
export function crumbs(path: string): string[] {
  if (path === '') return []

  const windows = /^[A-Za-z]:[\\/]/.test(path)
  if (windows) {
    const normalised = path.replace(/\//g, '\\')
    const root = normalised.slice(0, 3)
    const out = [root]
    let current = root
    for (const part of normalised.slice(3).split('\\')) {
      if (part === '') continue
      current = current.endsWith('\\') ? current + part : current + '\\' + part
      out.push(current)
    }
    return out
  }

  const out = ['/']
  let current = ''
  for (const part of path.split('/')) {
    if (part === '') continue
    current += '/' + part
    out.push(current)
  }
  return out
}

/** The label shown on one breadcrumb button. */
export function crumbLabel(path: string): string {
  const parts = path.split(/[\\/]/).filter((part) => part !== '')
  return parts.length === 0 ? path : parts[parts.length - 1]
}

/** The exported file's base name, safe on every filesystem. */
export function csvBase(path: string): string {
  const parts = path.split(/[\\/]/).filter((part) => part !== '')
  const name = parts.length === 0 ? 'root' : parts[parts.length - 1]
  return 'disk-' + name.replace(/[^A-Za-z0-9-]/g, '-')
}
