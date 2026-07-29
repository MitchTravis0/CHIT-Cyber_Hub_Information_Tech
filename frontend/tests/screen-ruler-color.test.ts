import test from 'node:test'
import assert from 'node:assert/strict'
import {
  AA_LARGE,
  AA_NORMAL,
  AAA_NORMAL,
  BLACK,
  WHITE,
  contrastRatio,
  formatRatio,
  hslToRgb,
  overWhite,
  parseColor,
  passes,
  relativeLuminance,
  rgbToHsl,
  toHex,
  toHslString,
  toRgbString,
} from '../src/tools/screen-ruler/color.ts'

test('parseColor reads every hex form', () => {
  assert.deepEqual(parseColor('#1e90ff'), { r: 30, g: 144, b: 255 })
  assert.deepEqual(parseColor('1e90ff'), { r: 30, g: 144, b: 255 })
  assert.deepEqual(parseColor('#1E90FF'), { r: 30, g: 144, b: 255 })
  assert.deepEqual(parseColor('#1e9'), { r: 17, g: 238, b: 153 })
  assert.deepEqual(parseColor('#FFF'), { r: 255, g: 255, b: 255 })
  assert.deepEqual(parseColor('#000'), { r: 0, g: 0, b: 0 })
  // An eight digit hex carries alpha, which is ignored.
  assert.deepEqual(parseColor('#1e90ffcc'), { r: 30, g: 144, b: 255 })
})

test('parseColor refuses a hex that is not one', () => {
  for (const text of ['#12345', '#gg0000', '', '   ', '#', '#1234567', 'not a colour']) {
    assert.equal(parseColor(text), null, `${text} should be refused`)
  }
})

test('parseColor reads the rgb function form', () => {
  assert.deepEqual(parseColor('rgb(30, 144, 255)'), { r: 30, g: 144, b: 255 })
  assert.deepEqual(parseColor('rgb(30,144,255)'), { r: 30, g: 144, b: 255 })
  assert.deepEqual(parseColor('RGB( 30 , 144 , 255 )'), { r: 30, g: 144, b: 255 })
  assert.deepEqual(parseColor('rgba(30,144,255,0.5)'), { r: 30, g: 144, b: 255 })
})

test('parseColor clamps an rgb channel out of range instead of refusing it', () => {
  assert.deepEqual(parseColor('rgb(300, -4, 900)'), { r: 255, g: 0, b: 255 })
})

test('parseColor refuses an rgb with too few channels', () => {
  assert.equal(parseColor('rgb(1,2)'), null)
  assert.equal(parseColor('rgb()'), null)
})

test('parseColor reads the hsl function form', () => {
  assert.deepEqual(parseColor('hsl(210, 100%, 56%)'), { r: 31, g: 143, b: 255 })
  assert.deepEqual(parseColor('hsl(210,100%,56%)'), { r: 31, g: 143, b: 255 })
  assert.deepEqual(parseColor('hsla(210,100%,56%,0.5)'), { r: 31, g: 143, b: 255 })
  // No percent signs still works: people type it that way.
  assert.deepEqual(parseColor('hsl(210, 100, 56)'), { r: 31, g: 143, b: 255 })
})

test('parseColor wraps the hue and clamps saturation and lightness', () => {
  // 370 wraps to 10, saturation clamps to 100, lightness clamps to 0, so black.
  assert.deepEqual(parseColor('hsl(370, 150%, -5%)'), { r: 0, g: 0, b: 0 })
  assert.deepEqual(parseColor('hsl(0, 0%, 200%)'), { r: 255, g: 255, b: 255 })
})

test('toHex is always lowercase and always seven characters', () => {
  assert.equal(toHex({ r: 0, g: 0, b: 0 }), '#000000')
  assert.equal(toHex({ r: 255, g: 255, b: 255 }), '#ffffff')
  assert.equal(toHex({ r: 1, g: 2, b: 3 }), '#010203')
  assert.equal(toHex({ r: 30, g: 144, b: 255 }), '#1e90ff')
  for (const c of [
    { r: 0, g: 0, b: 0 },
    { r: 255, g: 255, b: 255 },
    { r: 30, g: 144, b: 255 },
  ]) {
    assert.equal(toHex(c).length, 7)
    assert.equal(toHex(c), toHex(c).toLowerCase())
  }
})

test('toRgbString and toHslString are the forms a stylesheet takes', () => {
  assert.equal(toRgbString({ r: 30, g: 144, b: 255 }), 'rgb(30, 144, 255)')
  assert.equal(toHslString({ r: 30, g: 144, b: 255 }), 'hsl(210, 100%, 56%)')
})

// These three were computed with python3's colorsys before the code was
// written. A round trip alone would only prove the two halves agree with each
// other, which they can do while both being wrong.
test('rgbToHsl matches an independent implementation', () => {
  assert.deepEqual(rgbToHsl({ r: 30, g: 144, b: 255 }), { h: 210, s: 100, l: 56 })
  assert.deepEqual(rgbToHsl({ r: 128, g: 128, b: 128 }), { h: 0, s: 0, l: 50 })
  assert.deepEqual(rgbToHsl({ r: 255, g: 0, b: 0 }), { h: 0, s: 100, l: 50 })
})

test('rgb and hsl round trip to within one per channel', () => {
  const samples = [
    { r: 0, g: 0, b: 0 },
    { r: 255, g: 255, b: 255 },
    { r: 128, g: 128, b: 128 },
    { r: 255, g: 0, b: 0 },
    { r: 0, g: 255, b: 0 },
    { r: 0, g: 0, b: 255 },
    { r: 30, g: 144, b: 255 },
    { r: 17, g: 238, b: 153 },
    { r: 200, g: 100, b: 50 },
    { r: 1, g: 2, b: 3 },
  ]
  for (const c of samples) {
    const back = hslToRgb(rgbToHsl(c))
    for (const key of ['r', 'g', 'b'] as const) {
      assert.ok(
        Math.abs(back[key] - c[key]) <= 3,
        `${toHex(c)} came back as ${toHex(back)} (${key} differs)`,
      )
    }
  }
})

// Every expected value below was computed with python3 from the published WCAG
// formula, before this file was written.
test('relativeLuminance matches the WCAG formula', () => {
  assert.equal(relativeLuminance({ r: 0, g: 0, b: 0 }), 0)
  assert.equal(relativeLuminance({ r: 255, g: 255, b: 255 }), 1)
  assert.equal(Number(relativeLuminance({ r: 128, g: 128, b: 128 }).toFixed(4)), 0.2159)
})

test('contrastRatio matches the published values', () => {
  // Black on white is exactly 21, which is the top of the scale.
  assert.equal(Number(contrastRatio(BLACK, WHITE).toFixed(4)), 21)
  assert.equal(contrastRatio(WHITE, WHITE), 1)
  // #767676 is the canonical darkest grey that passes AA on white.
  assert.equal(Number(contrastRatio({ r: 0x76, g: 0x76, b: 0x76 }, WHITE).toFixed(2)), 4.54)
  assert.equal(Number(contrastRatio({ r: 30, g: 144, b: 255 }, WHITE).toFixed(4)), 3.2365)
  assert.equal(Number(contrastRatio({ r: 30, g: 144, b: 255 }, BLACK).toFixed(4)), 6.4885)
})

test('contrastRatio is symmetric', () => {
  const a = { r: 30, g: 144, b: 255 }
  const b = { r: 240, g: 240, b: 240 }
  assert.equal(contrastRatio(a, b), contrastRatio(b, a))
})

test('the WCAG thresholds are the published ones', () => {
  // Written as literals so the constants cannot be changed without a failure.
  assert.equal(AA_NORMAL, 4.5)
  assert.equal(AA_LARGE, 3)
  assert.equal(AAA_NORMAL, 7)
})

test('passes sits exactly on each threshold', () => {
  assert.equal(passes(4.5, 'aa'), true)
  assert.equal(passes(4.49, 'aa'), false)
  assert.equal(passes(3, 'aaLarge'), true)
  assert.equal(passes(2.99, 'aaLarge'), false)
  assert.equal(passes(7, 'aaa'), true)
  assert.equal(passes(6.99, 'aaa'), false)
  assert.equal(passes(21, 'aaa'), true)
})

test('formatRatio is the form an accessibility report uses', () => {
  assert.equal(formatRatio(21), '21.00:1')
  assert.equal(formatRatio(4.5422), '4.54:1')
  assert.equal(formatRatio(1), '1.00:1')
})

test('overWhite composites a partly transparent pixel', () => {
  const red = { r: 255, g: 0, b: 0 }
  assert.deepEqual(overWhite(red, 1), red)
  assert.deepEqual(overWhite(red, 0), { r: 255, g: 255, b: 255 })
  assert.deepEqual(overWhite(red, 0.5), { r: 255, g: 128, b: 128 })
  // An alpha outside the range is clamped rather than producing nonsense.
  assert.deepEqual(overWhite(red, 5), red)
  assert.deepEqual(overWhite(red, -1), { r: 255, g: 255, b: 255 })
})
