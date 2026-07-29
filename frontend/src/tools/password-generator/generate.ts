import { randomIndex, shuffle, type RandomFill } from './random'
import { WORDS } from './wordlist'

export const LOWER = 'abcdefghijklmnopqrstuvwxyz'
export const UPPER = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
export const DIGITS = '0123456789'
// Quote, apostrophe, backslash, pipe, comma, semicolon, colon, angle brackets,
// full stop, slash, backtick and space are left out because they break a CSV
// import, a shell command, an XML config or a paste out of a chat window, and
// dropping them costs under a third of a bit per character.
export const SYMBOLS = '!#$%&()*+-=?@[]^_{}~'
export const LOOKALIKES = 'Il1O0o'

export const MIN_LENGTH = 8
export const MAX_LENGTH = 128
export const MIN_WORDS = 3
export const MAX_WORDS = 12
export const MIN_COUNT = 1
export const MAX_COUNT = 20

export type SeparatorValue = '-' | '.' | '_' | ' '

export interface CharOptions {
  length: number
  lower: boolean
  upper: boolean
  digits: boolean
  symbols: boolean
  excludeLookAlikes: boolean
}

export interface PhraseOptions {
  words: number
  separator: SeparatorValue
  capitalise: boolean
  addNumber: boolean
}

export interface Strength {
  bits: number
  label: string
  tone: 'danger' | 'warn' | 'ok'
  advice: string
}

function without(pool: string, excludeLookAlikes: boolean): string {
  if (!excludeLookAlikes) return pool
  return [...pool].filter((character) => !LOOKALIKES.includes(character)).join('')
}

/** Every enabled class's own pool, in the fixed class order. */
function classPools(options: CharOptions): string[] {
  const pools: string[] = []
  if (options.lower) pools.push(without(LOWER, options.excludeLookAlikes))
  if (options.upper) pools.push(without(UPPER, options.excludeLookAlikes))
  if (options.digits) pools.push(without(DIGITS, options.excludeLookAlikes))
  if (options.symbols) pools.push(without(SYMBOLS, options.excludeLookAlikes))
  return pools
}

/** The characters a password may be built from, in class order. Empty when no
 *  class is enabled. */
export function poolFor(options: CharOptions): string {
  return classPools(options).join('')
}

/** Null when the options are usable, otherwise the sentence to show. */
export function optionsError(options: CharOptions): string | null {
  if (poolFor(options) === '') {
    return 'Pick at least one kind of character: lowercase, uppercase, digits or symbols.'
  }
  return null
}

/** Precondition: poolFor(options) is not empty and options.length is in range. */
export function generatePassword(options: CharOptions, fill?: RandomFill): string {
  const pool = poolFor(options)
  const picked: string[] = []
  // One character from every enabled class first, so the result satisfies
  // "must contain a digit" style rules, then shuffled so those characters are
  // not sitting in known positions.
  for (const classPool of classPools(options)) {
    picked.push(classPool[randomIndex(classPool.length, fill)])
  }
  while (picked.length < options.length) {
    picked.push(pool[randomIndex(pool.length, fill)])
  }
  shuffle(picked, fill)
  return picked.join('')
}

export function generatePassphrase(options: PhraseOptions, fill?: RandomFill): string {
  const parts: string[] = []
  for (let i = 0; i < options.words; i++) {
    const word = WORDS[randomIndex(WORDS.length, fill)]
    parts.push(options.capitalise ? word[0].toUpperCase() + word.slice(1) : word)
  }
  if (options.addNumber) {
    parts.push(String(randomIndex(100, fill)).padStart(2, '0'))
  }
  return parts.join(options.separator)
}

// Guaranteeing one character per class makes the true entropy very slightly
// lower than length * log2(pool), by well under one bit at these lengths. The
// figure below is the one every other tool and every auditor quotes, so it is
// the one shown.
export function charEntropyBits(options: CharOptions): number {
  return Math.round(options.length * Math.log2(poolFor(options).length))
}

// Capitalising adds nothing: the same transform runs over every word every
// time, so anyone who knows the tool knows the pattern.
export function phraseEntropyBits(options: PhraseOptions): number {
  const bits = options.words * Math.log2(WORDS.length) + (options.addNumber ? Math.log2(100) : 0)
  return Math.round(bits)
}

export function strengthFor(bits: number): Strength {
  if (bits < 50) {
    return {
      bits,
      label: 'Weak',
      tone: 'danger',
      advice: 'A machine could work this out. Make it longer, or turn on more character types.',
    }
  }
  if (bits < 75) {
    return {
      bits,
      label: 'Reasonable',
      tone: 'warn',
      advice: 'Fine for an everyday account that also has multi-factor turned on.',
    }
  }
  if (bits < 100) {
    return {
      bits,
      label: 'Strong',
      tone: 'ok',
      advice: 'Good for an admin account, a device password or a Wi-Fi key.',
    }
  }
  return {
    bits,
    label: 'Very strong',
    tone: 'ok',
    advice: 'More than anything on a normal network needs.',
  }
}

export interface ParsedNumber {
  ok: boolean
  value: number
  error: string | null
}

function parseWhole(
  raw: string,
  min: number,
  max: number,
  notWhole: string,
  tooLow: string,
  tooHigh: string,
): ParsedNumber {
  const text = raw.trim()
  if (text === '' || !Number.isInteger(Number(text))) {
    return { ok: false, value: 0, error: notWhole }
  }
  const value = Number(text)
  if (value < min) return { ok: false, value: 0, error: tooLow }
  if (value > max) return { ok: false, value: 0, error: tooHigh }
  return { ok: true, value, error: null }
}

export function parseLength(raw: string): ParsedNumber {
  return parseWhole(
    raw,
    MIN_LENGTH,
    MAX_LENGTH,
    'Type how many characters you want, between 8 and 128.',
    '8 characters is the shortest this will make. Anything shorter is not worth generating.',
    '128 characters is the longest this will make. Very few systems accept more.',
  )
}

export function parseWordCount(raw: string): ParsedNumber {
  return parseWhole(
    raw,
    MIN_WORDS,
    MAX_WORDS,
    'Type how many words you want, between 3 and 12.',
    'Use at least 3 words. Fewer than that is quick to guess.',
    '12 words is the most this will make. That is already far more than anything needs.',
  )
}

export function parseCount(raw: string): ParsedNumber {
  return parseWhole(
    raw,
    MIN_COUNT,
    MAX_COUNT,
    'Type how many passwords you want, between 1 and 20.',
    'Type how many passwords you want, between 1 and 20.',
    '20 at a time is the most. Press Generate again if you need more.',
  )
}
