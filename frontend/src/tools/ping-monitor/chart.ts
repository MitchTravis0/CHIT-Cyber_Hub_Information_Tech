// The latency graph is hand drawn, so its geometry lives here as pure functions
// the tests can pin down.

/** SVG user units. The chart is drawn once at this size and scaled by CSS. */
export const PLOT = {
  width: 640,
  height: 160,
  left: 34,
  right: 8,
  top: 8,
  bottom: 132,
  dropY: 138,
  dropH: 6,
} as const

/** Series colours, theme variables so both light and dark mode work. */
export const SERIES = ['var(--accent)', 'var(--ok)', 'var(--warn)', 'var(--fg-muted)'] as const

/** Dash patterns, so the lines are told apart without relying on colour. */
export const DASHES = ['', '5 3', '2 3', '8 2 2 2'] as const

const NICE = [5, 10, 20, 25, 50, 75, 100, 150, 200, 250, 500, 750, 1000, 1500, 2000, 3000, 5000]

/** A round upper bound for the y axis, plus the three tick values [0, max/2, max]. */
export function chartScale(values: number[]): { max: number; ticks: number[] } {
  let peak = 0
  for (const value of values) {
    if (value > peak) peak = value
  }
  if (peak <= 0) return { max: 10, ticks: [0, 5, 10] }

  const nice = NICE.find((step) => step >= peak)
  const max = nice ?? Math.ceil(peak / 1000) * 1000
  return { max, ticks: [0, max / 2, max] }
}

export function pointX(index: number, count: number): number {
  if (count <= 1) return PLOT.left
  return PLOT.left + (index * (PLOT.width - PLOT.left - PLOT.right)) / (count - 1)
}

export function pointY(value: number, max: number): number {
  return PLOT.bottom - (Math.min(value, max) / max) * (PLOT.bottom - PLOT.top)
}

/** Runs of consecutive answered samples as [x, y] pairs, so a lost ping breaks
 *  the line instead of being drawn through. */
export function segments(
  samples: Array<{ ok: boolean; latencyMs: number }>,
  max: number,
): Array<Array<[number, number]>> {
  const runs: Array<Array<[number, number]>> = []
  let run: Array<[number, number]> = []

  samples.forEach((sample, index) => {
    if (!sample.ok) {
      if (run.length > 0) runs.push(run)
      run = []
      return
    }
    run.push([pointX(index, samples.length), pointY(sample.latencyMs, max)])
  })
  if (run.length > 0) runs.push(run)
  return runs
}
