import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  Copy,
  FileDown,
  FileUp,
  Pencil,
  Play,
  Plus,
  Trash2,
} from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  Select,
  StatusDot,
  TextInput,
  ToolShell,
  useToast,
  type StatusTone,
} from '../../components'
import { downloadJson, downloadText } from '../../lib/download'
import { getAppInfo, readDoc, writeDoc } from '../../shell/bindings'
import {
  checklistsFileName,
  docWarning,
  ensureIds,
  exportChecklistsDoc,
  exportRunsDoc,
  formatRun,
  mergeChecklists,
  migrateDoc,
  moveItem,
  newChecklistId,
  newItemId,
  newRunId,
  readImport,
  reportFileName,
  runLabel,
  runTally,
  runsFileName,
  sortRuns,
  stampOf,
  startRun,
  starterChecklists,
  validateChecklist,
  validateNote,
  validateSite,
  CHECKLIST_DOC_VERSION,
  CHECKLIST_NAMESPACE,
  NO_STORE_MESSAGE,
  SAVE_FAILED_MESSAGE,
  type Checklist,
  type ChecklistDoc,
  type ChecklistErrors,
  type ItemState,
  type Run,
} from './checklists'

const STATE_OPTIONS = [
  { value: 'todo', label: 'To do' },
  { value: 'done', label: 'Done' },
  { value: 'skipped', label: 'Skipped' },
  { value: 'na', label: 'Not applicable' },
]

const STATE_TONE: Record<ItemState, StatusTone> = {
  todo: 'idle',
  done: 'ok',
  skipped: 'warn',
  na: 'idle',
}

const STATE_LABEL: Record<ItemState, string> = {
  todo: 'To do',
  done: 'Done',
  skipped: 'Skipped',
  na: 'Not applicable',
}

const HELP = (
  <>
    <p>
      A checklist is the procedure. A run is one time you carried it out. Starting a run copies the
      items as they are right now, so improving the checklist afterwards never rewrites a job you
      have already done.
    </p>
    <p className="mt-1.5">
      Mark an item Skipped when it should have happened and did not, and Not applicable when it never
      applied to this machine. The difference matters to whoever reads the report: one is an open
      loop and the other is not. The note beside each item is where the decision goes.
    </p>
    <p className="mt-1.5">
      Copy report gives you a dated, itemised record to paste into the ticket. That is what "proof of
      work" means here: a plain record of what you did, written by you. Nothing is signed and nothing
      is sent anywhere.
    </p>
    <p className="mt-1.5">
      Export on the Checklists view writes the procedures as one file, which is how the team keeps
      the same build steps on every laptop.
    </p>
  </>
)

interface Editor {
  id: string | null
  name: string
  description: string
  items: string[]
}

export default function SiteChecklistPage() {
  const toast = useToast()

  const [checklists, setChecklists] = useState<Checklist[]>([])
  const [runs, setRuns] = useState<Run[]>([])
  const [storeReady, setStoreReady] = useState(true)
  const [docNote, setDocNote] = useState('')

  const [view, setView] = useState<'runs' | 'checklists'>('runs')
  const [pickedList, setPickedList] = useState('')
  const [site, setSite] = useState('')
  const [siteError, setSiteError] = useState<string>()
  const [openRunId, setOpenRunId] = useState<string | null>(null)
  const [confirmRunId, setConfirmRunId] = useState<string | null>(null)
  const [confirmListId, setConfirmListId] = useState<string | null>(null)
  const [editor, setEditor] = useState<Editor | null>(null)
  const [editorErrors, setEditorErrors] = useState<ChecklistErrors>({})
  const fileRef = useRef<HTMLInputElement>(null)

  const persist = useCallback(
    (nextChecklists: Checklist[], nextRuns: Run[]) => {
      setChecklists(nextChecklists)
      setRuns(nextRuns)
      writeDoc(CHECKLIST_NAMESPACE, {
        version: CHECKLIST_DOC_VERSION,
        checklists: nextChecklists,
        runs: nextRuns,
      } satisfies ChecklistDoc).catch(() => toast.push('error', SAVE_FAILED_MESSAGE))
    },
    [toast],
  )

  useEffect(() => {
    void getAppInfo().then((info) => setStoreReady(info !== null))
    readDoc<unknown>(CHECKLIST_NAMESPACE)
      .then((raw) => {
        if (raw === null) {
          const seeded = starterChecklists(new Date().toISOString())
          setChecklists(seeded)
          setPickedList(seeded[0].id)
          writeDoc(CHECKLIST_NAMESPACE, {
            version: CHECKLIST_DOC_VERSION,
            checklists: seeded,
            runs: [],
          } satisfies ChecklistDoc).catch(() => {})
          return
        }
        setDocNote(docWarning(raw))
        const doc = migrateDoc(raw)
        const fixed = ensureIds(doc.checklists, doc.runs, newChecklistId, newItemId, newRunId)
        setChecklists(fixed.checklists)
        setRuns(sortRuns(fixed.runs))
        setPickedList(fixed.checklists[0]?.id ?? '')
        setOpenRunId(sortRuns(fixed.runs)[0]?.id ?? null)
      })
      .catch(() => setDocNote(''))
  }, [])

  const openRun = runs.find((run) => run.id === openRunId) ?? null
  const tally = openRun === null ? null : runTally(openRun)
  const report = openRun === null ? '' : formatRun(openRun)

  const listOptions = useMemo(
    () =>
      checklists.length === 0
        ? [{ value: '', label: 'No checklists yet' }]
        : checklists.map((list) => ({ value: list.id, label: list.name })),
    [checklists],
  )

  const runOptions = useMemo(
    () =>
      runs.length === 0
        ? [{ value: '', label: 'No runs yet' }]
        : sortRuns(runs).map((run) => ({ value: run.id, label: runLabel(run) })),
    [runs],
  )

  const updateRun = (id: string, change: Partial<Run>) => {
    const at = new Date().toISOString()
    persist(
      checklists,
      runs.map((run) => (run.id === id ? { ...run, ...change, updatedAt: at } : run)),
    )
  }

  const onStartRun = () => {
    const problem = validateSite(site)
    if (problem !== undefined) {
      setSiteError(problem)
      return
    }
    setSiteError(undefined)
    const list = checklists.find((entry) => entry.id === pickedList)
    if (list === undefined) return
    const now = new Date()
    const run = startRun(list, site, newRunId, stampOf(now), now.toISOString())
    persist(checklists, [run, ...runs])
    setOpenRunId(run.id)
  }

  const onDeleteRun = (id: string) => {
    const next = runs.filter((run) => run.id !== id)
    persist(checklists, next)
    setConfirmRunId(null)
    if (openRunId === id) setOpenRunId(sortRuns(next)[0]?.id ?? null)
  }

  const openEditor = (list: Checklist | null) => {
    setEditorErrors({})
    setEditor(
      list === null
        ? { id: null, name: '', description: '', items: [''] }
        : {
            id: list.id,
            name: list.name,
            description: list.description,
            items: list.items.map((item) => item.text),
          },
    )
  }

  const saveEditor = () => {
    if (editor === null) return
    const checked = validateChecklist(editor.name, editor.items)
    if (!checked.ok) {
      setEditorErrors(checked.errors)
      return
    }
    setEditorErrors({})
    const at = new Date().toISOString()
    const items = checked.items.map((text) => ({ id: newItemId(), text }))
    const next =
      editor.id === null
        ? [
            ...checklists,
            {
              id: newChecklistId(),
              name: checked.name,
              description: editor.description.trim(),
              items,
              addedAt: at,
              updatedAt: at,
            },
          ]
        : checklists.map((list) =>
            list.id === editor.id
              ? {
                  ...list,
                  name: checked.name,
                  description: editor.description.trim(),
                  items,
                  updatedAt: at,
                }
              : list,
          )
    persist(next, runs)
    if (pickedList === '') setPickedList(next[0].id)
    setEditor(null)
  }

  const duplicate = (list: Checklist) => {
    const at = new Date().toISOString()
    persist(
      [
        ...checklists,
        {
          ...list,
          id: newChecklistId(),
          name: `${list.name} (copy)`,
          items: list.items.map((item) => ({ id: newItemId(), text: item.text })),
          addedAt: at,
          updatedAt: at,
        },
      ],
      runs,
    )
  }

  const deleteChecklist = (id: string) => {
    const next = checklists.filter((list) => list.id !== id)
    persist(next, runs)
    setConfirmListId(null)
    if (pickedList === id) setPickedList(next[0]?.id ?? '')
  }

  const onFile = async (file: File) => {
    const result = readImport(await file.text())
    if (!result.ok) {
      toast.push('error', result.error)
      return
    }
    const merged = mergeChecklists(
      checklists,
      result.doc,
      newChecklistId,
      newItemId,
      new Date().toISOString(),
    )
    if (merged.error !== '') {
      toast.push('error', merged.error)
      return
    }
    persist(merged.checklists, runs)
    if (pickedList === '' && merged.checklists.length > 0) setPickedList(merged.checklists[0].id)
    toast.push(
      'success',
      `Imported: ${merged.added} added, ${merged.unchanged} already the same, ${merged.skipped} skipped.`,
    )
  }

  const runsForChecklist = (id: string) => runs.filter((run) => run.checklistId === id).length

  return (
    <ToolShell
      title="Site Checklist Runner"
      description="Work through a checklist on site and export the finished run as proof it was done."
      help={HELP}
      actions={
        <>
          <Button variant={view === 'runs' ? 'primary' : 'ghost'} onClick={() => setView('runs')}>
            Runs
          </Button>
          <Button
            variant={view === 'checklists' ? 'primary' : 'ghost'}
            onClick={() => setView('checklists')}
          >
            Checklists
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

        {view === 'runs' && (
          <>
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(event) => {
                event.preventDefault()
                onStartRun()
              }}
            >
              <div className="min-w-48">
                <Select
                  label="Checklist"
                  options={listOptions}
                  value={pickedList}
                  onChange={(event) => setPickedList(event.target.value)}
                />
              </div>
              <div className="min-w-56 flex-1">
                <TextInput
                  label="Site or machine"
                  value={site}
                  onChange={(event) => setSite(event.target.value)}
                  placeholder="Head Office, laptop CH-LT-042"
                  error={siteError}
                />
              </div>
              <Button
                type="submit"
                variant="primary"
                disabled={checklists.length === 0}
                icon={<Play size={14} aria-hidden />}
              >
                Start run
              </Button>
            </form>

            {runs.length > 0 && (
              <div className="flex flex-wrap items-end gap-2">
                <div className="min-w-72 flex-1">
                  <Select
                    label="Open run"
                    options={runOptions}
                    value={openRunId ?? ''}
                    onChange={(event) => setOpenRunId(event.target.value)}
                  />
                </div>
                {openRun !== null &&
                  (confirmRunId === openRun.id ? (
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-fg">Delete this run?</span>
                      <Button variant="danger" onClick={() => onDeleteRun(openRun.id)}>
                        Delete
                      </Button>
                      <Button onClick={() => setConfirmRunId(null)}>Cancel</Button>
                    </div>
                  ) : (
                    <Button
                      variant="ghost"
                      onClick={() => setConfirmRunId(openRun.id)}
                      icon={<Trash2 size={14} aria-hidden />}
                    >
                      Delete run
                    </Button>
                  ))}
              </div>
            )}

            {openRun === null || tally === null ? (
              <p className="text-sm text-fg-muted">
                No run open. Pick a checklist above and press Start run.
              </p>
            ) : (
              <>
                <ProgressBar
                  value={tally.dealtWith}
                  max={tally.total}
                  label={`${tally.dealtWith} of ${tally.total} items dealt with`}
                />
                <p className="text-xs text-fg-muted">
                  {[
                    `${tally.done} done`,
                    tally.skipped > 0 ? `${tally.skipped} skipped` : '',
                    tally.na > 0 ? `${tally.na} not applicable` : '',
                    tally.todo > 0 ? `${tally.todo} still to do` : '',
                  ]
                    .filter((part) => part !== '')
                    .join(', ')}
                  .
                </p>

                <div className="flex flex-col gap-2">
                  {openRun.items.map((item, at) => (
                    <div
                      key={item.id}
                      className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <StatusDot status={STATE_TONE[item.state]} />
                        <span className="min-w-0 flex-1 text-sm text-fg">{item.text}</span>
                        <div className="w-44">
                          <Select
                            aria-label={`State of "${item.text}"`}
                            options={STATE_OPTIONS}
                            value={item.state}
                            onChange={(event) =>
                              updateRun(openRun.id, {
                                items: openRun.items.map((entry, index) =>
                                  index === at
                                    ? { ...entry, state: event.target.value as ItemState }
                                    : entry,
                                ),
                              })
                            }
                          />
                        </div>
                        <span className="w-28 shrink-0 text-xs text-fg-muted">
                          {STATE_LABEL[item.state]}
                        </span>
                      </div>
                      <TextInput
                        label="Note"
                        value={item.note}
                        onChange={(event) =>
                          updateRun(openRun.id, {
                            items: openRun.items.map((entry, index) =>
                              index === at ? { ...entry, note: event.target.value } : entry,
                            ),
                          })
                        }
                        placeholder="Anything worth recording"
                        error={validateNote(item.note)}
                      />
                    </div>
                  ))}
                </div>

                <div className="flex flex-wrap gap-2">
                  {openRun.finishedStamp === '' ? (
                    <Button
                      variant="primary"
                      onClick={() =>
                        updateRun(openRun.id, { finishedStamp: stampOf(new Date()) })
                      }
                    >
                      Finish run
                    </Button>
                  ) : (
                    <Button onClick={() => updateRun(openRun.id, { finishedStamp: '' })}>
                      Reopen run
                    </Button>
                  )}
                  <CopyButton value={report} label="Copy report" />
                  <Button
                    onClick={() =>
                      downloadText(
                        reportFileName(openRun, new Date().toISOString()),
                        report,
                        'text/plain',
                      )
                    }
                    icon={<FileDown size={14} aria-hidden />}
                  >
                    Download report
                  </Button>
                  <Button
                    onClick={() => {
                      const now = new Date().toISOString()
                      downloadJson(runsFileName(now), exportRunsDoc(runs, now))
                    }}
                    icon={<FileDown size={14} aria-hidden />}
                  >
                    Export runs
                  </Button>
                </div>

                <pre className="overflow-x-auto rounded border border-border bg-surface-2 px-3 py-2 font-mono text-xs whitespace-pre-wrap text-fg">
                  {report}
                </pre>
              </>
            )}
          </>
        )}

        {view === 'checklists' && (
          <>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="primary"
                onClick={() => openEditor(null)}
                icon={<Plus size={14} aria-hidden />}
              >
                New checklist
              </Button>
              <Button onClick={() => fileRef.current?.click()} icon={<FileUp size={14} aria-hidden />}>
                Import
              </Button>
              <Button
                onClick={() => {
                  const now = new Date().toISOString()
                  downloadJson(checklistsFileName(now), exportChecklistsDoc(checklists, now))
                }}
                disabled={checklists.length === 0}
                icon={<FileDown size={14} aria-hidden />}
              >
                Export
              </Button>
            </div>

            {checklists.length === 0 ? (
              <p className="text-sm text-fg-muted">
                No checklists yet. Press New checklist, or Import a file a colleague exported.
              </p>
            ) : (
              <div className="flex flex-col gap-2">
                {checklists.map((list) => (
                  <div key={list.id} className="rounded border border-border bg-surface-2 px-3 py-2">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-fg">{list.name}</p>
                        {list.description !== '' && (
                          <p className="text-xs text-fg-muted">{list.description}</p>
                        )}
                        <p className="text-xs text-fg-muted">
                          {list.items.length} {list.items.length === 1 ? 'item' : 'items'}
                        </p>
                      </div>
                      {confirmListId === list.id ? (
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="text-xs text-fg">
                            Delete "{list.name}"? The {runsForChecklist(list.id)} runs already made
                            from it are kept.
                          </span>
                          <Button size="sm" variant="danger" onClick={() => deleteChecklist(list.id)}>
                            Delete
                          </Button>
                          <Button size="sm" onClick={() => setConfirmListId(null)}>
                            Cancel
                          </Button>
                        </div>
                      ) : (
                        <div className="flex shrink-0 items-center gap-1">
                          <Button
                            size="sm"
                            onClick={() => {
                              setPickedList(list.id)
                              setView('runs')
                            }}
                            icon={<Play size={14} aria-hidden />}
                          >
                            Start run
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={`Edit ${list.name}`}
                            onClick={() => openEditor(list)}
                            icon={<Pencil size={14} aria-hidden />}
                          />
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={`Duplicate ${list.name}`}
                            onClick={() => duplicate(list)}
                            icon={<Copy size={14} aria-hidden />}
                          />
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={`Delete ${list.name}`}
                            onClick={() => setConfirmListId(list.id)}
                            icon={<Trash2 size={14} aria-hidden />}
                          />
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {editor !== null && (
              <form
                className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
                onSubmit={(event) => {
                  event.preventDefault()
                  saveEditor()
                }}
              >
                <p className="text-xs font-medium text-fg-muted">
                  {editor.id === null ? 'New checklist' : 'Edit checklist'}
                </p>
                <TextInput
                  label="Name"
                  value={editor.name}
                  onChange={(event) => setEditor({ ...editor, name: event.target.value })}
                  placeholder="New PC setup"
                  error={editorErrors.name}
                />
                <TextInput
                  label="Description"
                  value={editor.description}
                  onChange={(event) => setEditor({ ...editor, description: event.target.value })}
                  placeholder="Everything a new machine needs before it reaches a user."
                />

                <div className="flex flex-col gap-2">
                  {editor.items.map((item, at) => (
                    <div key={at} className="flex flex-wrap items-end gap-2">
                      <div className="min-w-56 flex-1">
                        <TextInput
                          label={`Item ${at + 1}`}
                          value={item}
                          onChange={(event) =>
                            setEditor({
                              ...editor,
                              items: editor.items.map((entry, index) =>
                                index === at ? event.target.value : entry,
                              ),
                            })
                          }
                          error={editorErrors.itemAt?.[at]}
                        />
                      </div>
                      <Button
                        variant="ghost"
                        aria-label={`Move item ${at + 1} up`}
                        disabled={at === 0}
                        onClick={() => setEditor({ ...editor, items: moveItem(editor.items, at, -1) })}
                        icon={<ChevronUp size={14} aria-hidden />}
                      />
                      <Button
                        variant="ghost"
                        aria-label={`Move item ${at + 1} down`}
                        disabled={at === editor.items.length - 1}
                        onClick={() => setEditor({ ...editor, items: moveItem(editor.items, at, 1) })}
                        icon={<ChevronDown size={14} aria-hidden />}
                      />
                      <Button
                        variant="ghost"
                        aria-label={`Remove item ${at + 1}`}
                        onClick={() =>
                          setEditor({
                            ...editor,
                            items: editor.items.filter((_, index) => index !== at),
                          })
                        }
                        icon={<Trash2 size={14} aria-hidden />}
                      />
                    </div>
                  ))}
                </div>

                {editorErrors.items !== undefined && (
                  <p role="alert" className="text-xs text-danger">
                    {editorErrors.items}
                  </p>
                )}

                <div className="flex flex-wrap gap-2">
                  <Button
                    onClick={() => setEditor({ ...editor, items: [...editor.items, ''] })}
                    icon={<Plus size={14} aria-hidden />}
                  >
                    Add item
                  </Button>
                  <Button type="submit" variant="primary">
                    Save checklist
                  </Button>
                  <Button
                    onClick={() => {
                      setEditor(null)
                      setEditorErrors({})
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            )}
          </>
        )}
      </div>
    </ToolShell>
  )
}
