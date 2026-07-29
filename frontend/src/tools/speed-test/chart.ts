// The speed graph is a plain inline <svg>, so the arithmetic behind it lives
// here where it can be tested without a DOM.

/** Rounds a peak up to a friendly axis top: 1, 2 or 5 times a power of ten, never below 10. */
export function niceMax(peak: number): number {
  if (!(peak > 10)) return 10
  const power = Math.pow(10, Math.floor(Math.log10(peak)))
  for (const step of [1, 2, 5]) {
    if (step * power >= peak) return step * power
  }
  return 10 * power
}

/** SVG polyline points for values scaled into a width x height box, "x,y" pairs
 *  separated by spaces, two decimal places. Empty string for an empty array. */
export function polylinePoints(
  values: number[],
  width: number,
  height: number,
  max: number,
): string {
  return values
    .map((value, i) => {
      const x = values.length < 2 ? 0 : (i / (values.length - 1)) * width
      const y = height - Math.min(1, Math.max(0, value / max)) * height
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')
}

/** 0.5 -> "0.50", 8.25 -> "8.25", 94.7 -> "94.7", 943 -> "943". */
export function formatMbps(v: number): string {
  if (v < 10) return v.toFixed(2)
  if (v < 100) return v.toFixed(1)
  return v.toFixed(0)
}

/** Megabits per second to megabytes per second, the number people see in a browser download. */
export function toMBps(mbps: number): number {
  return mbps / 8
}
