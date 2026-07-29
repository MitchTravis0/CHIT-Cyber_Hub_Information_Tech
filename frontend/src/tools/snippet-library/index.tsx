import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FileDown, FileUp, Pencil, Plus, Trash2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  Select,
  Textarea,
  TextInput,
  ToolShell,
  useToast,
} from '../../components'
import { downloadJson } from '../../lib/download'
import { getAppInfo, readDoc, writeDoc } from '../../shell/bindings'
import {
  docWarning,
  draftOf,
  ensureIds,
  exportDoc,
  exportFileName,
  filterSnippets,
  groupNames,
  mergeSnippets,
  migrateDoc,
  newSnippetId,
  readImport,
  snippetKey,
  sortSnippets,
  starterSnippets,
  validateSnippet,
  EMPTY_DRAFT,
  NO_STORE_MESSAGE,
  SAVE_FAILED_MESSAGE,
  SNIPPET_DOC_VERSION,
  SNIPPET_NAMESPACE,
  type Snippet,
  type SnippetDoc,
  type SnippetDraft,
  type SnippetErrors,
} from './snippets'

const HELP = (
  <>
    <p>
      This is the team's shared cheat sheet. Anything you find yourself looking up twice belongs
      here: a command with awkward switches, a registry path, the wording of a canned reply to a
      user. Press Copy and it is on the clipboard, exactly as written.
    </p>
    <p className="mt-1.5">
      Search looks at the title, the snippet itself and the tags, so searching for "flush" finds the
      DNS command even if the title does not say it. The group picker is for narrowing to one area,
      for example Windows or Networking.
    </p>
    <p className="mt-1.5">
      Export writes one JSON file holding whatever the search is currently showing. That file is how
      the team stays in step: send it to a colleague, they press Import, and their library gains
      anything they were missing. An import never overwrites a snippet you have edited, so it is
      safe to run whenever somebody sends you a newer file.
    </p>
    <p className="mt-1.5">
      CHIT never runs a snippet. It only copies text, so a snippet that turns out to be wrong
      cannot do anything on its own.
    </p>
  </>
)

export default function SnippetLibraryPage() {
  const toast = useToast()

  const [snippets, setSnippets] = useState<Snippet[]>([])
  const [storeReady, setStoreReady] = useState(true)
  const [docNote, setDocNote] = useState('')

  const [query, setQuery] = useState('')
  const [group, setGroup] = useState('')
  const [draft, setDraft] = useState<SnippetDraft | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [errors, setErrors] = useState<SnippetErrors>({})
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const persist = useCallback(
    (next: Snippet[]) => {
      setSnippets(next)
      writeDoc(SNIPPET_NAMESPACE, {
        version: SNIPPET_DOC_VERSION,
        snippets: next,
      } satisfies SnippetDoc).catch(() => toast.push('error', SAVE_FAILED_MESSAGE))
    },
    [toast],
  )

  useEffect(() => {
    void getAppInfo().then((info) => setStoreReady(info !== null))
    readDoc<unknown>(SNIPPET_NAMESPACE)
      .then((raw) => {
        // No document at all is the only case that seeds. A library somebody
        // emptied on purpose has a document, and stays empty.
        if (raw === null) {
          const seeded = starterSnippets(new Date().toISOString())
          setSnippets(sortSnippets(seeded))
          writeDoc(SNIPPET_NAMESPACE, {
            version: SNIPPET_DOC_VERSION,
            snippets: seeded,
          } satisfies SnippetDoc).catch(() => {})
          return
        }
        setDocNote(docWarning(raw))
        setSnippets(sortSnippets(ensureIds(migrateDoc(raw).snippets, newSnippetId)))
      })
      .catch(() => setDocNote(''))
  }, [])

  const shown = useMemo(() => filterSnippets(snippets, query, group), [snippets, query, group])

  const groupOptions = useMemo(
    () => [
      { value: '', label: 'All groups' },
      ...groupNames(snippets).map((name) => ({ value: name, label: name })),
    ],
    [snippets],
  )

  const openAdd = () => {
    setDraft({ ...EMPTY_DRAFT, group })
    setEditingId(null)
    setErrors({})
  }

  const openEdit = (snippet: Snippet) => {
    setDraft(draftOf(snippet))
    setEditingId(snippet.id)
    setErrors({})
  }

  const remove = (id: string) => {
    persist(snippets.filter((snippet) => snippet.id !== id))
    setConfirmId(null)
  }

  const saveDraft = () => {
    if (draft === null) return
    const checked = validateSnippet(draft)
    if (!checked.ok) {
      setErrors(checked.errors)
      return
    }
    const key = snippetKey(checked.fields)
    const clash = snippets.find(
      (snippet) => snippetKey(snippet) === key && snippet.id !== editingId,
    )
    if (clash !== undefined) {
      setErrors({
        title: `${clash.group === '' ? 'This library' : clash.group} already has a snippet called "${clash.title}". Edit that one, or give this a different title.`,
      })
      return
    }
    setErrors({})

    const at = new Date().toISOString()
    const next =
      editingId === null
        ? [...snippets, { id: newSnippetId(), ...checked.fields, addedAt: at, updatedAt: at }]
        : snippets.map((snippet) =>
            snippet.id === editingId ? { ...snippet, ...checked.fields, updatedAt: at } : snippet,
          )
    persist(sortSnippets(next))
    setDraft(null)
    setEditingId(null)
  }

  const onFile = async (file: File) => {
    const result = readImport(await file.text())
    if (!result.ok) {
      toast.push('error', result.error)
      return
    }
    const report = mergeSnippets(snippets, result.doc, newSnippetId, new Date().toISOString())
    if (report.error !== '') {
      toast.push('error', report.error)
      return
    }
    persist(report.snippets)
    toast.push(
      'success',
      `Imported: ${report.added} added, ${report.updated} updated, ${report.skipped} skipped.`,
    )
  }

  const onExport = () => {
    const now = new Date().toISOString()
    downloadJson(exportFileName(group, now), exportDoc(shown, now))
  }

  return (
    <ToolShell
      title="Shared Snippet Library"
      description="Keep the commands, paths and canned replies the team uses, one click from the clipboard."
      help={HELP}
      actions={
        <>
          <Button
            variant="primary"
            onClick={openAdd}
            disabled={draft !== null}
            icon={<Plus size={14} aria-hidden />}
          >
            Add snippet
          </Button>
          <Button onClick={() => fileRef.current?.click()} icon={<FileUp size={14} aria-hidden />}>
            Import
          </Button>
          <Button
            onClick={onExport}
            disabled={shown.length === 0}
            icon={<FileDown size={14} aria-hidden />}
          >
            Export
          </Button>
        </>
      }
    >
      <div className="flex max-w-4xl flex-col gap-4">
        {!storeReady && <p className="text-xs text-warn">{NO_STORE_MESSAGE}</p>}
        {docNote !== '' && (
          <p role="alert" className="text-xs text-danger">
            {docNote}
          </p>
        )}

        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <TextInput
              label="Search snippets"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search titles, commands and tags"
              spellCheck={false}
              autoComplete="off"
            />
          </div>
          <div className="min-w-40">
            <Select
              label="Group"
              options={groupOptions}
              value={group}
              onChange={(event) => setGroup(event.target.value)}
            />
          </div>
        </div>

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

        {draft !== null && (
          <form
            className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
            onSubmit={(event) => {
              event.preventDefault()
              saveDraft()
            }}
          >
            <p className="text-xs font-medium text-fg-muted">
              {editingId === null ? 'Add a snippet' : 'Edit snippet'}
            </p>
            <TextInput
              label="Title"
              value={draft.title}
              onChange={(event) => setDraft({ ...draft, title: event.target.value })}
              placeholder="Flush the DNS cache"
              error={errors.title}
            />
            <div className="flex flex-wrap gap-3">
              <div className="min-w-40 flex-1">
                <TextInput
                  label="Group"
                  value={draft.group}
                  onChange={(event) => setDraft({ ...draft, group: event.target.value })}
                  placeholder="Windows"
                  error={errors.group}
                />
              </div>
              <div className="min-w-40 flex-1">
                <TextInput
                  label="Tags"
                  value={draft.tags}
                  onChange={(event) => setDraft({ ...draft, tags: event.target.value })}
                  placeholder="dns, cache"
                  hint="Separated by commas."
                />
              </div>
            </div>
            <Textarea
              id="snippet-body"
              label="Snippet"
              error={errors.body}
              rows={6}
              value={draft.body}
              onChange={(event) => setDraft({ ...draft, body: event.target.value })}
              spellCheck={false}
              className="resize-y font-mono text-xs"
            />
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary">
                Save snippet
              </Button>
              <Button
                onClick={() => {
                  setDraft(null)
                  setEditingId(null)
                  setErrors({})
                }}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}

        {shown.length === 0 ? (
          <p className="text-sm text-fg-muted">
            {snippets.length === 0
              ? 'All your snippets are gone. Press Add snippet to start again, or Import a file a colleague exported.'
              : 'Nothing matches that search. Clear the search box or pick another group.'}
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {shown.map((snippet) => (
              <div
                key={snippet.id}
                className="rounded border border-border bg-surface-2 px-3 py-2"
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-fg">{snippet.title}</p>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {snippet.group !== '' && (
                        <span className="rounded bg-surface-3 px-1.5 py-0.5 text-xs text-fg-muted">
                          {snippet.group}
                        </span>
                      )}
                      {snippet.tags.map((tag) => (
                        <span
                          key={tag}
                          className="rounded bg-surface-3 px-1.5 py-0.5 text-xs text-fg-muted"
                        >
                          {tag}
                        </span>
                      ))}
                    </div>
                  </div>
                  {confirmId === snippet.id ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-xs text-fg">Delete "{snippet.title}"?</span>
                      <Button size="sm" variant="danger" onClick={() => remove(snippet.id)}>
                        Delete
                      </Button>
                      <Button size="sm" onClick={() => setConfirmId(null)}>
                        Cancel
                      </Button>
                    </div>
                  ) : (
                    <div className="flex shrink-0 items-center gap-1">
                      <CopyButton value={snippet.body} label="Copy" />
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Edit ${snippet.title}`}
                        onClick={() => openEdit(snippet)}
                        icon={<Pencil size={14} aria-hidden />}
                      />
                      <Button
                        size="sm"
                        variant="ghost"
                        aria-label={`Delete ${snippet.title}`}
                        onClick={() => setConfirmId(snippet.id)}
                        icon={<Trash2 size={14} aria-hidden />}
                      />
                    </div>
                  )}
                </div>
                <pre className="mt-1.5 overflow-x-auto rounded bg-surface px-2 py-1.5 font-mono text-xs whitespace-pre-wrap text-fg">
                  {snippet.body}
                </pre>
              </div>
            ))}
          </div>
        )}

        <p className="text-xs text-fg-muted">
          {snippets.length === shown.length
            ? `${snippets.length} snippets saved.`
            : `${snippets.length} snippets saved, ${shown.length} shown.`}
        </p>
      </div>
    </ToolShell>
  )
}
