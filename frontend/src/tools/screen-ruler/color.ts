export interface RGB {
  r: number
  g: number
  b: number
}

export interface HSL {
  h: number
  s: number
  l: number
}

/**
 * WCAG 2.1 relative luminance. Every number here comes from the published
 * definition, not from anywhere in this file, and each was checked against a
 * python3 implementation of the same formula before the tests were written.
 */
const LUMINANCE_R = 0.2126
const LUMINANCE_G = 0.7152
const LUMINANCE_B = 0.0722
const SRGB_THRESHOLD = 0.04045
const SRGB_LOW_DIVISOR = 12.92
const SRGB_OFFSET = 0.055
const SRGB_DIVISOR = 1.055
const SRGB_EXPONENT = 2.4

/** WCAG 2.1 success criteria 1.4.3 and 1.4.6. */
export const AA_NORMAL = 4.5
export const AA_LARGE = 3
export const AAA_NORMAL = 7

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value))
}

function clampChannel(value: number): number {
  return Math.round(clamp(value, 0, 255))
}

/** Accepts #rgb, #rrggbb, #rrggbbaa, rgb(), rgba(), hsl() and hsla(). */
export function parseColor(text: string): RGB | null {
  const trimmed = text.trim().toLowerCase()
  if (trimmed === '') return null

  const hex = trimmed.startsWith('#') ? trimmed.slice(1) : trimmed
  if (/^[0-9a-f]+$/.test(hex)) {
    if (hex.length === 3) {
      return {
        r: parseInt(hex[0] + hex[0], 16),
        g: parseInt(hex[1] + hex[1], 16),
        b: parseInt(hex[2] + hex[2], 16),
      }
    }
    // An 8-digit hex carries an alpha channel, which is ignored: this tool
    // reports the colour, not the transparency.
    if (hex.length === 6 || hex.length === 8) {
      return {
        r: parseInt(hex.slice(0, 2), 16),
        g: parseInt(hex.slice(2, 4), 16),
        b: parseInt(hex.slice(4, 6), 16),
      }
    }
    return null
  }

  const rgb = /^rgba?\(([^)]*)\)$/.exec(trimmed)
  if (rgb) {
    const parts = numbersIn(rgb[1])
    if (parts.length < 3) return null
    return { r: clampChannel(parts[0]), g: clampChannel(parts[1]), b: clampChannel(parts[2]) }
  }

  const hsl = /^hsla?\(([^)]*)\)$/.exec(trimmed)
  if (hsl) {
    const parts = numbersIn(hsl[1])
    if (parts.length < 3) return null
    return hslToRgb({
      h: ((parts[0] % 360) + 360) % 360,
      s: clamp(parts[1], 0, 100),
      l: clamp(parts[2], 0, 100),
    })
  }

  return null
}

function numbersIn(text: string): number[] {
  const found = text.match(/-?\d*\.?\d+/g)
  if (found === null) return []
  return found.map(Number).filter((n) => Number.isFinite(n))
}

/** Always lowercase, always seven characters. */
export function toHex(c: RGB): string {
  const part = (value: number) => clampChannel(value).toString(16).padStart(2, '0')
  return '#' + part(c.r) + part(c.g) + part(c.b)
}

export function toRgbString(c: RGB): string {
  return `rgb(${clampChannel(c.r)}, ${clampChannel(c.g)}, ${clampChannel(c.b)})`
}

export function rgbToHsl(c: RGB): HSL {
  const r = clampChannel(c.r) / 255
  const g = clampChannel(c.g) / 255
  const b = clampChannel(c.b) / 255

  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const l = (max + min) / 2

  if (max === min) return { h: 0, s: 0, l: Math.round(l * 100) }

  const d = max - min
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)

  let h = 0
  if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6
  else if (max === g) h = ((b - r) / d + 2) / 6
  else h = ((r - g) / d + 4) / 6

  return { h: Math.round(h * 360) % 360, s: Math.round(s * 100), l: Math.round(l * 100) }
}

export function hslToRgb(c: HSL): RGB {
  const h = (((c.h % 360) + 360) % 360) / 360
  const s = clamp(c.s, 0, 100) / 100
  const l = clamp(c.l, 0, 100) / 100

  if (s === 0) {
    const grey = clampChannel(l * 255)
    return { r: grey, g: grey, b: grey }
  }

  const q = l < 0.5 ? l * (1 + s) : l + s - l * s
  const p = 2 * l - q
  const channel = (t: number) => {
    let value = t
    if (value < 0) value += 1
    if (value > 1) value -= 1
    if (value < 1 / 6) return p + (q - p) * 6 * value
    if (value < 1 / 2) return q
    if (value < 2 / 3) return p + (q - p) * (2 / 3 - value) * 6
    return p
  }

  return {
    r: clampChannel(channel(h + 1 / 3) * 255),
    g: clampChannel(channel(h) * 255),
    b: clampChannel(channel(h - 1 / 3) * 255),
  }
}

export function toHslString(c: RGB): string {
  const hsl = rgbToHsl(c)
  return `hsl(${hsl.h}, ${hsl.s}%, ${hsl.l}%)`
}

/** WCAG 2.1 relative luminance: 0 for black, 1 for white. */
export function relativeLuminance(c: RGB): number {
  const channel = (value: number) => {
    const v = clampChannel(value) / 255
    return v <= SRGB_THRESHOLD
      ? v / SRGB_LOW_DIVISOR
      : Math.pow((v + SRGB_OFFSET) / SRGB_DIVISOR, SRGB_EXPONENT)
  }
  return LUMINANCE_R * channel(c.r) + LUMINANCE_G * channel(c.g) + LUMINANCE_B * channel(c.b)
}

/** WCAG 2.1 contrast ratio, from 1 (identical) to 21 (black on white). */
export function contrastRatio(a: RGB, b: RGB): number {
  const la = relativeLuminance(a)
  const lb = relativeLuminance(b)
  const lighter = Math.max(la, lb)
  const darker = Math.min(la, lb)
  return (lighter + 0.05) / (darker + 0.05)
}

export function passes(ratio: number, level: 'aa' | 'aaLarge' | 'aaa'): boolean {
  if (level === 'aa') return ratio >= AA_NORMAL
  if (level === 'aaLarge') return ratio >= AA_LARGE
  return ratio >= AAA_NORMAL
}

/** The ratio as it is written in an accessibility report. */
export function formatRatio(ratio: number): string {
  return ratio.toFixed(2) + ':1'
}

export const WHITE: RGB = { r: 255, g: 255, b: 255 }
export const BLACK: RGB = { r: 0, g: 0, b: 0 }

/**
 * A pixel with alpha is composited over white before being reported, because
 * that is what it looks like on the page it came from. The caller says so on
 * screen rather than quietly reporting a colour nobody can see.
 */
export function overWhite(c: RGB, alpha: number): RGB {
  const a = clamp(alpha, 0, 1)
  return {
    r: clampChannel(c.r * a + 255 * (1 - a)),
    g: clampChannel(c.g * a + 255 * (1 - a)),
    b: clampChannel(c.b * a + 255 * (1 - a)),
  }
}
