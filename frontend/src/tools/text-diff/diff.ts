/**
 * Line-by-line comparison of two texts.
 *
 * The comparison is Myers 1986, the greedy O(ND) forward pass with a saved
 * trace, run over the middle once the common prefix and suffix are trimmed off.
 * Trimming is what makes the usual case (a 900 line config with two changed
 * lines) cost almost nothing, and it is why the edit cap below is rarely
 * reached.
 */

export interface DiffOptions {
  /** Trim both ends of a line and collapse internal runs of whitespace. */
  ignoreWhitespace: boolean
  /** Compare lines without regard to capitalisation. */
  ignoreCase: boolean
}

export type DiffKind = 'same' | 'added' | 'removed'

export interface DiffRow {
  kind: DiffKind
  /** 1-based line number in the left text. Null on an added line. */
  left: number | null
  /** 1-based line number in the right text. Null on a removed line. */
  right: number | null
  /** The line as it appears in its own side, with no line ending. */
  text: string
}

export interface DiffResult {
  rows: DiffRow[]
  added: number
  removed: number
  same: number
  /** No added and no removed rows. */
  identical: boolean
  /** Identical, but only because an ignore rule was switched on. */
  differsOnlyByRules: boolean
  /** Either side was longer than MAX_LINES and was cut. */
  truncated: boolean
  /** The middle needed more than MAX_EDITS changes, so it was not lined up. */
  tooDifferent: boolean
}

export type BlockKind = 'rows' | 'gap'

export interface DiffBlock {
  kind: BlockKind
  /** The rows to render. Empty for a gap. */
  rows: DiffRow[]
  /** How many unchanged lines the gap stands for. 0 for a rows block. */
  skipped: number
}

/** Lines per side. Anything past this is cut and `truncated` is set. */
export const MAX_LINES = 10000
/** The most insertions plus deletions the diff will line up one by one. */
export const MAX_EDITS = 3000
/** Unchanged lines kept either side of a change. */
export const CONTEXT_LINES = 3
/** Rows the page renders before it stops and says so. */
export const MAX_RENDERED_ROWS = 2000

export function splitLines(text: string): string[] {
  if (text === '') return []
  const lines = text.split(/\r\n|\r|\n/)
  // A file that ends with a newline has the same number of lines as one that
  // does not, which is what wc -l and diff both assume.
  if (lines[lines.length - 1] === '') lines.pop()
  return lines
}

export function normalizeLine(line: string, options: DiffOptions): string {
  let key = line
  if (options.ignoreWhitespace) key = key.trim().replace(/\s+/g, ' ')
  if (options.ignoreCase) key = key.toLowerCase()
  return key
}

type Edit = { kind: DiffKind; a: number; b: number }

/**
 * Myers over a and b, compared by key. Null when it would take more than
 * MAX_EDITS insertions and deletions, which is the caller's signal to give up
 * and show a plain replacement instead.
 */
function myers(a: string[], b: string[]): Edit[] | null {
  const n = a.length
  const m = b.length
  if (n === 0) return b.map((_, j) => ({ kind: 'added' as const, a: -1, b: j }))
  if (m === 0) return a.map((_, i) => ({ kind: 'removed' as const, a: i, b: -1 }))

  const max = n + m
  const v = new Int32Array(2 * max + 1)
  const trace: Int32Array[] = []
  const limit = Math.min(max, MAX_EDITS)

  for (let d = 0; d <= limit; d++) {
    // Only the live window is kept: the whole v array snapshotted for every d
    // would allocate hundreds of megabytes at the sizes this tool allows.
    trace.push(v.slice(max - d, max + d + 1))
    for (let k = -d; k <= d; k += 2) {
      let x: number
      if (k === -d || (k !== d && v[max + k - 1] < v[max + k + 1])) {
        x = v[max + k + 1]
      } else {
        x = v[max + k - 1] + 1
      }
      let y = x - k
      while (x < n && y < m && a[x] === b[y]) {
        x++
        y++
      }
      v[max + k] = x
      if (x >= n && y >= m) return backtrack(trace, n, m)
    }
  }
  return null
}

function backtrack(trace: Int32Array[], n: number, m: number): Edit[] {
  const out: Edit[] = []
  let x = n
  let y = m
  for (let d = trace.length - 1; d >= 0; d--) {
    const vd = trace[d]
    const k = x - y
    let prevK: number
    if (k === -d || (k !== d && vd[k - 1 + d] < vd[k + 1 + d])) {
      prevK = k + 1
    } else {
      prevK = k - 1
    }
    const prevX = vd[prevK + d]
    const prevY = prevX - prevK
    while (x > prevX && y > prevY) {
      out.push({ kind: 'same', a: x - 1, b: y - 1 })
      x--
      y--
    }
    if (d > 0) {
      if (x === prevX) out.push({ kind: 'added', a: -1, b: y - 1 })
      else out.push({ kind: 'removed', a: x - 1, b: -1 })
    }
    x = prevX
    y = prevY
  }
  out.reverse()
  return out
}

export function diffTexts(left: string, right: string, options: DiffOptions): DiffResult {
  let a = splitLines(left)
  let b = splitLines(right)
  const truncated = a.length > MAX_LINES || b.length > MAX_LINES
  if (a.length > MAX_LINES) a = a.slice(0, MAX_LINES)
  if (b.length > MAX_LINES) b = b.slice(0, MAX_LINES)

  const keyA = a.map((line) => normalizeLine(line, options))
  const keyB = b.map((line) => normalizeLine(line, options))

  const rows: DiffRow[] = []
  let lo = 0
  while (lo < a.length && lo < b.length && keyA[lo] === keyB[lo]) {
    rows.push({ kind: 'same', left: lo + 1, right: lo + 1, text: a[lo] })
    lo++
  }

  let tail = 0
  while (
    a.length - tail > lo &&
    b.length - tail > lo &&
    keyA[a.length - 1 - tail] === keyB[b.length - 1 - tail]
  ) {
    tail++
  }
  const aEnd = a.length - tail
  const bEnd = b.length - tail

  let tooDifferent = false
  const edits = myers(keyA.slice(lo, aEnd), keyB.slice(lo, bEnd))
  if (edits === null) {
    tooDifferent = true
    for (let i = lo; i < aEnd; i++) rows.push({ kind: 'removed', left: i + 1, right: null, text: a[i] })
    for (let j = lo; j < bEnd; j++) rows.push({ kind: 'added', left: null, right: j + 1, text: b[j] })
  } else {
    for (const edit of edits) {
      if (edit.kind === 'same') {
        rows.push({ kind: 'same', left: lo + edit.a + 1, right: lo + edit.b + 1, text: a[lo + edit.a] })
      } else if (edit.kind === 'removed') {
        rows.push({ kind: 'removed', left: lo + edit.a + 1, right: null, text: a[lo + edit.a] })
      } else {
        rows.push({ kind: 'added', left: null, right: lo + edit.b + 1, text: b[lo + edit.b] })
      }
    }
  }

  for (let t = tail - 1; t >= 0; t--) {
    rows.push({ kind: 'same', left: a.length - t, right: b.length - t, text: a[a.length - 1 - t] })
  }

  let added = 0
  let removed = 0
  let same = 0
  for (const row of rows) {
    if (row.kind === 'added') added++
    else if (row.kind === 'removed') removed++
    else same++
  }
  const identical = added === 0 && removed === 0
  // Comparing the joined lines rather than the raw strings, so a trailing
  // newline or a CRLF on one side alone is not mistaken for a rule doing work.
  return {
    rows,
    added,
    removed,
    same,
    identical,
    differsOnlyByRules: identical && a.join('\n') !== b.join('\n'),
    truncated,
    tooDifferent,
  }
}

export function collapse(rows: DiffRow[], context: number): DiffBlock[] {
  const blocks: DiffBlock[] = []
  const pushRows = (kept: DiffRow[]) => {
    if (kept.length === 0) return
    const last = blocks[blocks.length - 1]
    if (last !== undefined && last.kind === 'rows') last.rows.push(...kept)
    else blocks.push({ kind: 'rows', rows: kept, skipped: 0 })
  }

  let i = 0
  while (i < rows.length) {
    if (rows[i].kind !== 'same') {
      const start = i
      while (i < rows.length && rows[i].kind !== 'same') i++
      pushRows(rows.slice(start, i))
      continue
    }
    const start = i
    while (i < rows.length && rows[i].kind === 'same') i++
    const run = rows.slice(start, i)
    const atStart = start === 0
    const atEnd = i === rows.length
    const before = atStart ? 0 : context
    const after = atEnd ? 0 : context
    if (run.length <= before + after) {
      pushRows(run)
      continue
    }
    pushRows(run.slice(0, before))
    blocks.push({ kind: 'gap', rows: [], skipped: run.length - before - after })
    pushRows(run.slice(run.length - after))
  }
  return blocks
}

export function changedText(rows: DiffRow[]): string {
  const out: string[] = []
  for (const row of rows) {
    if (row.kind === 'removed') out.push(`-${row.text}`)
    else if (row.kind === 'added') out.push(`+${row.text}`)
  }
  return out.join('\n')
}
