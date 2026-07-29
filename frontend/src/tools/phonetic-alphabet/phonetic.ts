export interface PhoneticOptions {
  /** Capitals are marked in the readout. Off renders every letter the same. */
  showCapitals: boolean
}

export interface PhoneticGroup {
  /** The run of non-space characters exactly as typed. */
  source: string
  /** One readout word per character, plus a trailing 'Space' on every group
   *  except the last. */
  elements: string[]
  /** The finished line, prefix included. */
  line: string
}

export interface PhoneticResult {
  groups: PhoneticGroup[]
  /** Every line joined with a newline. '' for empty input. */
  text: string
  /** Distinct characters with no table entry, in the order they first appeared. */
  unknown: string[]
  /** True when the input was longer than MAX_CHARACTERS and was cut. */
  truncated: boolean
}

export const MAX_CHARACTERS = 500

/** Keyed by the uppercase letter: LETTERS.A === 'Alfa'. The ICAO spellings, so
 *  Alfa and Juliett rather than Alpha and Juliet: "ph" and a single final "t"
 *  are read wrong by speakers whose first language is not English, which is
 *  exactly the situation this tool is used in. */
export const LETTERS: Record<string, string> = {
  A: 'Alfa',
  B: 'Bravo',
  C: 'Charlie',
  D: 'Delta',
  E: 'Echo',
  F: 'Foxtrot',
  G: 'Golf',
  H: 'Hotel',
  I: 'India',
  J: 'Juliett',
  K: 'Kilo',
  L: 'Lima',
  M: 'Mike',
  N: 'November',
  O: 'Oscar',
  P: 'Papa',
  Q: 'Quebec',
  R: 'Romeo',
  S: 'Sierra',
  T: 'Tango',
  U: 'Uniform',
  V: 'Victor',
  W: 'Whiskey',
  X: 'X-ray',
  Y: 'Yankee',
  Z: 'Zulu',
}

/** Keyed by the digit character: NUMBERS['9'] === 'Nine'. Plain English, not the
 *  aviation forms (Fower, Fife, Niner, Tree). The person on the other end of
 *  this call is a user or a supplier's call centre, not a pilot: "Fife" gets
 *  asked to be repeated more often than "Five" does. */
export const NUMBERS: Record<string, string> = {
  '0': 'Zero',
  '1': 'One',
  '2': 'Two',
  '3': 'Three',
  '4': 'Four',
  '5': 'Five',
  '6': 'Six',
  '7': 'Seven',
  '8': 'Eight',
  '9': 'Nine',
}

/** Keyed by the symbol character: MARKS['-'] === 'Dash'. Every printable ASCII
 *  character that is not a letter or a digit, plus the space. */
export const MARKS: Record<string, string> = {
  ' ': 'Space',
  '!': 'Exclamation mark',
  '"': 'Quote',
  '#': 'Hash',
  $: 'Dollar',
  '%': 'Percent',
  '&': 'Ampersand',
  "'": 'Apostrophe',
  '(': 'Open bracket',
  ')': 'Close bracket',
  '*': 'Star',
  '+': 'Plus',
  ',': 'Comma',
  '-': 'Dash',
  '.': 'Dot',
  '/': 'Slash',
  ':': 'Colon',
  ';': 'Semicolon',
  '<': 'Less than',
  '=': 'Equals',
  '>': 'Greater than',
  '?': 'Question mark',
  '@': 'At',
  '[': 'Open square bracket',
  '\\': 'Backslash',
  ']': 'Close square bracket',
  '^': 'Caret',
  _: 'Underscore',
  '`': 'Backtick',
  '{': 'Open curly bracket',
  '|': 'Pipe',
  '}': 'Close curly bracket',
  '~': 'Tilde',
}

const LETTER = /^[A-Za-z]$/
const CAPITAL = /^[A-Z]$/
// Only the ASCII whitespace characters count as whitespace. A non-breaking
// space out of a Word document is a character the user has to type, so it is
// reported as unknown rather than quietly swallowed as a separator.
const WHITESPACE = /[ \t\n\r\f\v]+/g

function known(character: string): boolean {
  return (
    LETTER.test(character) || NUMBERS[character] !== undefined || MARKS[character] !== undefined
  )
}

/** The readout for one character. Exported so the tests can drive the tables
 *  directly rather than through string parsing. */
export function elementFor(character: string, showCapitals: boolean): string {
  if (LETTER.test(character)) {
    const word = LETTERS[character.toUpperCase()]
    if (!showCapitals) return word
    return CAPITAL.test(character) ? `Capital ${word}` : word.toLowerCase()
  }
  const digit = NUMBERS[character]
  if (digit !== undefined) return digit
  const mark = MARKS[character]
  if (mark !== undefined) return mark
  return `Unknown character "${character}"`
}

export function convert(input: string, options: PhoneticOptions): PhoneticResult {
  // Split by code point, so an emoji is one character rather than two halves.
  const characters = Array.from(input)
  const truncated = characters.length > MAX_CHARACTERS
  const kept = characters.slice(0, MAX_CHARACTERS).join('').replace(WHITESPACE, ' ')
  const sources = kept.split(' ').filter((source) => source !== '')

  const unknown: string[] = []
  const groups = sources.map((source, index) => {
    const elements = Array.from(source).map((character) => {
      if (!known(character) && !unknown.includes(character)) unknown.push(character)
      return elementFor(character, options.showCapitals)
    })
    if (index < sources.length - 1) elements.push(MARKS[' '])
    const prefix = sources.length > 1 ? `${index + 1}. ` : ''
    return { source, elements, line: `${prefix}${source}  =  ${elements.join(' - ')}` }
  })

  return { groups, text: groups.map((group) => group.line).join('\n'), unknown, truncated }
}
