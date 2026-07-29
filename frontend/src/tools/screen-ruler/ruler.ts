export interface Point {
  x: number
  y: number
}

export interface Measurement {
  width: number
  height: number
  diagonal: number
  deviceWidth: number
  deviceHeight: number
  mmWidth: number
  mmHeight: number
  inWidth: number
  inHeight: number
}

/**
 * The CSS Values and Units specification defines one inch as exactly 96 pixels,
 * and the international inch is exactly 25.4 millimetres. Neither is this
 * monitor's real dot pitch, which is why the page says the millimetres are
 * nominal.
 */
export const CSS_PX_PER_INCH = 96
export const MM_PER_INCH = 25.4

function round(value: number, places: number): number {
  const factor = Math.pow(10, places)
  return Math.round(value * factor) / factor
}

/** Width, height and diagonal of the box between two points. */
export function measure(a: Point, b: Point, dpr: number): Measurement {
  const width = Math.abs(b.x - a.x)
  const height = Math.abs(b.y - a.y)
  const scale = Number.isFinite(dpr) && dpr > 0 ? dpr : 1

  return {
    width: round(width, 0),
    height: round(height, 0),
    diagonal: round(Math.sqrt(width * width + height * height), 0),
    deviceWidth: Math.round(width * scale),
    deviceHeight: Math.round(height * scale),
    mmWidth: round((width / CSS_PX_PER_INCH) * MM_PER_INCH, 1),
    mmHeight: round((height / CSS_PX_PER_INCH) * MM_PER_INCH, 1),
    inWidth: round(width / CSS_PX_PER_INCH, 2),
    inHeight: round(height / CSS_PX_PER_INCH, 2),
  }
}

/**
 * Flattens the shorter axis, for the Shift modifier. An exact diagonal picks
 * horizontal, which is the documented tie-break.
 */
export function lockAxis(a: Point, b: Point): Point {
  const dx = Math.abs(b.x - a.x)
  const dy = Math.abs(b.y - a.y)
  return dy > dx ? { x: a.x, y: b.y } : { x: b.x, y: a.y }
}

/** What Windows calls "Scale and layout", which is the number that settles a
 *  "everything is huge on the new monitor" call. */
export function scalingLabel(dpr: number): string {
  if (!Number.isFinite(dpr) || dpr <= 0) return 'not reported'
  const percent = Math.round(dpr * 100)
  return percent === 100 ? '100% (no scaling)' : percent + '%'
}

export function formatMm(px: number): string {
  return round((px / CSS_PX_PER_INCH) * MM_PER_INCH, 1).toFixed(1) + ' mm'
}

export function formatInches(px: number): string {
  return round(px / CSS_PX_PER_INCH, 2).toFixed(2) + ' in'
}

/** Keeps a dragged point inside the measuring area. */
export function clampToBox(p: Point, width: number, height: number): Point {
  return {
    x: Math.min(Math.max(p.x, 0), width),
    y: Math.min(Math.max(p.y, 0), height),
  }
}
