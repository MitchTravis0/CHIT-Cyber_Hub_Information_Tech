import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FileDown, FileUp, Pencil, Plus, Trash2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  Select,
  Textarea,
  TextInput,
  ToolShell,
  focusRing,
  useToast,
} from '../../components'
import { downloadJson } from '../../lib/download'
import { cn, formatDuration } from '../../lib/format'
import { getAppInfo, readDoc, writeDoc } from '../../shell/bindings'
import {
  docWarning,
  elapsed,
  emptyNote,
  ensureIds,
  exportDoc,
  exportFileName,
  filterNotes,
  formatNote,
  mergeNotes,
  migrateDoc,
  newEntryId,
  newNoteId,
  noteLabel,
  readImport,
  sortNotes,
  stampOf,
  validateEntry,
  validateNote,
  NOTES_DOC_VERSION,
  NOTES_NAMESPACE,
  NO_STORE_MESSAGE,
  SAVE_FAILED_MESSAGE,
  type Entry,
  type Note,
  type NoteErrors,
  type NotesDoc,
  type Style,
} from './notes'

const STYLES = [
  { value: 'text', label: 'Plain text' },
  { value: 'markdown', label: 'Markdown' },
]

const HELP = (
  <>
    <p>
      Keep this open while you work. Every time you try something, type one line and press Add step.
      It gets the date and time, so the write-up at the end is accurate rather than reconstructed.
    </p>
    <p className="mt-1.5">
      Issue is what the user reported. Steps taken is what you did, in order. Resolution is what
      actually fixed it, which is the part the next person reads. Leave any of the three empty and it
      is left out of the write-up entirely.
    </p>
    <p className="mt-1.5">
      Copy write-up puts the whole thing on the clipboard, ready to paste into any ticket system.
      Pick Markdown instead of Plain text when your system renders it, for example Jira, GitHub or
      Teams.
    </p>
    <p className="mt-1.5">
      Notes are saved on this machine, not sent anywhere. This tool has no connection to any ticket
      system: nothing here is filed for you.
    </p>
  </>
)

export default function TicketNotesPage() {
  const toast = useToast()

  const [notes, setNotes] = useState<Note[]>([])
  const [openId, setOpenId] = useState<string | null>(null)
  const [storeReady, setStoreReady] = useState(true)
  const [docNote, setDocNote] = useState('')
  const [loaded, setLoaded] = useState(false)

  const [query, setQuery] = useState('')
  const [style, setStyle] = useState<Style>('text')
  const [entryText, setEntryText] = useState('')
  const [entryError, setEntryError] = useState<string>()
  const [editingEntry, setEditingEntry] = useState<Entry | null>(null)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const entryRef = useRef<HTMLInputElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    void getAppInfo().then((info) => setStoreReady(info !== null))
    readDoc<unknown>(NOTES_NAMESPACE)
      .then((raw) => {
        if (raw !== null) {
          setDocNote(docWarning(raw))
          const list = sortNotes(ensureIds(migrateDoc(raw).notes, newNoteId, newEntryId))
          setNotes(list)
          setOpenId(list[0]?.id ?? null)
        }
        setLoaded(true)
      })
      .catch(() => setLoaded(true))
  }, [])

  // A page that opens with nothing to type into is a page a tech closes again,
  // so an empty list gets a note in memory. It is only saved once it has
  // something in it.
  useEffect(() => {
    if (!loaded || notes.length > 0 || openId !== null) return
    setNotes([emptyNote(newNoteId(), new Date().toISOString())])
  }, [loaded, notes.length, openId])

  useEffect(() => {
    if (openId === null && notes.length > 0) setOpenId(notes[0].id)
  }, [openId, notes])

  const persist = useCallback(
    (next: Note[]) => {
      setNotes(next)
      writeDoc(NOTES_NAMESPACE, {
        version: NOTES_DOC_VERSION,
        notes: next,
      } satisfies NotesDoc).catch(() => toast.push('error', SAVE_FAILED_MESSAGE))
    },
    [toast],
  )

  const open = notes.find((note) => note.id === openId) ?? null

  const update = useCallback(
    (change: Partial<Note>) => {
      if (open === null) return
      const at = new Date().toISOString()
      persist(notes.map((note) => (note.id === open.id ? { ...note, ...change, updatedAt: at } : note)))
    },
    [notes, open, persist],
  )

  const shownNotes = useMemo(() => sortNotes(filterNotes(notes, query)), [notes, query])
  const errors: NoteErrors = open === null ? {} : validateNote(open)
  const written = open === null ? '' : formatNote(open, style)
  const span = open === null ? null : elapsed(open.entries)

  const addEntry = () => {
    const problem = validateEntry(entryText)
    if (problem !== undefined || open === null) {
      setEntryError(problem)
      return
    }
    setEntryError(undefined)
    update({
      entries: [
        ...open.entries,
        { id: newEntryId(), stamp: stampOf(new Date()), text: entryText.trim() },
      ],
    })
    setEntryText('')
    entryRef.current?.focus()
  }

  const saveEntry = () => {
    if (open === null || editingEntry === null) return
    const problem = validateEntry(editingEntry.text)
    if (problem !== undefined) {
      setEntryError(problem)
      return
    }
    setEntryError(undefined)
    update({
      entries: open.entries.map((entry) =>
        entry.id === editingEntry.id ? { ...editingEntry, text: editingEntry.text.trim() } : entry,
      ),
    })
    setEditingEntry(null)
  }

  const removeEntry = (id: string) => {
    if (open === null) return
    update({ entries: open.entries.filter((entry) => entry.id !== id) })
  }

  const newNote = () => {
    const note = emptyNote(newNoteId(), new Date().toISOString())
    persist([note, ...notes])
    setOpenId(note.id)
  }

  const removeNote = (id: string) => {
    const next = notes.filter((note) => note.id !== id)
    persist(next)
    setConfirmId(null)
    setOpenId(next[0]?.id ?? null)
  }

  const onFile = async (file: File) => {
    const result = readImport(await file.text())
    if (!result.ok) {
      toast.push('error', result.error)
      return
    }
    const report = mergeNotes(notes, result.doc, newNoteId, new Date().toISOString())
    if (report.error !== '') {
      toast.push('error', report.error)
      return
    }
    persist(report.notes)
    toast.push('success', `Imported: ${report.added} added, ${report.skipped} skipped.`)
  }

  const onExport = () => {
    const now = new Date().toISOString()
    downloadJson(exportFileName(now), exportDoc(shownNotes, now))
  }

  return (
    <ToolShell
      title="Ticket Note Formatter"
      description="Keep timestamped notes while you work, then copy them out in the shape every ticket wants."
      help={HELP}
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
        <section className="flex flex-col gap-3">
          {!storeReady && <p className="text-xs text-warn">{NO_STORE_MESSAGE}</p>}
          {docNote !== '' && (
            <p role="alert" className="text-xs text-danger">
              {docNote}
            </p>
          )}

          <div className="flex flex-wrap gap-2">
            <Button variant="primary" onClick={newNote} icon={<Plus size={14} aria-hidden />}>
              New note
            </Button>
            <Button onClick={() => fileRef.current?.click()} icon={<FileUp size={14} aria-hidden />}>
              Import
            </Button>
            <Button
              onClick={onExport}
              disabled={shownNotes.length === 0}
              icon={<FileDown size={14} aria-hidden />}
            >
              Export
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0]
                event.target.value = ''
                if (file !== undefined) void onFile(file)
              }}
            />
          </div>

          <TextInput
            label="Filter notes"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Filter by reference or title"
            spellCheck={false}
            autoComplete="off"
          />

          <div className="flex flex-col gap-2">
            {shownNotes.length === 0 ? (
              <p className="py-4 text-center text-xs text-fg-muted">
                No notes yet. Press New note when you pick up a job.
              </p>
            ) : (
              shownNotes.map((note) => (
                <div
                  key={note.id}
                  className={cn(
                    'rounded border px-2 py-1.5',
                    note.id === openId ? 'border-accent bg-surface-3' : 'border-border bg-surface-2',
                  )}
                >
                  {confirmId === note.id ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-xs text-fg">Delete "{noteLabel(note)}"?</span>
                      <Button size="sm" variant="danger" onClick={() => removeNote(note.id)}>
                        Delete
                      </Button>
                      <Button size="sm" onClick={() => setConfirmId(null)}>
                        Cancel
                      </Button>
                    </div>
                  ) : (
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => setOpenId(note.id)}
                        className={cn('min-w-0 flex-1 rounded text-left', focusRing)}
                      >
                        {note.ref !== '' && (
                          <span className="block truncate font-mono text-xs text-fg-muted">
                            {note.ref}
                          </span>
                        )}
                        <span className="block truncate text-sm text-fg">{noteLabel(note)}</span>
                        <span className="block text-xs text-fg-muted">
                          {note.entries.length} {note.entries.length === 1 ? 'entry' : 'entries'}
                        </span>
                      </button>
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Delete note ${noteLabel(note)}`}
                        onClick={() => setConfirmId(note.id)}
                        icon={<Trash2 size={14} aria-hidden />}
                      />
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </section>

        {open !== null && (
          <section className="flex flex-col gap-4">
            <div className="flex flex-wrap gap-3">
              <div className="min-w-40 flex-1">
                <TextInput
                  label="Ticket reference"
                  value={open.ref}
                  onChange={(event) => update({ ref: event.target.value })}
                  placeholder="INC0012345"
                  spellCheck={false}
                  autoComplete="off"
                  className="font-mono"
                  error={errors.ref}
                />
              </div>
              <div className="min-w-56 flex-1">
                <TextInput
                  label="Title"
                  value={open.title}
                  onChange={(event) => update({ title: event.target.value })}
                  placeholder="PC will not join the domain"
                  error={errors.title}
                />
              </div>
            </div>

            <Textarea
              id="ticket-issue"
              label="Issue"
              hint="What the user reported, in their words."
              error={errors.issue}
              rows={3}
              value={open.issue}
              onChange={(event) => update({ issue: event.target.value })}
              className="resize-y"
            />

            <div className="flex flex-col gap-2">
              <p className="text-sm font-semibold text-fg">Steps taken</p>
              {open.entries.map((entry) => (
                <div
                  key={entry.id}
                  className="rounded border border-border bg-surface-2 px-2 py-1.5"
                >
                  {editingEntry !== null && editingEntry.id === entry.id ? (
                    <form
                      className="flex flex-wrap items-end gap-2"
                      onSubmit={(event) => {
                        event.preventDefault()
                        saveEntry()
                      }}
                    >
                      <div className="w-40">
                        <TextInput
                          label="Time"
                          value={editingEntry.stamp}
                          onChange={(event) =>
                            setEditingEntry({ ...editingEntry, stamp: event.target.value })
                          }
                          className="font-mono text-xs"
                          spellCheck={false}
                        />
                      </div>
                      <div className="min-w-56 flex-1">
                        <TextInput
                          label="Step"
                          value={editingEntry.text}
                          onChange={(event) =>
                            setEditingEntry({ ...editingEntry, text: event.target.value })
                          }
                          error={entryError}
                        />
                      </div>
                      <Button type="submit" variant="primary" size="sm">
                        Save
                      </Button>
                      <Button
                        size="sm"
                        onClick={() => {
                          setEditingEntry(null)
                          setEntryError(undefined)
                        }}
                      >
                        Cancel
                      </Button>
                    </form>
                  ) : (
                    <div className="flex items-start gap-2">
                      <span className="shrink-0 font-mono text-xs text-fg-muted">{entry.stamp}</span>
                      <span className="min-w-0 flex-1 text-sm text-fg">{entry.text}</span>
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Edit step ${entry.stamp}`}
                        onClick={() => {
                          setEditingEntry(entry)
                          setEntryError(undefined)
                        }}
                        icon={<Pencil size={14} aria-hidden />}
                      />
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Delete step ${entry.stamp}`}
                        onClick={() => removeEntry(entry.id)}
                        icon={<Trash2 size={14} aria-hidden />}
                      />
                    </div>
                  )}
                </div>
              ))}

              <form
                className="flex flex-wrap items-end gap-2"
                onSubmit={(event) => {
                  event.preventDefault()
                  addEntry()
                }}
              >
                <div className="min-w-56 flex-1">
                  <TextInput
                    ref={entryRef}
                    label="What did you just do?"
                    value={entryText}
                    onChange={(event) => setEntryText(event.target.value)}
                    placeholder="Cleared the DNS cache and retried the join"
                    error={editingEntry === null ? entryError : undefined}
                  />
                </div>
                <Button type="submit" variant="primary" icon={<Plus size={14} aria-hidden />}>
                  Add step
                </Button>
              </form>
            </div>

            <Textarea
              id="ticket-resolution"
              label="Resolution"
              hint="What actually fixed it, so the next person does not repeat the six things that did not."
              error={errors.resolution}
              rows={3}
              value={open.resolution}
              onChange={(event) => update({ resolution: event.target.value })}
              className="resize-y"
            />

            <div className="flex flex-wrap items-end gap-2">
              <div className="min-w-40">
                <Select
                  label="Copy as"
                  options={STYLES}
                  value={style}
                  onChange={(event) => setStyle(event.target.value as Style)}
                />
              </div>
              <CopyButton value={written} label="Copy write-up" />
            </div>

            <pre className="overflow-x-auto rounded border border-border bg-surface-2 px-3 py-2 font-mono text-xs whitespace-pre-wrap text-fg">
              {written}
            </pre>

            {span !== null && (
              <p className="text-xs text-fg-muted">
                First step {open.entries[0]?.stamp}, last step{' '}
                {open.entries[open.entries.length - 1]?.stamp}, {formatDuration(span)} on this note.
              </p>
            )}
          </section>
        )}
      </div>
    </ToolShell>
  )
}
