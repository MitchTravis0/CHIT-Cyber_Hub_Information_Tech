export interface KeyDef {
  /** The KeyboardEvent.code this key produces. Unique across the layout. */
  code: string
  /** The face label, e.g. 'E', 'Enter', 'F1'. */
  label: string
  /** Width in key units. 1 is a standard letter key. */
  width: number
  /** Set when the operating system or the webview takes this key first. */
  swallowed?: string
}

export interface KeyRow {
  keys: KeyDef[]
}

export interface KeyBlock {
  name: string
  rows: KeyRow[]
}

// Reasons a key never reaches the page. Shown as a title on the dashed key and
// listed at the bottom of the page, so an unlit dashed key is never read as a
// fault.
const FN_REASON = 'Handled inside the keyboard itself and never reaches the computer.'
const PRINT_REASON = 'The operating system takes this for screenshots.'
const META_REASON = 'Opens the Start menu or Spotlight, so it may not stay held.'
const FULLSCREEN_REASON = 'May switch the window to full screen instead.'

export const OS_COMBINATIONS: Array<{ combination: string; reason: string }> = [
  { combination: 'Fn', reason: FN_REASON },
  {
    combination: 'Alt+Tab, Ctrl+Alt+Delete, Win+L',
    reason: 'Taken by the operating system before any application sees it.',
  },
]

function key(code: string, label: string, width = 1, swallowed?: string): KeyDef {
  return swallowed === undefined ? { code, label, width } : { code, label, width, swallowed }
}

function letters(text: string): KeyDef[] {
  return text.split('').map((c) => key('Key' + c, c))
}

/**
 * The standard ANSI 104-key layout, in four blocks.
 *
 * Every code is a UI Events KeyboardEvent.code value, checked against the
 * WebKit engine's own string table rather than written from memory: Backquote
 * (not Grave), Quote (not Apostrophe), ContextMenu (not Menu) and NumpadDecimal
 * (not NumpadPeriod) are the four most often got wrong.
 *
 * Each of the five main rows sums to 15 units, which is what keeps the picture
 * rectangular.
 */
export const LAYOUT: KeyBlock[] = [
  {
    name: 'main',
    rows: [
      {
        keys: [
          key('Escape', 'Esc'),
          key('F1', 'F1'),
          key('F2', 'F2'),
          key('F3', 'F3'),
          key('F4', 'F4'),
          key('F5', 'F5'),
          key('F6', 'F6'),
          key('F7', 'F7'),
          key('F8', 'F8'),
          key('F9', 'F9'),
          key('F10', 'F10'),
          key('F11', 'F11', 1, FULLSCREEN_REASON),
          key('F12', 'F12'),
          key('PrintScreen', 'PrtSc', 1, PRINT_REASON),
          key('ScrollLock', 'ScrLk'),
        ],
      },
      {
        keys: [
          key('Backquote', '`'),
          key('Digit1', '1'),
          key('Digit2', '2'),
          key('Digit3', '3'),
          key('Digit4', '4'),
          key('Digit5', '5'),
          key('Digit6', '6'),
          key('Digit7', '7'),
          key('Digit8', '8'),
          key('Digit9', '9'),
          key('Digit0', '0'),
          key('Minus', '-'),
          key('Equal', '='),
          key('Backspace', 'Backspace', 2),
        ],
      },
      {
        keys: [
          key('Tab', 'Tab', 1.5),
          ...letters('QWERTYUIOP'),
          key('BracketLeft', '['),
          key('BracketRight', ']'),
          key('Backslash', '\\', 1.5),
        ],
      },
      {
        keys: [
          key('CapsLock', 'Caps', 1.75),
          ...letters('ASDFGHJKL'),
          key('Semicolon', ';'),
          key('Quote', "'"),
          key('Enter', 'Enter', 2.25),
        ],
      },
      {
        keys: [
          key('ShiftLeft', 'Shift', 2.25),
          ...letters('ZXCVBNM'),
          key('Comma', ','),
          key('Period', '.'),
          key('Slash', '/'),
          key('ShiftRight', 'Shift', 2.75),
        ],
      },
      {
        keys: [
          key('ControlLeft', 'Ctrl', 1.25),
          key('MetaLeft', 'Win', 1.25, META_REASON),
          key('AltLeft', 'Alt', 1.25),
          key('Space', 'Space', 6.25),
          key('AltRight', 'Alt', 1.25),
          key('MetaRight', 'Win', 1.25, META_REASON),
          key('ContextMenu', 'Menu', 1.25),
          key('ControlRight', 'Ctrl', 1.25),
        ],
      },
    ],
  },
  {
    name: 'navigation',
    rows: [
      { keys: [key('Pause', 'Pause'), key('Insert', 'Ins'), key('Home', 'Home')] },
      { keys: [key('PageUp', 'PgUp'), key('Delete', 'Del'), key('End', 'End')] },
      { keys: [key('PageDown', 'PgDn')] },
    ],
  },
  {
    name: 'arrows',
    rows: [
      { keys: [key('ArrowUp', 'Up')] },
      { keys: [key('ArrowLeft', 'Left'), key('ArrowDown', 'Down'), key('ArrowRight', 'Right')] },
    ],
  },
  {
    name: 'keypad',
    rows: [
      {
        keys: [
          key('NumLock', 'Num'),
          key('NumpadDivide', '/'),
          key('NumpadMultiply', '*'),
          key('NumpadSubtract', '-'),
        ],
      },
      {
        keys: [
          key('Numpad7', '7'),
          key('Numpad8', '8'),
          key('Numpad9', '9'),
          key('NumpadAdd', '+'),
        ],
      },
      { keys: [key('Numpad4', '4'), key('Numpad5', '5'), key('Numpad6', '6')] },
      {
        keys: [
          key('Numpad1', '1'),
          key('Numpad2', '2'),
          key('Numpad3', '3'),
          key('NumpadEnter', 'Enter'),
        ],
      },
      { keys: [key('Numpad0', '0', 2), key('NumpadDecimal', '.')] },
    ],
  },
]

/** Every code in LAYOUT, in the order the keys are drawn. */
export function allCodes(): string[] {
  const out: string[] = []
  for (const block of LAYOUT) {
    for (const row of block.rows) {
      for (const k of row.keys) out.push(k.code)
    }
  }
  return out
}

/** Every key that CHIT knows the operating system may keep. */
export function swallowedKeys(): KeyDef[] {
  return LAYOUT.flatMap((block) => block.rows.flatMap((row) => row.keys)).filter(
    (k) => k.swallowed !== undefined,
  )
}
