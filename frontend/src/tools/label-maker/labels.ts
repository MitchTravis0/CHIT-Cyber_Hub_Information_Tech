// Label geometry, the drawing and the print sheet. All pure and in millimetres,
// so the whole thing can be tested without a DOM and without a printer.

// Fifteen lines of pure geometry that turn a QR module matrix into one SVG
// path, already tested by frontend/tests/wifi-qr-render.test.ts. Imported
// rather than copied: a second QR renderer is a fork.
import { modulesToPath } from '../wifi-qr/render'

export const LABEL_NAMESPACE = 'label-maker'
export const LABEL_DOC_VERSION = 1

export const MAX_LINE = 60
export const MIN_COPIES = 1
export const MAX_COPIES = 500

export type PresetKind = 'sheet' | 'roll'
export type PageId = 'a4' | 'letter'
export type Border = 'thin' | 'none'
export type QrSide = 'right' | 'left'

export interface Preset {
  id: string
  label: string
  /** Millimetres. */
  w: number
  h: number
  kind: PresetKind
  marginTop: number
  marginLeft: number
  gapX: number
  gapY: number
}

export interface Page {
  id: PageId
  label: string
  w: number
  h: number
}

export const PAGES: Page[] = [
  { id: 'a4', label: 'A4', w: 210, h: 297 },
  { id: 'letter', label: 'Letter', w: 215.9, h: 279.4 },
]

/**
 * Label stock, in the millimetres the vendor publishes. How many fit on a page
 * is computed from these rather than stored, so the two can never disagree.
 *
 * CHIT does not claim a preset is calibrated to a particular printer: printer
 * margins differ and nothing in this repo can prove otherwise, which is why the
 * help blurb tells the tech to print one plain-paper page first.
 */
export const PRESETS: Preset[] = [
  {
    id: 'avery-l7159',
    label: 'Avery L7159, 63.5 x 33.9 mm, 24 per sheet',
    w: 63.5,
    h: 33.9,
    kind: 'sheet',
    marginTop: 12.7,
    marginLeft: 7.2,
    gapX: 2.5,
    gapY: 0,
  },
  {
    id: 'avery-l7160',
    label: 'Avery L7160, 63.5 x 38.1 mm, 21 per sheet',
    w: 63.5,
    h: 38.1,
    kind: 'sheet',
    marginTop: 15.1,
    marginLeft: 7.2,
    gapX: 2.5,
    gapY: 0,
  },
  {
    id: 'avery-l7163',
    label: 'Avery L7163, 99.1 x 38.1 mm, 14 per sheet',
    w: 99.1,
    h: 38.1,
    kind: 'sheet',
    marginTop: 15.1,
    marginLeft: 5,
    gapX: 2.5,
    gapY: 0,
  },
  {
    id: 'avery-l7651',
    label: 'Avery L7651, 38.1 x 21.2 mm, 65 per sheet',
    w: 38.1,
    h: 21.2,
    kind: 'sheet',
    marginTop: 10.7,
    marginLeft: 4.7,
    gapX: 2.5,
    gapY: 0,
  },
  {
    id: 'brother-dk11201',
    label: 'Brother DK-11201 roll, 90 x 29 mm',
    w: 90,
    h: 29,
    kind: 'roll',
    marginTop: 0,
    marginLeft: 0,
    gapX: 0,
    gapY: 0,
  },
  {
    id: 'brother-dk11209',
    label: 'Brother DK-11209 roll, 62 x 29 mm',
    w: 62,
    h: 29,
    kind: 'roll',
    marginTop: 0,
    marginLeft: 0,
    gapX: 0,
    gapY: 0,
  },
  {
    id: 'dymo-99012',
    label: 'DYMO 99012 roll, 89 x 36 mm',
    w: 89,
    h: 36,
    kind: 'roll',
    marginTop: 0,
    marginLeft: 0,
    gapX: 0,
    gapY: 0,
  },
  {
    id: 'plain-50x25',
    label: 'Plain 50 x 25 mm',
    w: 50,
    h: 25,
    kind: 'sheet',
    marginTop: 10,
    marginLeft: 3,
    gapX: 2,
    gapY: 0,
  },
]

export function presetById(id: string): Preset {
  return PRESETS.find((preset) => preset.id === id) ?? PRESETS[0]
}

export function pageById(id: string): Page {
  return PAGES.find((page) => page.id === id) ?? PAGES[0]
}

export interface Position {
  page: number
  x: number
  y: number
}

export interface Layout {
  columns: number
  rows: number
  perPage: number
  pages: number
  /** The page size the labels are laid on, which a roll ignores. */
  sheet: { w: number; h: number }
  positions: Position[]
}

/**
 * Where every copy sits.
 *
 * Only the leading margin is reserved: the trailing edge simply has to fit,
 * which is how the published counts for these sheets come out (an Avery L7163
 * leaves 4.3 mm on the right, not another 5).
 *
 * A roll has no page, so it lays one label per page at exactly the label size.
 */
export function layout(preset: Preset, copies: number, page: Page): Layout {
  const wanted = Math.max(1, Math.floor(copies))

  if (preset.kind === 'roll') {
    return {
      columns: 1,
      rows: 1,
      perPage: 1,
      pages: wanted,
      sheet: { w: preset.w, h: preset.h },
      positions: Array.from({ length: wanted }, (_, at) => ({ page: at, x: 0, y: 0 })),
    }
  }

  const columns = Math.max(
    1,
    Math.floor((page.w - preset.marginLeft + preset.gapX) / (preset.w + preset.gapX)),
  )
  const rows = Math.max(
    1,
    Math.floor((page.h - preset.marginTop + preset.gapY) / (preset.h + preset.gapY)),
  )
  const perPage = columns * rows
  const pages = Math.ceil(wanted / perPage)

  const positions: Position[] = []
  for (let at = 0; at < wanted; at++) {
    const onPage = at % perPage
    positions.push({
      page: Math.floor(at / perPage),
      x: preset.marginLeft + (onPage % columns) * (preset.w + preset.gapX),
      y: preset.marginTop + Math.floor(onPage / columns) * (preset.h + preset.gapY),
    })
  }

  return { columns, rows, perPage, pages, sheet: { w: page.w, h: page.h }, positions }
}

/** The smallest a line is allowed to shrink to before it stops being readable. */
const MIN_FONT_MM = 1.2

/**
 * Shrinks a line until it fits the width it has. A character in a sans-serif at
 * these sizes averages about 0.6 of the font size wide, which is the usual
 * approximation and close enough for a label.
 */
export function fitFontSize(text: string, widthMm: number, startMm: number): number {
  const characters = Math.max(1, text.length)
  return Math.max(MIN_FONT_MM, Math.min(startMm, widthMm / (0.6 * characters)))
}

export interface LabelText {
  line1: string
  line2: string
  line3: string
}

export interface QrMatrix {
  size: number
  modules: boolean[]
  quiet: number
}

export interface DrawOptions {
  border: Border
  qrSide: QrSide
}

function escapeXml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

/** Rounds to three decimals so the SVG stays readable and comparable. */
function mm(value: number): string {
  return String(Math.round(value * 1000) / 1000)
}

const LINE_SHARE = [0.44, 0.26, 0.2]

/**
 * One label as a complete SVG element, in millimetre units.
 *
 * White background, black marks, and no theme token in sight. A label is
 * printed on white paper and read by a camera, so it cannot follow the app's
 * dark mode. This is the same exception the Wi-Fi QR tool takes in
 * frontend/src/tools/wifi-qr/png.ts, and these are the only two colours in this
 * tool that are not semantic tokens.
 */
export function labelSvg(
  text: LabelText,
  preset: Preset,
  qr: QrMatrix | null,
  options: DrawOptions,
): string {
  const padding = Math.min(Math.min(preset.w, preset.h) * 0.08, 3)
  const parts: string[] = [
    `<rect x="0" y="0" width="${mm(preset.w)}" height="${mm(preset.h)}" fill="#ffffff"/>`,
  ]

  if (options.border === 'thin') {
    parts.push(
      `<rect x="0.1" y="0.1" width="${mm(preset.w - 0.2)}" height="${mm(preset.h - 0.2)}" fill="none" stroke="#000000" stroke-width="0.2"/>`,
    )
  }

  const qrSide = qr === null ? 0 : preset.h - padding * 2
  if (qr !== null) {
    const modules = qr.size + qr.quiet * 2
    const scale = qrSide / modules
    const x = options.qrSide === 'right' ? preset.w - padding - qrSide : padding
    parts.push(
      `<g transform="translate(${mm(x)} ${mm(padding)}) scale(${mm(scale)})">` +
        `<path d="${modulesToPath(qr.modules, qr.size, qr.quiet)}" fill="#000000"/></g>`,
    )
  }

  const textX = qr !== null && options.qrSide === 'left' ? padding * 2 + qrSide : padding
  const textWidth = preset.w - padding * 2 - (qr === null ? 0 : qrSide + padding)

  const lines = [text.line1, text.line2, text.line3]
    .map((body, at) => ({ body: body.trim(), share: LINE_SHARE[at], bold: at === 0 }))
    .filter((line) => line.body !== '')

  if (lines.length > 0) {
    const available = preset.h - padding * 2
    const sized = lines.map((line) => ({
      ...line,
      size: fitFontSize(line.body, textWidth, available * line.share),
    }))
    const gap = preset.h * 0.08
    const total = sized.reduce((sum, line) => sum + line.size, 0) + gap * (sized.length - 1)

    let cursor = (preset.h - total) / 2
    for (const line of sized) {
      // SVG anchors text on its baseline, which sits about 80% down the em box.
      const baseline = cursor + line.size * 0.8
      parts.push(
        `<text x="${mm(textX)}" y="${mm(baseline)}" font-family="Helvetica, Arial, sans-serif"` +
          ` font-size="${mm(line.size)}"${line.bold ? ' font-weight="bold"' : ''} fill="#000000">` +
          `${escapeXml(line.body)}</text>`,
      )
      cursor += line.size + gap
    }
  }

  return (
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${mm(preset.w)} ${mm(preset.h)}"` +
    ` width="100%" height="100%">${parts.join('')}</svg>`
  )
}

/**
 * The whole print sheet as one HTML document, written into a hidden iframe and
 * handed to the OS print dialog.
 *
 * This is the one place in the tool with a style block. It is not app UI: it is
 * a generated print artifact rendered in its own document by the OS print
 * pipeline, it has to express physical millimetres and an @page size that no
 * utility class can, and it must be black on white whatever theme the app is
 * in. The in-app preview follows the normal rules. The function is pure and its
 * output is asserted in the test suite.
 */
export function sheetHtml(labels: string[], preset: Preset, page: Page): string {
  const plan = layout(preset, labels.length, page)
  const pages: string[][] = Array.from({ length: plan.pages }, () => [])

  plan.positions.forEach((position, at) => {
    pages[position.page].push(
      `<div class="label" style="left:${mm(position.x)}mm;top:${mm(position.y)}mm;` +
        `width:${mm(preset.w)}mm;height:${mm(preset.h)}mm">${labels[at]}</div>`,
    )
  })

  const sheets = pages
    .map((body) => `<div class="sheet">${body.join('')}</div>`)
    .join('')

  return (
    '<!doctype html><html><head><meta charset="utf-8"><title>CHIT labels</title><style>' +
    `@page { size: ${mm(plan.sheet.w)}mm ${mm(plan.sheet.h)}mm; margin: 0 }` +
    'html, body { margin: 0; padding: 0; background: #fff }' +
    `.sheet { position: relative; width: ${mm(plan.sheet.w)}mm; height: ${mm(plan.sheet.h)}mm; page-break-after: always }` +
    '.sheet:last-child { page-break-after: auto }' +
    '.label { position: absolute }' +
    'svg { display: block; width: 100%; height: 100% }' +
    `</style></head><body>${sheets}</body></html>`
  )
}

export const LINE_LONG_MESSAGE =
  'Keep a line to 60 characters or fewer. It would be too small to read at this label size.'
export const COPIES_MESSAGE = 'Type how many labels to print, from 1 to 500.'
export const NO_QR_MESSAGE = 'QR codes need the desktop app. The label will print without one.'
export const PRINT_FAILED_MESSAGE =
  'The print sheet could not be prepared. Try Download PNG instead, or restart CHIT.'
export const NEWER_VERSION_MESSAGE =
  'These label settings were written by a newer version of CHIT and could not be read. Update CHIT to use them.'

export interface LabelErrors {
  line1?: string
  line2?: string
  line3?: string
  qrText?: string
  copies?: string
}

export function validateLabel(text: LabelText, qrText: string, copies: string): LabelErrors {
  const errors: LabelErrors = {}
  if (text.line1.length > MAX_LINE) errors.line1 = LINE_LONG_MESSAGE
  if (text.line2.length > MAX_LINE) errors.line2 = LINE_LONG_MESSAGE
  if (text.line3.length > MAX_LINE) errors.line3 = LINE_LONG_MESSAGE

  const count = Number(copies.trim())
  if (!/^\d+$/.test(copies.trim()) || count < MIN_COPIES || count > MAX_COPIES) {
    errors.copies = COPIES_MESSAGE
  }
  // qrText has no length rule here: the encoder decides what fits, and says so
  // in its own sentence.
  void qrText
  return errors
}

export function slug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/** label-ch-lt-042.png */
export function pngFileName(line1: string): string {
  const name = slug(line1)
  return `label-${name === '' ? '' : name}`.replace(/-$/, '') || 'label'
}

export interface LabelSettings {
  line1: string
  line2: string
  line3: string
  qrText: string
  presetId: string
  page: PageId
  copies: number
  border: Border
  qrSide: QrSide
  pngScale: number
}

export interface LabelDoc extends LabelSettings {
  version: number
}

export const DEFAULT_SETTINGS: LabelSettings = {
  line1: '',
  line2: '',
  line3: '',
  qrText: '',
  presetId: 'avery-l7159',
  page: 'a4',
  copies: 24,
  border: 'thin',
  qrSide: 'right',
  pngScale: 8,
}

export const PNG_SCALES = [4, 8, 12]

function text(value: unknown, fallback: string): string {
  return typeof value === 'string' ? value : fallback
}

function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  return typeof value === 'string' && (allowed as readonly string[]).includes(value)
    ? (value as T)
    : fallback
}

/** Reads the saved settings. Each field falls back on its own, so one unknown
 *  value does not throw the whole document away. */
export function migrateDoc(raw: unknown): LabelSettings {
  if (typeof raw !== 'object' || raw === null) return { ...DEFAULT_SETTINGS }
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > LABEL_DOC_VERSION) {
    return { ...DEFAULT_SETTINGS }
  }

  const copies =
    typeof doc.copies === 'number' &&
    Number.isInteger(doc.copies) &&
    doc.copies >= MIN_COPIES &&
    doc.copies <= MAX_COPIES
      ? doc.copies
      : DEFAULT_SETTINGS.copies

  const scale =
    typeof doc.pngScale === 'number' && PNG_SCALES.includes(doc.pngScale)
      ? doc.pngScale
      : DEFAULT_SETTINGS.pngScale

  return {
    line1: text(doc.line1, ''),
    line2: text(doc.line2, ''),
    line3: text(doc.line3, ''),
    qrText: text(doc.qrText, ''),
    presetId: PRESETS.some((preset) => preset.id === doc.presetId)
      ? (doc.presetId as string)
      : DEFAULT_SETTINGS.presetId,
    page: oneOf<PageId>(doc.page, ['a4', 'letter'], DEFAULT_SETTINGS.page),
    copies,
    border: oneOf<Border>(doc.border, ['thin', 'none'], DEFAULT_SETTINGS.border),
    qrSide: oneOf<QrSide>(doc.qrSide, ['right', 'left'], DEFAULT_SETTINGS.qrSide),
    pngScale: scale,
  }
}

export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > LABEL_DOC_VERSION) return NEWER_VERSION_MESSAGE
  return ''
}

/** The caption under the preview. */
export function previewCaption(preset: Preset, plan: Layout, page: Page): string {
  const size = `${preset.w} x ${preset.h} mm`
  if (preset.kind === 'roll') return `${size}, roll label`
  return `${size}, ${plan.perPage} per ${page.label} page`
}
