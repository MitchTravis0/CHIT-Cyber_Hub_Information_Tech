// Ticket notes: schema, the timestamp, validation, the write-up formatter and
// the import merge. All pure, so the whole thing is testable without a DOM.

export const NOTES_NAMESPACE = 'ticket-notes'
export const NOTES_DOC_VERSION = 1
export const NOTES_EXPORT_KIND = 'chit/ticket-notes'

export const MAX_IMPORT_BYTES = 5 * 1024 * 1024

export const MAX_REF = 40
export const MAX_TITLE = 120
export const MAX_LONG = 4000
export const MAX_ENTRY = 500

export interface Entry {
  id: string
  stamp: string
  text: string
}

export interface Note {
  id: string
  ref: string
  title: string
  issue: string
  resolution: string
  entries: Entry[]
  createdAt: string
  updatedAt: string
}

export interface NotesDoc {
  version: number
  notes: Note[]
}

export type Style = 'text' | 'markdown'

export const REF_LONG_MESSAGE = 'Keep the ticket reference to 40 characters or fewer.'
export const TITLE_LONG_MESSAGE = 'Keep the title to 120 characters or fewer.'
export const LONG_MESSAGE = 'Keep this to 4000 characters or fewer.'
export const ENTRY_LONG_MESSAGE = 'Keep one step to 500 characters or fewer. Add a second step instead.'

export const NOT_A_NOTE_FILE_MESSAGE =
  'That file is not a CHIT ticket note file. Export one from this tool to see the format.'
export const NEWER_FILE_MESSAGE =
  'That file was written by a newer version of CHIT and could not be read. Update CHIT, or export it again from the machine that wrote it.'
export const NEWER_VERSION_MESSAGE =
  'These ticket notes were written by a newer version of CHIT and could not be read. Update CHIT, or export them again from the machine that wrote them.'
export const TOO_BIG_MESSAGE =
  'That file is larger than 5 MB. A ticket note file is normally a few kilobytes, so this is probably not one.'
export const NO_STORE_MESSAGE =
  'Saving needs the desktop app. Anything you type here will be gone when you close the page.'
export const SAVE_FAILED_MESSAGE =
  'The notes could not be saved. Check that the CHIT data folder is still there.'

export function newNoteId(): string {
  return 'note-' + randomHex()
}

export function newEntryId(): string {
  return 'step-' + randomHex()
}

function randomHex(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(4))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

/**
 * YYYY-MM-DD HH:MM from the local clock, stored as text on purpose.
 *
 * A tech writing a report at a client site means the time on the wall in front
 * of them. Re-rendering an ISO instant in a different time zone six months
 * later would silently change what the report says happened when.
 */
export function stampOf(date: Date): string {
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    ` ${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

const STAMP_PATTERN = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2})$/

/** Milliseconds for a stamp, or null when it is not one this tool wrote. A
 *  stamp a tech corrected by hand is still shown; it just cannot be measured. */
export function parseStamp(text: string): number | null {
  const match = STAMP_PATTERN.exec(text.trim())
  if (match === null) return null
  const [, year, month, day, hour, minute] = match.map(Number)
  if (month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59) return null
  const date = new Date(year, month - 1, day, hour, minute)
  if (date.getMonth() !== month - 1 || date.getDate() !== day) return null
  return date.getTime()
}

/** The span from the earliest step to the latest, in milliseconds, or null when
 *  fewer than two steps carry a stamp this tool can read. */
export function elapsed(entries: Entry[]): number | null {
  const times: number[] = []
  for (const entry of entries) {
    const at = parseStamp(entry.stamp)
    if (at !== null) times.push(at)
  }
  if (times.length < 2) return null
  return Math.max(...times) - Math.min(...times)
}

export function emptyNote(id: string, now: string): Note {
  return {
    id,
    ref: '',
    title: '',
    issue: '',
    resolution: '',
    entries: [],
    createdAt: now,
    updatedAt: now,
  }
}

export type NoteErrors = Partial<Record<'ref' | 'title' | 'issue' | 'resolution', string>>

export function validateNote(note: Pick<Note, 'ref' | 'title' | 'issue' | 'resolution'>): NoteErrors {
  const errors: NoteErrors = {}
  if (note.ref.length > MAX_REF) errors.ref = REF_LONG_MESSAGE
  if (note.title.length > MAX_TITLE) errors.title = TITLE_LONG_MESSAGE
  if (note.issue.length > MAX_LONG) errors.issue = LONG_MESSAGE
  if (note.resolution.length > MAX_LONG) errors.resolution = LONG_MESSAGE
  return errors
}

export function validateEntry(text: string): string | undefined {
  const trimmed = text.trim()
  if (trimmed === '') return 'Type what you just did, then press Add step.'
  if (trimmed.length > MAX_ENTRY) return ENTRY_LONG_MESSAGE
  return undefined
}

/** An entry is one line in the write-up, so an embedded newline is folded away
 *  rather than breaking the alignment of every ticket system worth naming. */
function oneLine(text: string): string {
  return text.replace(/\s*\n\s*/g, ' ').trim()
}

function heading(note: Note): string {
  const ref = note.ref.trim()
  const title = note.title.trim()
  if (ref !== '' && title !== '') return `${ref} - ${title}`
  if (ref !== '') return ref
  if (title !== '') return title
  return 'Ticket note'
}

/** The write-up, in the shape a ticket system wants. Sections with nothing in
 *  them are left out entirely, heading included. */
export function formatNote(note: Note, style: Style): string {
  const bold = (text: string) => (style === 'markdown' ? `**${text}**` : text)
  const blocks: string[] = [bold(heading(note))]

  const issue = note.issue.trim()
  if (issue !== '') {
    blocks.push(style === 'markdown' ? `${bold('Issue')}\n\n${issue}` : `${bold('Issue')}\n${issue}`)
  }

  const steps = note.entries.filter((entry) => oneLine(entry.text) !== '')
  if (steps.length > 0) {
    const lines = steps.map((entry) =>
      style === 'markdown'
        ? `- \`${entry.stamp}\` ${oneLine(entry.text)}`
        : `${entry.stamp}  ${oneLine(entry.text)}`,
    )
    blocks.push(
      style === 'markdown'
        ? `${bold('Steps taken')}\n\n${lines.join('\n')}`
        : `${bold('Steps taken')}\n${lines.join('\n')}`,
    )
  }

  const resolution = note.resolution.trim()
  if (resolution !== '') {
    blocks.push(
      style === 'markdown'
        ? `${bold('Resolution')}\n\n${resolution}`
        : `${bold('Resolution')}\n${resolution}`,
    )
  }

  return blocks.join('\n\n')
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function readEntry(raw: unknown): Entry | null {
  if (typeof raw !== 'object' || raw === null) return null
  const entry = raw as Record<string, unknown>
  const body = text(entry.text).trim()
  if (body === '') return null
  return { id: text(entry.id).trim(), stamp: text(entry.stamp).trim(), text: body }
}

function readNote(raw: unknown): Note | null {
  if (typeof raw !== 'object' || raw === null) return null
  const note = raw as Record<string, unknown>
  const entries: Entry[] = []
  if (Array.isArray(note.entries)) {
    for (const item of note.entries) {
      const entry = readEntry(item)
      if (entry !== null) entries.push(entry)
    }
  }
  const out: Note = {
    id: text(note.id).trim(),
    ref: text(note.ref).trim(),
    title: text(note.title).trim(),
    issue: text(note.issue),
    resolution: text(note.resolution),
    entries,
    createdAt: text(note.createdAt).trim(),
    updatedAt: text(note.updatedAt).trim(),
  }
  const empty =
    out.ref === '' &&
    out.title === '' &&
    out.issue.trim() === '' &&
    out.resolution.trim() === '' &&
    entries.length === 0
  return empty ? null : out
}

function readList(raw: unknown): Note[] | null {
  if (typeof raw !== 'object' || raw === null) return null
  const doc = raw as Record<string, unknown>
  if (!Array.isArray(doc.notes)) return null
  const notes: Note[] = []
  for (const item of doc.notes) {
    const note = readNote(item)
    if (note !== null) notes.push(note)
  }
  return notes
}

export function migrateDoc(raw: unknown): NotesDoc {
  const empty: NotesDoc = { version: NOTES_DOC_VERSION, notes: [] }
  if (typeof raw !== 'object' || raw === null) return empty
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > NOTES_DOC_VERSION) return empty
  return { version: NOTES_DOC_VERSION, notes: readList(doc) ?? [] }
}

export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > NOTES_DOC_VERSION) return NEWER_VERSION_MESSAGE
  return ''
}

export function ensureIds(notes: Note[], makeNoteId: () => string, makeEntryId: () => string): Note[] {
  return notes.map((note) => ({
    ...note,
    id: note.id === '' ? makeNoteId() : note.id,
    entries: note.entries.map((entry) =>
      entry.id === '' ? { ...entry, id: makeEntryId() } : entry,
    ),
  }))
}

export function sortNotes(notes: Note[]): Note[] {
  return notes.slice().sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
}

export function filterNotes(notes: Note[], query: string): Note[] {
  const needle = query.trim().toLowerCase()
  if (needle === '') return notes
  return notes.filter((note) => `${note.ref} ${note.title}`.toLowerCase().includes(needle))
}

export interface MergeReport {
  added: number
  skipped: number
  notes: Note[]
  error: string
}

/**
 * Adds imported notes to the current list.
 *
 * A note whose id is already here is skipped, never merged: two people writing
 * up the same ticket produce two different, both-correct accounts, and
 * interleaving their steps would fabricate a history that never happened.
 */
export function mergeNotes(
  current: Note[],
  incoming: unknown,
  makeId: () => string,
  now: string,
): MergeReport {
  const parsed = readList(incoming)
  if (parsed === null) {
    return { added: 0, skipped: 0, notes: current, error: NOT_A_NOTE_FILE_MESSAGE }
  }

  const raw = (incoming as Record<string, unknown>).notes
  let skipped = Array.isArray(raw) ? raw.length - parsed.length : 0

  const merged = current.map((note) => ({ ...note }))
  const ids = new Set(merged.map((note) => note.id))

  let added = 0
  for (const note of parsed) {
    if (note.id !== '' && ids.has(note.id)) {
      skipped++
      continue
    }
    const id = note.id === '' ? makeId() : note.id
    merged.push({ ...note, id, updatedAt: note.updatedAt === '' ? now : note.updatedAt })
    ids.add(id)
    added++
  }

  return { added, skipped, notes: sortNotes(merged), error: '' }
}

export interface NotesExport {
  version: number
  kind: string
  exportedAt: string
  notes: Note[]
}

export function exportDoc(notes: Note[], now: string): NotesExport {
  return { version: NOTES_DOC_VERSION, kind: NOTES_EXPORT_KIND, exportedAt: now, notes }
}

export function exportFileName(now: string): string {
  return `chit-ticket-notes-${now.slice(0, 10)}.json`
}

export type ReadResult = { ok: true; doc: unknown } | { ok: false; error: string }

export function readImport(body: string): ReadResult {
  if (body.length > MAX_IMPORT_BYTES) return { ok: false, error: TOO_BIG_MESSAGE }
  let parsed: unknown
  try {
    parsed = JSON.parse(body)
  } catch {
    return { ok: false, error: NOT_A_NOTE_FILE_MESSAGE }
  }
  if (typeof parsed !== 'object' || parsed === null) {
    return { ok: false, error: NOT_A_NOTE_FILE_MESSAGE }
  }
  const doc = parsed as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > NOTES_DOC_VERSION) {
    return { ok: false, error: NEWER_FILE_MESSAGE }
  }
  if (!Array.isArray(doc.notes)) return { ok: false, error: NOT_A_NOTE_FILE_MESSAGE }
  return { ok: true, doc: parsed }
}

/** The label a note gets in the list on the left. */
export function noteLabel(note: Note): string {
  const title = note.title.trim()
  if (title !== '') return title
  const ref = note.ref.trim()
  if (ref !== '') return ref
  return 'Untitled note'
}
