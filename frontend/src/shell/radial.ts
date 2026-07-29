/**
 * Ring geometry for the radial navigator. Pure maths, no React and no DOM, so
 * the awkward part (angles, arc flags, the single-segment case) is pinned by
 * tests before any of it reaches a screen.
 *
 * Convention: 0 degrees points straight up and angles increase clockwise, which
 * is how a ring is read on screen. SVG's y axis grows downward, hence the minus
 * on the cosine.
 */

export interface Point {
  x: number
  y: number
}

export interface RingSegment {
  startDeg: number
  endDeg: number
  /** Middle of the segment, where its icon and label sit. */
  midDeg: number
}

export interface RingOptions {
  /** Angle the first segment starts at. */
  startDeg?: number
  /** Blank angle left between neighbouring segments. */
  gapDeg?: number
}

/** A full circle is capped just short of 360, because an arc that ends exactly
 *  where it starts draws nothing at all. */
const FULL_CIRCLE = 359.99

function round2(value: number): number {
  const rounded = Math.round(value * 100) / 100
  // Math.round can hand back -0, which is not deep-equal to 0.
  return rounded === 0 ? 0 : rounded
}

export function polarToXY(cx: number, cy: number, r: number, angleDeg: number): Point {
  const radians = (angleDeg * Math.PI) / 180
  return {
    x: round2(cx + r * Math.sin(radians)),
    y: round2(cy - r * Math.cos(radians)),
  }
}

/** Divides the circle into `count` segments, each shortened by `gapDeg`. */
export function layoutRing(count: number, options: RingOptions = {}): RingSegment[] {
  if (count <= 0) return []

  const { startDeg = 0, gapDeg = 2 } = options

  if (count === 1) {
    const endDeg = round2(startDeg + FULL_CIRCLE)
    return [{ startDeg: round2(startDeg), endDeg, midDeg: round2((startDeg + endDeg) / 2) }]
  }

  const slice = 360 / count
  return Array.from({ length: count }, (_, index) => {
    const start = startDeg + index * slice
    const end = start + slice - gapDeg
    return {
      startDeg: round2(start),
      endDeg: round2(end),
      midDeg: round2((start + end) / 2),
    }
  })
}

/** The `d` attribute for one donut segment: outer arc clockwise, inner arc back. */
export function arcPath(
  cx: number,
  cy: number,
  rInner: number,
  rOuter: number,
  startDeg: number,
  endDeg: number,
): string {
  const outerStart = polarToXY(cx, cy, rOuter, startDeg)
  const outerEnd = polarToXY(cx, cy, rOuter, endDeg)
  const innerEnd = polarToXY(cx, cy, rInner, endDeg)
  const innerStart = polarToXY(cx, cy, rInner, startDeg)
  const largeArc = endDeg - startDeg > 180 ? 1 : 0

  return [
    `M ${outerStart.x} ${outerStart.y}`,
    `A ${rOuter} ${rOuter} 0 ${largeArc} 1 ${outerEnd.x} ${outerEnd.y}`,
    `L ${innerEnd.x} ${innerEnd.y}`,
    `A ${rInner} ${rInner} 0 ${largeArc} 0 ${innerStart.x} ${innerStart.y}`,
    'Z',
  ].join(' ')
}
