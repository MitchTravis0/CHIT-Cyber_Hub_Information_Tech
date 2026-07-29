import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { contrastRatio, parseColor } from '../src/tools/screen-ruler/color.ts'

// The danger button is outlined rather than filled, so its text sits directly on
// the page background and its legibility is now load bearing. There is no DOM in
// this runner, so the rendered result cannot be asserted. What is asserted is the
// input to it: the real values out of style.css, run through the WCAG maths the
// screen ruler already ships and tests. A palette edit that makes outlined danger
// text illegible fails here.
//
// The ratios below were computed independently in python3 before being written
// in, per the project rule about never typing a constant from memory.

const STYLE = readFileSync(new URL('../src/style.css', import.meta.url), 'utf8')
const BUTTON = readFileSync(new URL('../src/components/Button.tsx', import.meta.url), 'utf8')

function token(name: string): string {
  // Only the :root block, so an html[data-accent] override cannot be picked up.
  const match = STYLE.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{3,8})\\s*;`))
  assert.ok(match, `--${name} is not declared in style.css`)
  return match![1]
}

function ratio(a: string, b: string): number {
  const first = parseColor(a)
  const second = parseColor(b)
  assert.ok(first && second, `could not parse ${a} or ${b}`)
  return contrastRatio(first!, second!)
}

test('style.css still declares the tokens this test reads', () => {
  // A control: if the regex silently stopped matching, every ratio below would
  // be computed from a default and the suite would pass vacuously.
  assert.equal(token('surface'), '#171717')
  assert.equal(token('danger'), '#ff5d5d')
  assert.throws(() => token('not-a-real-token'))
})

test('outlined danger text is legible on the page background', () => {
  const measured = ratio(token('danger'), token('surface'))
  assert.equal(measured.toFixed(4), '5.9553')
  assert.ok(measured >= 4.5, `danger on surface is ${measured.toFixed(2)}:1, below 4.5:1`)
})

test('the other status colours are legible on the page background too', () => {
  assert.equal(ratio(token('ok'), token('surface')).toFixed(4), '10.0472')
  assert.equal(ratio(token('warn'), token('surface')).toFixed(4), '10.0789')
  assert.equal(ratio(token('fg'), token('surface')).toFixed(4), '15.1755')
  assert.equal(ratio(token('fg-muted'), token('surface')).toFixed(4), '6.6916')
  for (const name of ['ok', 'warn', 'fg', 'fg-muted']) {
    assert.ok(ratio(token(name), token('surface')) >= 4.5, `${name} is below 4.5:1`)
  }
})

test('the default accent still reads against its own foreground', () => {
  // primary is a solid accent fill, so this pair is the one that has to work.
  assert.equal(ratio(token('accent-fg'), token('accent')).toFixed(4), '5.6398')
})

// A source check, which SPECS/CONVENTIONS.md 8.3 allows where a property cannot
// be observed from a test, provided the comment says what did prove it. What
// proved it: reading the built app on screen, and the 5.96:1 figure above.
test('danger is the outlined variant and primary is the only solid fill', () => {
  const danger = BUTTON.match(/danger: '([^']*)'/)
  const primary = BUTTON.match(/primary: '([^']*)'/)
  assert.ok(danger && primary, 'Button.tsx no longer declares its variants as string literals')

  assert.ok(danger![1].includes('bg-transparent'), 'danger must not be a solid fill')
  assert.ok(!/bg-danger(?![/-])/.test(danger![1]), 'danger must not set a solid bg-danger')
  assert.ok(danger![1].includes('text-danger'), 'danger text must carry the colour')
  assert.ok(danger![1].includes('border-danger'), 'danger must keep its outline')

  assert.ok(primary![1].includes('bg-accent'), 'primary is the solid accent button')
  assert.ok(!danger![1].includes('bg-accent'), 'danger must never use the accent colour')
})
