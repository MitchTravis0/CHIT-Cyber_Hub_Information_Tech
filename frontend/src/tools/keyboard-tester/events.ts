/**
 * Narrow shapes carrying only the fields this module reads. They exist so the
 * tests can build one without a DOM, which the test runner does not have: that
 * is the difference between this logic being tested and not.
 */
export interface KeyboardEventLike {
  code: string
  key: string
  keyCode: number
  location: number
  repeat: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  metaKey: boolean
}

export interface ModifierState {
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  metaKey: boolean
}

export interface KeyInfo {
  code: string
  key: string
  keyCode: number
  location: string
  repeat: boolean
  modifiers: string[]
}

export interface LogEntry {
  kind: string
  code: string
  detail: string
  atMs: number
}

const LOCATIONS: Record<number, string> = {
  0: 'standard',
  1: 'left',
  2: 'right',
  3: 'numpad',
}

export function locationName(location: number): string {
  return LOCATIONS[location] ?? 'standard'
}

/** A fixed order, so two readings of the same combination look the same. */
export function modifierList(e: ModifierState): string[] {
  const out: string[] = []
  if (e.ctrlKey) out.push('Ctrl')
  if (e.altKey) out.push('Alt')
  if (e.shiftKey) out.push('Shift')
  if (e.metaKey) out.push('Meta')
  return out
}

export function describeKey(e: KeyboardEventLike): KeyInfo {
  return {
    code: e.code,
    key: e.key,
    keyCode: e.keyCode,
    location: locationName(e.location),
    repeat: e.repeat,
    modifiers: modifierList(e),
  }
}

/**
 * The counter above the keyboard. A key pressed that is not on the drawn board
 * (an ISO key, a laptop Fn) must not push the count above the total.
 */
export function coverageLine(seen: Set<string>, codes: string[], total: number): string {
  let found = 0
  for (const code of codes) {
    if (seen.has(code)) found++
  }
  if (found >= total) {
    return 'Every key on this keyboard reported. Nothing is stuck.'
  }
  const left = total - found
  return `${found} of ${total} keys seen. ${left} to go.`
}

/** One line of the event log. */
export function logLine(entry: LogEntry, previousAtMs: number | null): string {
  const gap = previousAtMs === null ? 0 : Math.max(0, Math.round(entry.atMs - previousAtMs))
  const head = `+${gap} ms  ${entry.kind}  ${entry.code}`
  return entry.detail === '' ? head : `${head}  ${entry.detail}`
}

/**
 * What Copy log puts on the clipboard: newest first, exactly as displayed. The
 * gaps are measured against the entry below each line, which is the one that
 * happened before it.
 */
export function logText(entries: LogEntry[]): string {
  return entries
    .map((entry, index) => {
      const previous = index + 1 < entries.length ? entries[index + 1].atMs : null
      return logLine(entry, previous)
    })
    .join('\n')
}

const BUTTONS: Record<number, string> = {
  0: 'Left',
  1: 'Middle',
  2: 'Right',
  3: 'Back',
  4: 'Forward',
}

export function buttonName(button: number): string {
  return BUTTONS[button] ?? `Button ${button + 1}`
}

const WHEEL_UNITS: Record<number, string> = {
  0: 'pixels',
  1: 'lines',
  2: 'pages',
}

export function wheelLabel(x: number, y: number, deltaMode: number): string {
  const unit = WHEEL_UNITS[deltaMode] ?? 'units'
  return `x ${Math.round(x)}, y ${Math.round(y)} (${unit})`
}

/**
 * A modifier reporting as held with nothing physically down is either a stuck
 * key or Sticky Keys, which users turn on by accident by pressing Shift five
 * times. The held set is cleared on blur, so returning from Alt+Tab does not
 * raise a phantom warning.
 */
export function stuckModifiers(held: Set<string>, anyKeyDown: boolean): string[] {
  if (anyKeyDown) return []
  return ['Ctrl', 'Alt', 'Shift', 'Meta'].filter((name) => held.has(name))
}

/** The sentence shown for a stuck modifier. */
export function stuckSentence(names: string[]): string {
  if (names.length === 0) return ''
  const list = names.length === 1 ? names[0] : names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1]
  const verb = names.length === 1 ? 'is' : 'are'
  return `${list} ${verb} reporting as held down. If nothing is pressed, check for a stuck key or turn Sticky Keys off in accessibility settings.`
}
