/**
 * Builds the SVG path for every dark module, merging each horizontal run into
 * one subpath so a version 10 code is a single DOM node instead of 1600.
 * Coordinates are in module units, offset by the quiet zone.
 */
export function modulesToPath(modules: boolean[], size: number, quiet: number): string {
  let path = ''
  for (let row = 0; row < size; row++) {
    let col = 0
    while (col < size) {
      if (!modules[row * size + col]) {
        col++
        continue
      }
      let run = 1
      while (col + run < size && modules[row * size + col + run]) run++
      path += `M${col + quiet} ${row + quiet}h${run}v1h-${run}z`
      col += run
    }
  }
  return path
}

const MAX_SLUG = 32

/**
 * File name for the downloaded PNG, without the extension. Non-alphanumeric
 * runs collapse to a single dash, the result is lower case and at most 32
 * characters, and an empty result becomes the fallback.
 */
export function downloadName(mode: string, label: string): string {
  const prefix = mode === 'wifi' ? 'wifi' : 'qr'
  const fallback = mode === 'wifi' ? 'network' : 'code'
  const slug = label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, MAX_SLUG)
    .replace(/-+$/, '')
  return `${prefix}-${slug === '' ? fallback : slug}`
}
