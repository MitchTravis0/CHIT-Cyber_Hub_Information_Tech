// The whole strength scorer. It lives in TypeScript and nowhere else: the meter
// has to move on every keystroke, so the half-typed password must never cross
// the bridge into Go. internal/tools/hibp does the breach request and no scoring.

import { COMMON_PASSWORDS } from './common-passwords'

export type StrengthTone = 'ok' | 'warn' | 'danger'

export interface Strength {
  /** Code points, not UTF-16 units, so an emoji counts as one character. */
  length: number
  /** How many different characters the password draws on. 0 for an empty one. */
  charsetSize: number
  /** Entropy in bits before any penalty, floored to a whole number. */
  rawBits: number
  /** rawBits minus the penalties, floored at 0. This drives the band. */
  bits: number
  /** 'Very weak' | 'Weak' | 'Reasonable' | 'Strong' | 'Very strong'. */
  band: string
  tone: StrengthTone
  /** One sentence of advice for the band. */
  advice: string
  /** Plain sentences saying what cost the password bits. Empty when nothing fired. */
  reasons: string[]
}

export interface Band {
  band: string
  tone: StrengthTone
  advice: string
}

/** Exact-match lookup, built once so rule 1 costs nothing per keystroke. */
const COMMON_SET = new Set(COMMON_PASSWORDS)

// The digit row 1234567890 is deliberately absent: the sequence rule already
// charges for a run like 1234 and charging twice for one weakness would be
// wrong. Only QWERTY is modelled; AZERTY, QWERTZ and Dvorak are out of scope.
const KEYBOARD_ROWS = ['qwertyuiop', 'asdfghjkl', 'zxcvbnm']

export function charsetSize(password: string): number {
  let lower = false
  let upper = false
  let digit = false
  let symbol = false
  let other = false

  for (const ch of password) {
    const cp = ch.codePointAt(0) ?? 0
    if (cp >= 0x61 && cp <= 0x7a) lower = true
    else if (cp >= 0x41 && cp <= 0x5a) upper = true
    else if (cp >= 0x30 && cp <= 0x39) digit = true
    else if (cp >= 0x20 && cp <= 0x7e) symbol = true
    // A flat 100 rather than the real Unicode count, because the true number is
    // enormous and would make a two-emoji password look unbreakable.
    else other = true
  }

  return (
    (lower ? 26 : 0) + (upper ? 26 : 0) + (digit ? 10 : 0) + (symbol ? 33 : 0) + (other ? 100 : 0)
  )
}

/** Maximal runs of 4 or more code points stepping by exactly +1 or exactly -1. */
export function sequenceRuns(password: string): string[] {
  const chars = [...password]
  const runs: string[] = []
  let start = 0
  let direction = 0

  for (let i = 1; i <= chars.length; i++) {
    const step =
      i < chars.length ? (chars[i].codePointAt(0) ?? 0) - (chars[i - 1].codePointAt(0) ?? 0) : 0
    if ((step === 1 || step === -1) && (direction === 0 || step === direction)) {
      direction = step
      continue
    }
    if (i - start >= 4) runs.push(chars.slice(start, i).join(''))
    if (step === 1 || step === -1) {
      start = i - 1
      direction = step
    } else {
      start = i
      direction = 0
    }
  }
  return runs
}

/** Maximal runs of 3 or more identical code points. */
export function repeatRuns(password: string): string[] {
  const chars = [...password]
  const runs: string[] = []
  let start = 0

  for (let i = 1; i <= chars.length; i++) {
    if (i < chars.length && chars[i] === chars[start]) continue
    if (i - start >= 3) runs.push(chars.slice(start, i).join(''))
    start = i
  }
  return runs
}

/** How many characters from start still spell a piece of one keyboard row. */
function rowMatchLength(text: string, start: number): number {
  let best = 0
  for (const row of KEYBOARD_ROWS) {
    const backwards = [...row].reverse().join('')
    for (const line of [row, backwards]) {
      let n = 0
      while (start + n < text.length && line.includes(text.slice(start, start + n + 1))) n++
      if (n > best) best = n
    }
  }
  return best
}

/** Distinct runs of 4 or more characters taken forwards or backwards from a
 *  keyboard row, lowercased. */
export function keyboardRuns(password: string): string[] {
  const text = password.toLowerCase()
  const runs: string[] = []
  let i = 0

  while (i < text.length) {
    const n = rowMatchLength(text, i)
    if (n >= 4) {
      const run = text.slice(i, i + n)
      if (!runs.includes(run)) runs.push(run)
      i += n
    } else {
      i++
    }
  }
  return runs
}

/** The first 4-digit substring that reads as a year from 1900 to 2099. */
export function yearIn(password: string): string {
  for (let i = 0; i + 4 <= password.length; i++) {
    const chunk = password.slice(i, i + 4)
    if (!/^\d{4}$/.test(chunk)) continue
    const year = Number(chunk)
    if (year >= 1900 && year <= 2099) return chunk
  }
  return ''
}

export function commonExact(password: string): boolean {
  return COMMON_SET.has(password.toLowerCase())
}

/** The longest listed password of 5 or more characters sitting inside this one. */
export function commonInside(password: string): string {
  const text = password.toLowerCase()
  let best = ''
  for (const entry of COMMON_PASSWORDS) {
    if (entry.length < 5 || entry.length <= best.length) continue
    if (text.includes(entry)) best = entry
  }
  return best
}

export function bandFor(bits: number): Band {
  if (bits <= 27) {
    return {
      band: 'Very weak',
      tone: 'danger',
      advice: 'An ordinary graphics card would work this out in seconds. Do not use it for anything.',
    }
  }
  if (bits <= 39) {
    return {
      band: 'Weak',
      tone: 'danger',
      advice:
        'This would fall to an offline guessing attack within hours. Use it for nothing that matters.',
    }
  }
  if (bits <= 59) {
    return {
      band: 'Reasonable',
      tone: 'warn',
      advice:
        'Acceptable for a low-value account. Not for email, banking, or anything with admin rights.',
    }
  }
  if (bits <= 79) {
    return {
      band: 'Strong',
      tone: 'ok',
      advice: 'Good enough for a normal work account. Still do not reuse it anywhere else.',
    }
  }
  return {
    band: 'Very strong',
    tone: 'ok',
    advice: "Strong enough for an administrator account or a password manager's master password.",
  }
}

export function score(password: string): Strength {
  const length = [...password].length
  const size = charsetSize(password)
  const rawBits = size === 0 ? 0 : Math.floor(length * Math.log2(size))

  // A password that is literally "password" does not need five more explanations.
  if (commonExact(password)) {
    return {
      length,
      charsetSize: size,
      rawBits,
      bits: 0,
      ...bandFor(0),
      reasons: [
        'This is one of the most common passwords there is, so it is tried in the first few seconds of any attack.',
      ],
    }
  }

  const reasons: string[] = []
  let penalty = 0

  const inside = commonInside(password)
  if (inside !== '') {
    penalty += 12
    reasons.push(`It is built around "${inside}", which is on the common-password list.`)
  }

  const sequences = sequenceRuns(password)
  if (sequences.length > 0) {
    penalty += Math.min(16, 8 * sequences.length)
    for (const run of sequences) {
      reasons.push(`It contains the run "${run}", which guessing tools try early.`)
    }
  }

  const repeats = repeatRuns(password)
  if (repeats.length > 0) {
    penalty += Math.min(12, 6 * repeats.length)
    for (const run of repeats) {
      reasons.push(`It repeats "${run}", which adds far less than it looks like it does.`)
    }
  }

  const keyboard = keyboardRuns(password)
  if (keyboard.length > 0) {
    penalty += Math.min(20, 10 * keyboard.length)
    for (const run of keyboard) {
      reasons.push(`It contains the keyboard run "${run}".`)
    }
  }

  const year = yearIn(password)
  if (year !== '') {
    penalty += 8
    reasons.push(`It contains the year ${year}, which is one of the first things guessed.`)
  }

  const bits = Math.max(0, rawBits - penalty)
  return { length, charsetSize: size, rawBits, bits, ...bandFor(bits), reasons }
}
