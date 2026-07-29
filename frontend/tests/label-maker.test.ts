import test from 'node:test'
import assert from 'node:assert/strict'
import { modulesToPath } from '../src/tools/wifi-qr/render.ts'
import {
  DEFAULT_SETTINGS,
  MAX_COPIES,
  PAGES,
  PRESETS,
  COPIES_MESSAGE,
  LINE_LONG_MESSAGE,
  NEWER_VERSION_MESSAGE,
  docWarning,
  fitFontSize,
  labelSvg,
  layout,
  migrateDoc,
  pageById,
  pngFileName,
  presetById,
  previewCaption,
  sheetHtml,
  validateLabel,
  type QrMatrix,
} from '../src/tools/label-maker/labels.ts'

const A4 = pageById('a4')
const LETTER = pageById('letter')

/** A tiny 3x3 matrix standing in for a QR code, so no backend is needed. */
function matrix(): QrMatrix {
  return {
    size: 3,
    quiet: 1,
    modules: [true, false, true, false, true, false, true, false, true],
  }
}

test('PRESETS are eight distinct, positively sized labels', () => {
  assert.equal(PRESETS.length, 8)
  assert.equal(new Set(PRESETS.map((p) => p.id)).size, 8)
  for (const preset of PRESETS) {
    assert.ok(preset.w > 0, preset.id)
    assert.ok(preset.h > 0, preset.id)
    assert.notEqual(preset.label, '')
  }
  assert.deepEqual(
    PRESETS.filter((p) => p.kind === 'roll').map((p) => p.id),
    ['brother-dk11201', 'brother-dk11209', 'dymo-99012'],
  )
})

test('presetById and pageById fall back rather than returning undefined', () => {
  assert.equal(presetById('avery-l7163').w, 99.1)
  assert.equal(presetById('nonsense').id, PRESETS[0].id)
  assert.equal(pageById('letter').w, 215.9)
  assert.equal(pageById('nonsense').id, 'a4')
})

// The counts on the right are the vendors' own published figures, written here
// as literals. The code computes them from the millimetres, so this is a real
// check rather than a table compared with itself.
test('layout works out the published per-sheet counts on A4', () => {
  const published: Record<string, number> = {
    'avery-l7159': 24,
    'avery-l7160': 21,
    'avery-l7163': 14,
    'avery-l7651': 65,
  }
  for (const [id, count] of Object.entries(published)) {
    const plan = layout(presetById(id), 1, A4)
    assert.equal(plan.perPage, count, `${id} works out at ${plan.perPage}, the sheet holds ${count}`)
  }
})

test('layout puts avery-l7159 in 3 columns and 8 rows on A4', () => {
  const plan = layout(presetById('avery-l7159'), 1, A4)
  assert.equal(plan.columns, 3)
  assert.equal(plan.rows, 8)
})

test('layout spills onto more pages, and the second page is full', () => {
  const preset = presetById('avery-l7159')
  assert.equal(layout(preset, 24, A4).pages, 1)
  assert.equal(layout(preset, 25, A4).pages, 2)
  assert.equal(layout(preset, 48, A4).pages, 2)
  assert.equal(layout(preset, 49, A4).pages, 3)

  const plan = layout(preset, 48, A4)
  assert.equal(plan.positions.length, 48)
  assert.equal(plan.positions.filter((p) => p.page === 1).length, 24)
})

test('layout starts at the margins and steps by the label plus the gap', () => {
  const preset = presetById('avery-l7159')
  const plan = layout(preset, 4, A4)
  assert.equal(plan.positions[0].x, preset.marginLeft)
  assert.equal(plan.positions[0].y, preset.marginTop)
  assert.equal(plan.positions[1].x, preset.marginLeft + preset.w + preset.gapX)
  assert.equal(plan.positions[1].y, preset.marginTop)
  // The fourth label wraps onto the second row.
  assert.equal(plan.positions[3].x, preset.marginLeft)
  assert.equal(plan.positions[3].y, preset.marginTop + preset.h + preset.gapY)
})

test('Letter is recomputed and fits fewer rows than A4', () => {
  const preset = presetById('avery-l7159')
  const a4 = layout(preset, 1, A4)
  const letter = layout(preset, 1, LETTER)
  assert.ok(letter.rows < a4.rows, `letter ${letter.rows} rows, a4 ${a4.rows} rows`)
  assert.equal(letter.sheet.w, 215.9)
  assert.equal(letter.sheet.h, 279.4)
})

test('a roll gives one label per page and as many pages as copies', () => {
  const plan = layout(presetById('dymo-99012'), 5, A4)
  assert.equal(plan.perPage, 1)
  assert.equal(plan.pages, 5)
  assert.equal(plan.sheet.w, 89)
  assert.equal(plan.sheet.h, 36)
  assert.equal(
    plan.positions.every((p) => p.x === 0 && p.y === 0),
    true,
  )
})

test('fitFontSize keeps a short line and shrinks a long one', () => {
  assert.equal(fitFontSize('AB', 40, 10), 10)
  assert.ok(fitFontSize('A'.repeat(40), 40, 10) < 10)
})

test('fitFontSize shrinks by the documented 0.6 character width', () => {
  // 40 characters across 40 mm at 0.6 of the font size each: 40 / (0.6 * 40).
  // The literal is deliberate. Asserting only "smaller than the start" would
  // pass for any character width, including one that overflows the label.
  assert.ok(Math.abs(fitFontSize('A'.repeat(40), 40, 10) - 1.6667) < 0.001)
  assert.ok(Math.abs(fitFontSize('A'.repeat(10), 60, 20) - 10) < 0.001)
})

test('fitFontSize never goes below the readable floor and never divides by zero', () => {
  assert.equal(fitFontSize('A'.repeat(500), 10, 10), 1.2)
  assert.equal(fitFontSize('', 40, 10), 10)
})

test('labelSvg carries the preset millimetres in its viewBox', () => {
  const preset = presetById('avery-l7159')
  const svg = labelSvg(
    { line1: 'CH-LT-042', line2: '', line3: '' },
    preset,
    null,
    { border: 'thin', qrSide: 'right' },
  )
  assert.match(svg, /viewBox="0 0 63\.5 33\.9"/)
  assert.match(svg, /^<svg /)
  assert.match(svg, /<\/svg>$/)
})

test('labelSvg uses only black and white', () => {
  const svg = labelSvg(
    { line1: 'CH-LT-042', line2: 'Reception', line3: '192.168.1.42' },
    presetById('avery-l7159'),
    matrix(),
    { border: 'thin', qrSide: 'right' },
  )
  const colours = new Set(svg.match(/#[0-9a-f]{3,6}/gi) ?? [])
  assert.deepEqual([...colours].sort(), ['#000000', '#ffffff'])
})

test('labelSvg draws the QR code with exactly the shared path builder', () => {
  const qr = matrix()
  const withQr = labelSvg(
    { line1: 'A', line2: '', line3: '' },
    presetById('avery-l7159'),
    qr,
    { border: 'none', qrSide: 'right' },
  )
  const expected = modulesToPath(qr.modules, qr.size, qr.quiet)
  assert.equal(withQr.includes(`d="${expected}"`), true)
  assert.equal((withQr.match(/<path/g) ?? []).length, 1)
})

test('labelSvg with no QR code draws no path at all', () => {
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, presetById('avery-l7159'), null, {
    border: 'none',
    qrSide: 'right',
  })
  assert.equal(svg.includes('<path'), false)
})

test('labelSvg puts the QR on the side it was asked for', () => {
  const preset = presetById('avery-l7159')
  const right = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, matrix(), {
    border: 'none',
    qrSide: 'right',
  })
  const left = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, matrix(), {
    border: 'none',
    qrSide: 'left',
  })

  const qrX = (svg: string) => Number(/translate\(([\d.]+) /.exec(svg)?.[1])
  const textX = (svg: string) => Number(/<text x="([\d.]+)"/.exec(svg)?.[1])

  assert.ok(qrX(right) > textX(right), 'on the right the QR should sit after the text')
  assert.ok(qrX(left) < textX(left), 'on the left the QR should sit before the text')
})

test('labelSvg emits a text element only for the lines that have something in them', () => {
  const preset = presetById('avery-l7159')
  const one = labelSvg({ line1: 'A', line2: '', line3: '   ' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  assert.equal((one.match(/<text/g) ?? []).length, 1)

  const three = labelSvg({ line1: 'A', line2: 'B', line3: 'C' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  assert.equal((three.match(/<text/g) ?? []).length, 3)

  const none = labelSvg({ line1: '', line2: '', line3: '' }, preset, null, {
    border: 'thin',
    qrSide: 'right',
  })
  assert.equal(none.includes('<text'), false)
})

test('labelSvg draws the border only when asked', () => {
  const preset = presetById('avery-l7159')
  const thin = labelSvg({ line1: '', line2: '', line3: '' }, preset, null, {
    border: 'thin',
    qrSide: 'right',
  })
  const none = labelSvg({ line1: '', line2: '', line3: '' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  assert.equal(thin.includes('stroke="#000000"'), true)
  assert.equal(none.includes('stroke='), false)
})

test('labelSvg escapes text that would otherwise break the SVG', () => {
  const svg = labelSvg(
    { line1: 'A & B <tag> "q"', line2: '', line3: '' },
    presetById('avery-l7159'),
    null,
    { border: 'none', qrSide: 'right' },
  )
  assert.equal(svg.includes('A &amp; B &lt;tag&gt; &quot;q&quot;'), true)
  assert.equal(svg.includes('<tag>'), false)
})

test('sheetHtml is a complete document with the right page size', () => {
  const preset = presetById('avery-l7159')
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, null, {
    border: 'thin',
    qrSide: 'right',
  })
  const html = sheetHtml([svg, svg], preset, A4)

  assert.match(html, /^<!doctype html>/)
  assert.match(html, /@page \{ size: 210mm 297mm; margin: 0 \}/)
  assert.equal((html.match(/class="sheet"/g) ?? []).length, 1)
  assert.equal((html.match(/class="label"/g) ?? []).length, 2)
  assert.match(html, /<\/body><\/html>$/)
})

test('sheetHtml breaks the page between sheets, and not after the last one', () => {
  // Without this every sheet runs into the next and a two page job prints as
  // one long strip with the labels in the wrong places.
  const preset = presetById('avery-l7159')
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  const html = sheetHtml(Array.from({ length: 25 }, () => svg), preset, A4)
  assert.match(html, /\.sheet \{[^}]*page-break-after: always/)
  assert.match(html, /\.sheet:last-child \{ page-break-after: auto \}/)
})

test('sheetHtml positions every label exactly where layout says', () => {
  const preset = presetById('avery-l7159')
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  const html = sheetHtml([svg, svg], preset, A4)
  const plan = layout(preset, 2, A4)

  for (const position of plan.positions) {
    assert.equal(
      html.includes(`left:${position.x}mm;top:${position.y}mm;`),
      true,
      `no label at ${position.x},${position.y}`,
    )
  }
})

test('sheetHtml makes one sheet div per page', () => {
  const preset = presetById('avery-l7159')
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  const html = sheetHtml(Array.from({ length: 25 }, () => svg), preset, A4)
  assert.equal((html.match(/class="sheet"/g) ?? []).length, 2)
  assert.equal((html.match(/class="label"/g) ?? []).length, 25)
})

test('sheetHtml gives A4 and Letter different page sizes', () => {
  const preset = presetById('avery-l7159')
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  assert.match(sheetHtml([svg], preset, A4), /size: 210mm 297mm/)
  assert.match(sheetHtml([svg], preset, LETTER), /size: 215\.9mm 279\.4mm/)
})

test('sheetHtml lays a roll label at exactly the label size', () => {
  const preset = presetById('dymo-99012')
  const svg = labelSvg({ line1: 'A', line2: '', line3: '' }, preset, null, {
    border: 'none',
    qrSide: 'right',
  })
  assert.match(sheetHtml([svg], preset, A4), /size: 89mm 36mm/)
})

test('pngFileName slugs line 1 and falls back when it is empty', () => {
  assert.equal(pngFileName('CH-LT-042'), 'label-ch-lt-042')
  assert.equal(pngFileName('  Reception  Laptop!  '), 'label-reception-laptop')
  assert.equal(pngFileName(''), 'label')
  assert.equal(pngFileName('!!!'), 'label')
})

test('validateLabel checks each line length at the boundary', () => {
  const ok = validateLabel(
    { line1: 'a'.repeat(60), line2: 'b'.repeat(60), line3: 'c'.repeat(60) },
    '',
    '1',
  )
  assert.deepEqual(ok, {})

  const long = validateLabel(
    { line1: 'a'.repeat(61), line2: 'b'.repeat(61), line3: 'c'.repeat(61) },
    '',
    '1',
  )
  assert.equal(long.line1, LINE_LONG_MESSAGE)
  assert.equal(long.line2, LINE_LONG_MESSAGE)
  assert.equal(long.line3, LINE_LONG_MESSAGE)
})

test('validateLabel checks the copies field', () => {
  const empty = { line1: '', line2: '', line3: '' }
  assert.equal(validateLabel(empty, '', '1').copies, undefined)
  assert.equal(validateLabel(empty, '', '500').copies, undefined)
  assert.equal(validateLabel(empty, '', '0').copies, COPIES_MESSAGE)
  assert.equal(validateLabel(empty, '', '501').copies, COPIES_MESSAGE)
  assert.equal(validateLabel(empty, '', '').copies, COPIES_MESSAGE)
  assert.equal(validateLabel(empty, '', 'abc').copies, COPIES_MESSAGE)
  assert.equal(validateLabel(empty, '', '-1').copies, COPIES_MESSAGE)
  assert.equal(validateLabel(empty, '', '1.5').copies, COPIES_MESSAGE)
})

test('validateLabel accepts a completely empty label', () => {
  assert.deepEqual(validateLabel({ line1: '', line2: '', line3: '' }, '', '24'), {})
})

test('migrateDoc gives the defaults when there is nothing usable saved', () => {
  assert.deepEqual(migrateDoc(null), DEFAULT_SETTINGS)
  assert.deepEqual(migrateDoc('text'), DEFAULT_SETTINGS)
  assert.deepEqual(migrateDoc({}), DEFAULT_SETTINGS)
  assert.deepEqual(migrateDoc({ version: 2, line1: 'kept?' }), DEFAULT_SETTINGS)
})

test('migrateDoc falls back field by field rather than losing the whole document', () => {
  const saved = migrateDoc({
    version: 1,
    line1: 'CH-LT-042',
    line2: 'Reception',
    line3: '192.168.1.42',
    qrText: 'CH-LT-042',
    presetId: 'no-such-preset',
    page: 'foolscap',
    copies: 9999,
    border: 'dotted',
    qrSide: 'middle',
    pngScale: 3,
  })
  assert.equal(saved.line1, 'CH-LT-042')
  assert.equal(saved.line3, '192.168.1.42')
  assert.equal(saved.presetId, DEFAULT_SETTINGS.presetId)
  assert.equal(saved.page, DEFAULT_SETTINGS.page)
  assert.equal(saved.copies, DEFAULT_SETTINGS.copies)
  assert.equal(saved.border, DEFAULT_SETTINGS.border)
  assert.equal(saved.qrSide, DEFAULT_SETTINGS.qrSide)
  assert.equal(saved.pngScale, DEFAULT_SETTINGS.pngScale)
})

test('migrateDoc keeps values that are valid', () => {
  const saved = migrateDoc({
    version: 1,
    presetId: 'dymo-99012',
    page: 'letter',
    copies: 1,
    border: 'none',
    qrSide: 'left',
    pngScale: 12,
  })
  assert.equal(saved.presetId, 'dymo-99012')
  assert.equal(saved.page, 'letter')
  assert.equal(saved.copies, 1)
  assert.equal(saved.border, 'none')
  assert.equal(saved.qrSide, 'left')
  assert.equal(saved.pngScale, 12)
})

test('migrateDoc rejects copies outside the range at the boundary', () => {
  assert.equal(migrateDoc({ version: 1, copies: 1 }).copies, 1)
  assert.equal(migrateDoc({ version: 1, copies: MAX_COPIES }).copies, MAX_COPIES)
  assert.equal(migrateDoc({ version: 1, copies: 0 }).copies, DEFAULT_SETTINGS.copies)
  assert.equal(migrateDoc({ version: 1, copies: MAX_COPIES + 1 }).copies, DEFAULT_SETTINGS.copies)
})

test('docWarning only fires for a document from the future', () => {
  assert.equal(docWarning({ version: 2 }), NEWER_VERSION_MESSAGE)
  assert.equal(docWarning({ version: 1 }), '')
  assert.equal(docWarning(null), '')
})

test('previewCaption names the millimetres and how many fit', () => {
  const preset = presetById('avery-l7159')
  assert.equal(
    previewCaption(preset, layout(preset, 1, A4), A4),
    '63.5 x 33.9 mm, 24 per A4 page',
  )
  const roll = presetById('dymo-99012')
  assert.equal(previewCaption(roll, layout(roll, 1, A4), A4), '89 x 36 mm, roll label')
})

test('PAGES are the two the tool offers, in millimetres', () => {
  assert.deepEqual(
    PAGES.map((p) => p.id),
    ['a4', 'letter'],
  )
  assert.equal(PAGES[0].w, 210)
  assert.equal(PAGES[0].h, 297)
})
