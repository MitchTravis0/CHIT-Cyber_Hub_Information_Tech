// Checklists (the procedures) and runs (one time you carried one out): schema,
// the starter set, validation, the report and the import merge. All pure, so it
// can be tested without a DOM, with ids and stamps injected by the caller.

export const CHECKLIST_NAMESPACE = 'site-checklist'
export const CHECKLIST_DOC_VERSION = 1
export const CHECKLIST_EXPORT_KIND = 'chit/site-checklists'
export const RUNS_EXPORT_KIND = 'chit/site-checklist-runs'

export const MAX_IMPORT_BYTES = 5 * 1024 * 1024

export const MAX_LIST_NAME = 80
export const MAX_ITEM_TEXT = 200
export const MAX_ITEMS = 200
export const MAX_NOTE = 500
export const MAX_SITE = 80

export type ItemState = 'todo' | 'done' | 'skipped' | 'na'

const STATES: ItemState[] = ['todo', 'done', 'skipped', 'na']

export interface ChecklistItem {
  id: string
  text: string
}

export interface Checklist {
  id: string
  name: string
  description: string
  items: ChecklistItem[]
  addedAt: string
  updatedAt: string
}

export interface RunItem {
  id: string
  text: string
  state: ItemState
  note: string
}

export interface Run {
  id: string
  checklistId: string
  checklistName: string
  site: string
  startedStamp: string
  finishedStamp: string
  items: RunItem[]
  updatedAt: string
}

export interface ChecklistDoc {
  version: number
  checklists: Checklist[]
  runs: Run[]
}

export const NAME_MESSAGE = 'Give the checklist a name so you can pick it later.'
export const NAME_LONG_MESSAGE = 'Keep the checklist name to 80 characters or fewer.'
export const NO_ITEMS_MESSAGE = 'A checklist with no items has nothing to tick. Press Add item.'
export const ITEM_LONG_MESSAGE = 'Keep an item to 200 characters or fewer. Split it into two.'
export const TOO_MANY_ITEMS_MESSAGE =
  'A checklist can hold 200 items. That is already more than anyone works through in one visit.'
export const NOTE_LONG_MESSAGE = 'Keep a note to 500 characters or fewer.'
export const SITE_LONG_MESSAGE = 'Keep the site to 80 characters or fewer.'

export const NOT_A_CHECKLIST_FILE_MESSAGE =
  'That file is not a CHIT checklist file. Export one from this tool to see the format.'
export const NEWER_FILE_MESSAGE =
  'That file was written by a newer version of CHIT and could not be read. Update CHIT, or export it again from the machine that wrote it.'
export const NEWER_VERSION_MESSAGE =
  'These checklists were written by a newer version of CHIT and could not be read. Update CHIT, or export them again from the machine that wrote them.'
export const TOO_BIG_MESSAGE =
  'That file is larger than 5 MB. A checklist file is normally a few kilobytes, so this is probably not one.'
export const NO_STORE_MESSAGE =
  'Saving needs the desktop app. Anything you do here will be gone when you close the page.'
export const SAVE_FAILED_MESSAGE =
  'The checklists could not be saved. Check that the CHIT data folder is still there.'

function randomHex(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(4))
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export function newChecklistId(): string {
  return 'list-' + randomHex()
}

export function newItemId(): string {
  return 'item-' + randomHex()
}

export function newRunId(): string {
  return 'run-' + randomHex()
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

/** The same local YYYY-MM-DD HH:MM text the Ticket Note Formatter writes, for
 *  the same reason: a report shows the time on the wall where the work happened. */
export function stampOf(date: Date): string {
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    ` ${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

export type ChecklistErrors = { name?: string; items?: string; itemAt?: Record<number, string> }

export function validateChecklist(
  name: string,
  items: string[],
):
  | { ok: true; name: string; items: string[] }
  | { ok: false; errors: ChecklistErrors } {
  const errors: ChecklistErrors = {}

  const trimmedName = name.trim()
  if (trimmedName === '') errors.name = NAME_MESSAGE
  else if (trimmedName.length > MAX_LIST_NAME) errors.name = NAME_LONG_MESSAGE

  // A stray Add item does no harm: an empty row is simply dropped.
  const kept = items.map((text) => text.trim()).filter((text) => text !== '')

  const itemAt: Record<number, string> = {}
  items.forEach((text, at) => {
    if (text.trim().length > MAX_ITEM_TEXT) itemAt[at] = ITEM_LONG_MESSAGE
  })
  if (Object.keys(itemAt).length > 0) errors.itemAt = itemAt

  if (kept.length === 0) errors.items = NO_ITEMS_MESSAGE
  else if (kept.length > MAX_ITEMS) errors.items = TOO_MANY_ITEMS_MESSAGE

  if (Object.keys(errors).length > 0) return { ok: false, errors }
  return { ok: true, name: trimmedName, items: kept }
}

export function validateNote(note: string): string | undefined {
  return note.length > MAX_NOTE ? NOTE_LONG_MESSAGE : undefined
}

export function validateSite(site: string): string | undefined {
  return site.length > MAX_SITE ? SITE_LONG_MESSAGE : undefined
}

/**
 * Starts a run from a checklist.
 *
 * The item text is copied into the run rather than referenced, so improving the
 * checklist afterwards never rewrites a job that has already happened.
 */
export function startRun(
  checklist: Checklist,
  site: string,
  makeId: () => string,
  stamp: string,
  now: string,
): Run {
  return {
    id: makeId(),
    checklistId: checklist.id,
    checklistName: checklist.name,
    site: site.trim(),
    startedStamp: stamp,
    finishedStamp: '',
    items: checklist.items.map((item) => ({
      id: item.id,
      text: item.text,
      state: 'todo',
      note: '',
    })),
    updatedAt: now,
  }
}

export interface Tally {
  done: number
  skipped: number
  na: number
  todo: number
  dealtWith: number
  total: number
}

export function runTally(run: Run): Tally {
  const tally: Tally = { done: 0, skipped: 0, na: 0, todo: 0, dealtWith: 0, total: run.items.length }
  for (const item of run.items) {
    switch (item.state) {
      case 'done':
        tally.done++
        break
      case 'skipped':
        tally.skipped++
        break
      case 'na':
        tally.na++
        break
      default:
        tally.todo++
    }
  }
  tally.dealtWith = tally.done + tally.skipped + tally.na
  return tally
}

const MARKERS: Record<ItemState, string> = {
  done: '[x]',
  skipped: '[-]',
  na: '[n/a]',
  todo: '[ ]',
}

/** The "Result:" line. Only the non-zero counts appear, except done, which is
 *  always there so a report never reads as if nothing happened. */
export function resultLine(run: Run): string {
  const tally = runTally(run)
  const parts = [`${tally.done} done`]
  if (tally.skipped > 0) parts.push(`${tally.skipped} skipped`)
  if (tally.na > 0) parts.push(`${tally.na} not applicable`)
  if (tally.todo > 0) parts.push(`${tally.todo} still to do`)
  const line = `Result: ${parts.join(', ')}`
  return run.finishedStamp === '' ? `${line} (still open)` : line
}

/** The record a tech pastes into the ticket as proof of what was done. */
export function formatRun(run: Run): string {
  const header: string[] = [run.checklistName]
  if (run.site !== '') header.push(`Site: ${run.site}`)
  header.push(`Started: ${run.startedStamp}`)
  if (run.finishedStamp !== '') header.push(`Finished: ${run.finishedStamp}`)
  header.push(resultLine(run))

  const lines: string[] = []
  for (const item of run.items) {
    lines.push(`${MARKERS[item.state]} ${item.text}`)
    const note = item.note.trim()
    if (note !== '') {
      for (const noteLine of note.split('\n')) lines.push(`    ${noteLine}`)
    }
  }

  return lines.length === 0 ? header.join('\n') : `${header.join('\n')}\n\n${lines.join('\n')}`
}

function slug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
}

/** chit-run-new-pc-setup-head-office-2026-07-26.txt */
export function reportFileName(run: Run, now: string): string {
  const list = slug(run.checklistName)
  const site = slug(run.site)
  return `chit-run-${list === '' ? 'checklist' : list}-${site === '' ? 'no-site' : site}-${now.slice(0, 10)}.txt`
}

/** Moves one item by a single position. At either end nothing changes. */
export function moveItem<T>(items: T[], from: number, by: -1 | 1): T[] {
  const to = from + by
  if (from < 0 || from >= items.length || to < 0 || to >= items.length) return items
  const next = items.slice()
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}

/**
 * The set the tool starts with, so Start run works on the very first visit.
 * These are ordinary checklists a tech can edit or delete. Seeded only when
 * there is no saved document at all.
 */
export function starterChecklists(now: string): Checklist[] {
  const seeds: Array<{ id: string; name: string; description: string; items: string[] }> = [
    {
      id: 'list-starter-new-pc',
      name: 'New PC setup',
      description: 'Everything a new machine needs before it reaches a user.',
      items: [
        'Windows updates installed and rebooted',
        'Machine renamed and joined to the domain',
        'BitLocker enabled and the recovery key escrowed',
        'Antivirus installed and reporting in',
        'Power plan set so the machine does not sleep on mains',
        'Standard software installed',
        'Printers mapped and a test page printed',
        'User profile signed in once and the mailbox loaded',
        'Asset tag applied and recorded in the inventory',
        "Old machine's data copied and confirmed by the user",
      ],
    },
    {
      id: 'list-starter-office-move',
      name: 'Office move',
      description: 'Taking a site down and standing it back up.',
      items: [
        'Photograph the comms cabinet before anything is unplugged',
        'Record every switch port and what is patched into it',
        "Note the WAN circuit reference and the router's static addresses",
        'Label every cable at both ends',
        'Back up the switch and firewall configuration',
        "Confirm the new site's internet line is live and tested",
        'Rack and power the equipment at the new site',
        'Bring the firewall and switches up and check the internet works',
        'Bring the server or NAS up and check shares are reachable',
        'Test a printer, a phone and a wireless client',
        'Update the inventory with the new addresses',
      ],
    },
    {
      id: 'list-starter-decommission',
      name: 'Decommission a PC',
      description: 'Retiring a machine without leaving anything behind.',
      items: [
        'Confirm with the user that nothing is left on the machine',
        'Back up anything still needed and confirm the backup opens',
        'Sign the user out and remove cached credentials',
        'Remove the machine from the domain and delete the AD object',
        'Remove it from antivirus, MDM and any monitoring',
        'Retrieve the BitLocker key and record where the machine went',
        'Wipe the drive or destroy it, and record which',
        'Remove it from the inventory and note the date',
        'Return or dispose of it and keep the disposal receipt',
      ],
    },
  ]

  return seeds.map((seed) => ({
    id: seed.id,
    name: seed.name,
    description: seed.description,
    items: seed.items.map((text, at) => ({ id: `${seed.id}-item-${at + 1}`, text })),
    addedAt: now,
    updatedAt: now,
  }))
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function readItem(raw: unknown): ChecklistItem | null {
  if (typeof raw !== 'object' || raw === null) return null
  const item = raw as Record<string, unknown>
  const body = text(item.text).trim()
  if (body === '') return null
  return { id: text(item.id).trim(), text: body }
}

function readChecklist(raw: unknown): Checklist | null {
  if (typeof raw !== 'object' || raw === null) return null
  const list = raw as Record<string, unknown>
  const name = text(list.name).trim()
  if (name === '') return null
  const items: ChecklistItem[] = []
  if (Array.isArray(list.items)) {
    for (const entry of list.items) {
      const item = readItem(entry)
      if (item !== null) items.push(item)
    }
  }
  if (items.length === 0) return null
  return {
    id: text(list.id).trim(),
    name,
    description: text(list.description).trim(),
    items,
    addedAt: text(list.addedAt).trim(),
    updatedAt: text(list.updatedAt).trim(),
  }
}

function readState(value: unknown): ItemState {
  return typeof value === 'string' && (STATES as string[]).includes(value)
    ? (value as ItemState)
    : 'todo'
}

function readRun(raw: unknown): Run | null {
  if (typeof raw !== 'object' || raw === null) return null
  const run = raw as Record<string, unknown>
  const items: RunItem[] = []
  if (Array.isArray(run.items)) {
    for (const entry of run.items) {
      if (typeof entry !== 'object' || entry === null) continue
      const item = entry as Record<string, unknown>
      const body = text(item.text).trim()
      if (body === '') continue
      items.push({
        id: text(item.id).trim(),
        text: body,
        state: readState(item.state),
        note: text(item.note),
      })
    }
  }
  if (items.length === 0) return null
  return {
    id: text(run.id).trim(),
    checklistId: text(run.checklistId).trim(),
    checklistName: text(run.checklistName).trim(),
    site: text(run.site).trim(),
    startedStamp: text(run.startedStamp).trim(),
    finishedStamp: text(run.finishedStamp).trim(),
    items,
    updatedAt: text(run.updatedAt).trim(),
  }
}

export function migrateDoc(raw: unknown): ChecklistDoc {
  const empty: ChecklistDoc = { version: CHECKLIST_DOC_VERSION, checklists: [], runs: [] }
  if (typeof raw !== 'object' || raw === null) return empty
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > CHECKLIST_DOC_VERSION) return empty

  const checklists: Checklist[] = []
  if (Array.isArray(doc.checklists)) {
    for (const entry of doc.checklists) {
      const list = readChecklist(entry)
      if (list !== null) checklists.push(list)
    }
  }
  const runs: Run[] = []
  if (Array.isArray(doc.runs)) {
    for (const entry of doc.runs) {
      const run = readRun(entry)
      if (run !== null) runs.push(run)
    }
  }
  return { version: CHECKLIST_DOC_VERSION, checklists, runs }
}

export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > CHECKLIST_DOC_VERSION) {
    return NEWER_VERSION_MESSAGE
  }
  return ''
}

export function ensureIds(
  checklists: Checklist[],
  runs: Run[],
  makeListId: () => string,
  makeItemId: () => string,
  makeRunId: () => string,
): { checklists: Checklist[]; runs: Run[] } {
  return {
    checklists: checklists.map((list) => ({
      ...list,
      id: list.id === '' ? makeListId() : list.id,
      items: list.items.map((item) => (item.id === '' ? { ...item, id: makeItemId() } : item)),
    })),
    runs: runs.map((run) => ({
      ...run,
      id: run.id === '' ? makeRunId() : run.id,
      items: run.items.map((item) => (item.id === '' ? { ...item, id: makeItemId() } : item)),
    })),
  }
}

export function sortRuns(runs: Run[]): Run[] {
  return runs.slice().sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
}

export function checklistKey(name: string): string {
  return name.trim().toLowerCase()
}

function sameItems(a: Checklist, b: Checklist): boolean {
  if (a.items.length !== b.items.length) return false
  return a.items.every((item, at) => item.text === b.items[at].text)
}

export interface MergeReport {
  added: number
  unchanged: number
  skipped: number
  checklists: Checklist[]
  error: string
}

/**
 * Merges imported checklists into the saved ones.
 *
 * A colleague's procedure never silently replaces yours: same name but
 * different items keeps both, and the incoming one is marked "(imported)" so a
 * human compares them and deletes one.
 */
export function mergeChecklists(
  current: Checklist[],
  incoming: unknown,
  makeListId: () => string,
  makeItemId: () => string,
  now: string,
): MergeReport {
  if (typeof incoming !== 'object' || incoming === null) {
    return { added: 0, unchanged: 0, skipped: 0, checklists: current, error: NOT_A_CHECKLIST_FILE_MESSAGE }
  }
  const doc = incoming as Record<string, unknown>
  if (!Array.isArray(doc.checklists)) {
    return { added: 0, unchanged: 0, skipped: 0, checklists: current, error: NOT_A_CHECKLIST_FILE_MESSAGE }
  }

  const parsed: Checklist[] = []
  for (const entry of doc.checklists) {
    const list = readChecklist(entry)
    if (list !== null) parsed.push(list)
  }

  const merged = current.map((list) => ({ ...list, items: list.items.slice() }))
  const byKey = new Map(merged.map((list) => [checklistKey(list.name), list]))

  let added = 0
  let unchanged = 0
  const skipped = doc.checklists.length - parsed.length

  for (const entry of parsed) {
    const withIds: Checklist = {
      ...entry,
      id: makeListId(),
      items: entry.items.map((item) => ({ id: makeItemId(), text: item.text })),
      addedAt: now,
      updatedAt: now,
    }
    const existing = byKey.get(checklistKey(entry.name))
    if (existing === undefined) {
      merged.push(withIds)
      byKey.set(checklistKey(withIds.name), withIds)
      added++
      continue
    }
    if (sameItems(existing, entry)) {
      unchanged++
      continue
    }
    const renamed: Checklist = { ...withIds, name: `${entry.name} (imported)` }
    merged.push(renamed)
    byKey.set(checklistKey(renamed.name), renamed)
    added++
  }

  return { added, unchanged, skipped, checklists: merged, error: '' }
}

export interface ChecklistExport {
  version: number
  kind: string
  exportedAt: string
  checklists: Checklist[]
}

export interface RunsExport {
  version: number
  kind: string
  exportedAt: string
  runs: Run[]
}

export function exportChecklistsDoc(checklists: Checklist[], now: string): ChecklistExport {
  return { version: CHECKLIST_DOC_VERSION, kind: CHECKLIST_EXPORT_KIND, exportedAt: now, checklists }
}

export function exportRunsDoc(runs: Run[], now: string): RunsExport {
  return { version: CHECKLIST_DOC_VERSION, kind: RUNS_EXPORT_KIND, exportedAt: now, runs }
}

export function checklistsFileName(now: string): string {
  return `chit-checklists-${now.slice(0, 10)}.json`
}

export function runsFileName(now: string): string {
  return `chit-runs-${now.slice(0, 10)}.json`
}

export type ReadResult = { ok: true; doc: unknown } | { ok: false; error: string }

export function readImport(body: string): ReadResult {
  if (body.length > MAX_IMPORT_BYTES) return { ok: false, error: TOO_BIG_MESSAGE }
  let parsed: unknown
  try {
    parsed = JSON.parse(body)
  } catch {
    return { ok: false, error: NOT_A_CHECKLIST_FILE_MESSAGE }
  }
  if (typeof parsed !== 'object' || parsed === null) {
    return { ok: false, error: NOT_A_CHECKLIST_FILE_MESSAGE }
  }
  const doc = parsed as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > CHECKLIST_DOC_VERSION) {
    return { ok: false, error: NEWER_FILE_MESSAGE }
  }
  if (!Array.isArray(doc.checklists)) return { ok: false, error: NOT_A_CHECKLIST_FILE_MESSAGE }
  return { ok: true, doc: parsed }
}

/** The label a run gets in the picker. */
export function runLabel(run: Run): string {
  const site = run.site === '' ? 'No site' : run.site
  return `${run.checklistName} - ${site} - ${run.startedStamp}`
}
