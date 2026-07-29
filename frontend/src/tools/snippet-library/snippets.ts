// The shared snippet library: schema, validation, seed, merge and export.
// Everything here is pure so it can be tested without a DOM, with ids and the
// clock injected by the caller.

export const SNIPPET_NAMESPACE = 'snippet-library'
export const SNIPPET_DOC_VERSION = 1
export const SNIPPET_EXPORT_KIND = 'chit/snippet-library'

/** A snippet library is a few kilobytes. Anything much bigger is not one. */
export const MAX_IMPORT_BYTES = 5 * 1024 * 1024

export const MAX_TITLE = 80
export const MAX_BODY = 4000
export const MAX_GROUP = 40

export interface Snippet {
  id: string
  title: string
  group: string
  tags: string[]
  body: string
  addedAt: string
  updatedAt: string
}

export interface SnippetDoc {
  version: number
  snippets: Snippet[]
}

/** What the add / edit form holds. Every field is text, as typed. */
export interface SnippetDraft {
  title: string
  group: string
  tags: string
  body: string
}

export type SnippetErrors = Partial<Record<keyof SnippetDraft, string>>

export const EMPTY_DRAFT: SnippetDraft = { title: '', group: '', tags: '', body: '' }

export const TITLE_MESSAGE = 'Give the snippet a title so you can find it later.'
export const TITLE_LONG_MESSAGE = 'Keep the title to 80 characters or fewer.'
export const BODY_MESSAGE = 'A snippet with nothing in it has nothing to copy. Type the command or the text.'
export const BODY_LONG_MESSAGE =
  'Keep a snippet to 4000 characters or fewer. Longer than that belongs in a document, not on the clipboard.'
export const GROUP_LONG_MESSAGE = 'Keep the group name to 40 characters or fewer.'

export const NOT_A_LIBRARY_MESSAGE =
  'That file is not a CHIT snippet library. Export one from this tool to see the format.'
export const NEWER_FILE_MESSAGE =
  'That file was written by a newer version of CHIT and could not be read. Update CHIT, or export it again from the machine that wrote it.'
export const NEWER_VERSION_MESSAGE =
  'This snippet library was written by a newer version of CHIT and could not be read. Update CHIT, or export the library again from the machine that wrote it.'
export const TOO_BIG_MESSAGE =
  'That file is larger than 5 MB. A snippet library is normally a few kilobytes, so this is probably not one.'
export const NO_STORE_MESSAGE =
  'Saving needs the desktop app. Anything you add here will be gone when you close the page.'
export const SAVE_FAILED_MESSAGE =
  'The snippet library could not be saved. Check that the CHIT data folder is still there.'

// The same collator ResultsTable sorts with, so "Server 2" comes before "Server 10".
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

export function newSnippetId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(4))
  return 'snip-' + Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

/** Turns the comma-separated tag field into a clean, de-duplicated list. */
export function normalizeTags(text: string): string[] {
  const seen = new Set<string>()
  for (const part of text.split(',')) {
    const tag = part.trim().toLowerCase()
    if (tag !== '') seen.add(tag)
  }
  return Array.from(seen)
}

export function validateSnippet(
  draft: SnippetDraft,
): { ok: true; fields: Omit<Snippet, 'id' | 'addedAt' | 'updatedAt'> } | { ok: false; errors: SnippetErrors } {
  const errors: SnippetErrors = {}

  const title = draft.title.trim()
  if (title === '') errors.title = TITLE_MESSAGE
  else if (title.length > MAX_TITLE) errors.title = TITLE_LONG_MESSAGE

  const group = draft.group.trim()
  if (group.length > MAX_GROUP) errors.group = GROUP_LONG_MESSAGE

  const body = draft.body.trim()
  if (body === '') errors.body = BODY_MESSAGE
  else if (body.length > MAX_BODY) errors.body = BODY_LONG_MESSAGE

  if (Object.keys(errors).length > 0) return { ok: false, errors }
  return { ok: true, fields: { title, group, tags: normalizeTags(draft.tags), body } }
}

export function draftOf(snippet: Snippet): SnippetDraft {
  return {
    title: snippet.title,
    group: snippet.group,
    tags: snippet.tags.join(', '),
    body: snippet.body,
  }
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function readSnippet(raw: unknown): Snippet | null {
  if (typeof raw !== 'object' || raw === null) return null
  const entry = raw as Record<string, unknown>
  const title = text(entry.title)
  const body = typeof entry.body === 'string' ? entry.body.trim() : ''
  if (title === '' || body === '') return null
  return {
    id: text(entry.id),
    title,
    group: text(entry.group),
    tags: Array.isArray(entry.tags) ? normalizeTags(entry.tags.map(text).join(',')) : [],
    body,
    addedAt: text(entry.addedAt),
    updatedAt: text(entry.updatedAt),
  }
}

function readList(raw: unknown): Snippet[] | null {
  if (typeof raw !== 'object' || raw === null) return null
  const doc = raw as Record<string, unknown>
  if (!Array.isArray(doc.snippets)) return null
  const snippets: Snippet[] = []
  for (const entry of doc.snippets) {
    const snippet = readSnippet(entry)
    if (snippet !== null) snippets.push(snippet)
  }
  return snippets
}

/** Accepts anything read from disk. A shape that cannot be read becomes an
 *  empty library rather than an error, so a corrupt file never blocks the tool. */
export function migrateDoc(raw: unknown): SnippetDoc {
  const empty: SnippetDoc = { version: SNIPPET_DOC_VERSION, snippets: [] }
  if (typeof raw !== 'object' || raw === null) return empty
  const doc = raw as Record<string, unknown>
  if (typeof doc.version !== 'number' || doc.version > SNIPPET_DOC_VERSION) return empty
  return { version: SNIPPET_DOC_VERSION, snippets: readList(doc) ?? [] }
}

export function docWarning(raw: unknown): string {
  if (typeof raw !== 'object' || raw === null) return ''
  const doc = raw as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > SNIPPET_DOC_VERSION) {
    return NEWER_VERSION_MESSAGE
  }
  return ''
}

export function ensureIds(snippets: Snippet[], makeId: () => string): Snippet[] {
  return snippets.map((snippet) => (snippet.id === '' ? { ...snippet, id: makeId() } : snippet))
}

/**
 * The set the library starts with, so the tool is not an empty box on first
 * run. Every one is a real command a tech uses, and every one is an ordinary
 * snippet the tech can edit or delete. Seeded only when there is no saved
 * document at all: a library somebody emptied on purpose stays empty.
 */
export function starterSnippets(now: string): Snippet[] {
  const seeds: Array<Omit<Snippet, 'id' | 'addedAt' | 'updatedAt'>> = [
    {
      title: 'Flush the DNS cache',
      group: 'Windows',
      tags: ['dns', 'cache', 'name resolution'],
      body: 'ipconfig /flushdns',
    },
    {
      title: 'Renew the DHCP lease',
      group: 'Windows',
      tags: ['dhcp', 'ip', 'network'],
      body: 'ipconfig /release\nipconfig /renew',
    },
    {
      title: 'Force a Group Policy update',
      group: 'Windows',
      tags: ['gpo', 'policy', 'domain'],
      body: 'gpupdate /force',
    },
    {
      title: 'Check Windows system files',
      group: 'Windows',
      tags: ['sfc', 'corruption', 'repair'],
      body: 'sfc /scannow',
    },
    {
      title: 'Repair the Windows image',
      group: 'Windows',
      tags: ['dism', 'corruption', 'repair'],
      body: 'DISM /Online /Cleanup-Image /RestoreHealth',
    },
    {
      title: 'Show the printer server properties',
      group: 'Windows',
      tags: ['printer', 'driver', 'spooler'],
      body: 'printui.exe /s /t2',
    },
    {
      title: 'Restart the print spooler',
      group: 'Windows',
      tags: ['printer', 'spooler', 'queue'],
      body: 'net stop spooler\nnet start spooler',
    },
    {
      title: 'Startup programs in the registry',
      group: 'Windows',
      tags: ['startup', 'registry', 'slow pc'],
      body:
        'HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run\n' +
        'HKEY_LOCAL_MACHINE\\Software\\Microsoft\\Windows\\CurrentVersion\\Run',
    },
    {
      title: 'Show the ARP table',
      group: 'Networking',
      tags: ['arp', 'mac', 'switch'],
      body: 'arp -a',
    },
    {
      title: 'Ask a user to restart properly',
      group: 'Helpdesk',
      tags: ['canned reply', 'restart', 'user'],
      body:
        'Please choose Restart rather than Shut Down. On Windows, Shut Down does not fully close ' +
        'everything, so a Restart is what clears the problem.',
    },
  ]
  return seeds.map((seed, at) => ({
    ...seed,
    id: `snip-starter-${at + 1}`,
    addedAt: now,
    updatedAt: now,
  }))
}

export function snippetKey(snippet: Pick<Snippet, 'group' | 'title'>): string {
  return `${snippet.group.trim().toLowerCase()}|${snippet.title.trim().toLowerCase()}`
}

export function sortSnippets(snippets: Snippet[]): Snippet[] {
  return snippets
    .slice()
    .sort((a, b) => collator.compare(a.group, b.group) || collator.compare(a.title, b.title))
}

export interface MergeReport {
  added: number
  updated: number
  unchanged: number
  skipped: number
  snippets: Snippet[]
  error: string
}

/**
 * Merges an imported library into the current one.
 *
 * A colleague's version of a command never silently replaces the one you fixed:
 * when the titles match but the bodies differ, both are kept and the incoming
 * one is marked "(imported)" so a human decides which to delete.
 *
 * Pure: ids and the clock come from the caller.
 */
export function mergeSnippets(
  current: Snippet[],
  incoming: unknown,
  makeId: () => string,
  now: string,
): MergeReport {
  const parsed = readList(incoming)
  if (parsed === null) {
    return { added: 0, updated: 0, unchanged: 0, skipped: 0, snippets: current, error: NOT_A_LIBRARY_MESSAGE }
  }

  const raw = (incoming as Record<string, unknown>).snippets
  const skippedByReader = Array.isArray(raw) ? raw.length - parsed.length : 0

  const merged = current.map((snippet) => ({ ...snippet, tags: snippet.tags.slice() }))
  const byKey = new Map(merged.map((snippet) => [snippetKey(snippet), snippet]))

  let added = 0
  let updated = 0
  let unchanged = 0

  for (const entry of parsed) {
    const existing = byKey.get(snippetKey(entry))
    if (existing === undefined) {
      const snippet: Snippet = { ...entry, id: makeId(), addedAt: now, updatedAt: now }
      merged.push(snippet)
      byKey.set(snippetKey(snippet), snippet)
      added++
      continue
    }

    if (existing.body === entry.body) {
      const before = existing.tags.length
      for (const tag of entry.tags) {
        if (!existing.tags.includes(tag)) existing.tags.push(tag)
      }
      if (existing.tags.length > before) {
        existing.updatedAt = now
        updated++
      } else {
        unchanged++
      }
      continue
    }

    const snippet: Snippet = {
      ...entry,
      title: `${entry.title} (imported)`,
      id: makeId(),
      addedAt: now,
      updatedAt: now,
    }
    merged.push(snippet)
    byKey.set(snippetKey(snippet), snippet)
    added++
  }

  return {
    added,
    updated,
    unchanged,
    skipped: skippedByReader,
    snippets: sortSnippets(merged),
    error: '',
  }
}

/** The snippets matching the search box and the group picker. */
export function filterSnippets(snippets: Snippet[], query: string, group: string): Snippet[] {
  const needle = query.trim().toLowerCase()
  return snippets.filter((snippet) => {
    if (group !== '' && snippet.group !== group) return false
    if (needle === '') return true
    return [snippet.title, snippet.body, snippet.group, snippet.tags.join(' ')]
      .join(' ')
      .toLowerCase()
      .includes(needle)
  })
}

export function groupNames(snippets: Snippet[]): string[] {
  const groups = new Set<string>()
  for (const snippet of snippets) {
    if (snippet.group !== '') groups.add(snippet.group)
  }
  return Array.from(groups).sort(collator.compare)
}

export interface SnippetExport {
  version: number
  kind: string
  exportedAt: string
  snippets: Snippet[]
}

export function exportDoc(snippets: Snippet[], now: string): SnippetExport {
  return { version: SNIPPET_DOC_VERSION, kind: SNIPPET_EXPORT_KIND, exportedAt: now, snippets }
}

export function exportFileName(group: string, now: string): string {
  const name = group
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
  return `chit-snippets-${name === '' ? 'all' : name}-${now.slice(0, 10)}.json`
}

export type ReadResult = { ok: true; doc: unknown } | { ok: false; error: string }

/** Checks a picked file before it goes anywhere near the merge. */
export function readImport(body: string): ReadResult {
  if (body.length > MAX_IMPORT_BYTES) return { ok: false, error: TOO_BIG_MESSAGE }
  let parsed: unknown
  try {
    parsed = JSON.parse(body)
  } catch {
    return { ok: false, error: NOT_A_LIBRARY_MESSAGE }
  }
  if (typeof parsed !== 'object' || parsed === null) {
    return { ok: false, error: NOT_A_LIBRARY_MESSAGE }
  }
  const doc = parsed as Record<string, unknown>
  if (typeof doc.version === 'number' && doc.version > SNIPPET_DOC_VERSION) {
    return { ok: false, error: NEWER_FILE_MESSAGE }
  }
  if (!Array.isArray(doc.snippets)) return { ok: false, error: NOT_A_LIBRARY_MESSAGE }
  return { ok: true, doc: parsed }
}
