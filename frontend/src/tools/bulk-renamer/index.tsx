import { useEffect, useMemo, useState } from 'react'
import { Eye, FolderOpen, PenLine, TriangleAlert, Undo2 } from 'lucide-react'
import {
  Button,
  ResultsTable,
  Select,
  StatusDot,
  TextInput,
  ToolShell,
  useToast,
  type Column,
  type StatusTone,
} from '../../components'
import { errorMessage } from '../../lib/format'
import { readDoc, writeDoc } from '../../shell/bindings'
import {
  applyRename,
  pickRenameFolder,
  previewRename,
  undoRename,
  type ApplyItem,
  type ApplyResult,
  type RenameParams,
  type RenamePlan,
  type RenameRow,
} from './api'
import { RENAMER_DOC_VERSION, RENAMER_NAMESPACE, readBatch, type Batch } from './journal'

const DESCRIPTION =
  'Rename every file in one folder by a rule, with a full before-and-after preview and an undo.'

const HELP = (
  <>
    <p>
      Choose a folder, set up the rules, and press Preview. Nothing on disk changes until you press
      Apply: the table is a full before-and-after list of every file in that folder, and it is
      exactly what will happen. Only the files directly in the folder are touched. Sub-folders are
      listed but never renamed, and nothing is ever moved out of the folder or deleted.
    </p>
    <p className="mt-2">
      The rules are applied in a fixed order: find and replace, then change case, then the text you
      add at the start, then the text you add at the end, then the numbering. "Keep the file
      extension" starts switched on, which means the part after the last dot is left alone, so a
      lower-case rule will not turn <code>Report.PDF</code> into <code>report.pdf</code>. Switch it
      off if you want the extension changed too. Put <code>{'{n}'}</code> wherever you want the
      sequence number to appear; without it, the number goes on the end.
    </p>
    <p className="mt-2">
      A row marked <strong>Blocked</strong> is one the tool will not do, and the Why column says
      which of the rules it broke: two files ending up with the same name, a name that already
      exists, a name that is empty, or a character Windows does not allow. Apply stays switched off
      until every blocked row is gone. The Windows rules are checked even on a Mac or on Linux,
      because the folder is usually on a share that a Windows machine can also see.
    </p>
    <p className="mt-2">
      <strong>Undo puts the last batch back.</strong> Only the most recent one is kept, and it
      survives closing CHIT. A file that has since been moved, deleted or renamed again is left
      alone and reported, rather than guessed at.
    </p>
  </>
)

const CASE_OPTIONS = [
  { value: '', label: 'Leave alone' },
  { value: 'upper', label: 'UPPER CASE' },
  { value: 'lower', label: 'lower case' },
  { value: 'title', label: 'Title Case' },
]

const FIND_HINT = 'Leave empty to skip this step.'
const PATTERN_HINT =
  'Go pattern syntax. Use (?i) at the start to ignore capitals, and $1 in Replace with to put a captured group back.'

const ACTION_WORDS: Record<RenameRow['action'], string> = {
  rename: 'Rename',
  unchanged: 'No change',
  skipped: 'Skipped',
  blocked: 'Blocked',
}

const ACTION_TONES: Record<RenameRow['action'], StatusTone> = {
  rename: 'ok',
  unchanged: 'idle',
  skipped: 'idle',
  blocked: 'danger',
}

const STATE_WORDS: Record<ApplyItem['state'], string> = {
  renamed: 'Renamed',
  failed: 'Not renamed',
  skipped: 'Skipped',
}

const STATE_TONES: Record<ApplyItem['state'], StatusTone> = {
  renamed: 'ok',
  failed: 'danger',
  skipped: 'idle',
}

/**
 * The openings of the messages Preview returns about the folder itself. They
 * decide only whether the message is ALSO shown next to the folder field: it is
 * always shown in the alert above the table, so a message this list misses is
 * still in front of the user.
 */
const FOLDER_PROBLEMS = [
  'Choose the folder whose files you want to rename.',
  'There is no folder at ',
  ' is a file, not a folder.',
  'This computer will not let CHIT list the files in ',
  ' could not be opened.',
  'That folder has ',
]

function isFolderProblem(message: string): boolean {
  return FOLDER_PROBLEMS.some((part) => message.includes(part))
}

/** The last part of the folder path, made safe for a download file name. */
function safeName(folder: string): string {
  const base = folder.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? ''
  const safe = base.toLowerCase().replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '')
  return safe === '' ? 'folder' : safe
}

function caseOf(value: string): RenameParams['case'] {
  switch (value) {
    case 'upper':
    case 'lower':
    case 'title':
      return value
    default:
      return ''
  }
}

export default function BulkRenamerPage() {
  const toast = useToast()

  const [folder, setFolder] = useState('')
  const [folderError, setFolderError] = useState<string | null>(null)

  const [find, setFind] = useState('')
  const [replace, setReplace] = useState('')
  const [useRegex, setUseRegex] = useState(false)
  const [caseOption, setCaseOption] = useState('')
  const [prefix, setPrefix] = useState('')
  const [suffix, setSuffix] = useState('')
  const [numbering, setNumbering] = useState(false)
  const [start, setStart] = useState('1')
  const [step, setStep] = useState('1')
  const [padding, setPadding] = useState('3')
  const [keepExtension, setKeepExtension] = useState(true)

  const [plan, setPlan] = useState<RenamePlan | null>(null)
  const [stale, setStale] = useState(false)
  const [result, setResult] = useState<ApplyResult | null>(null)
  const [mode, setMode] = useState<'preview' | 'applied'>('preview')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState<Batch | null>(null)

  useEffect(() => {
    let cancelled = false
    readDoc<unknown>(RENAMER_NAMESPACE)
      .then((doc) => {
        if (!cancelled) setSaved(readBatch(doc))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  const refreshSaved = async () => {
    try {
      setSaved(readBatch(await readDoc<unknown>(RENAMER_NAMESPACE)))
    } catch {
      setSaved(null)
    }
  }

  const paramsFor = (folderText: string): RenameParams => ({
    folder: folderText,
    find,
    replace,
    useRegex,
    case: caseOf(caseOption),
    prefix,
    suffix,
    number: numbering,
    start: Number(start),
    step: Number(step),
    padding: Number(padding),
    keepExtension,
  })

  const runPreview = async (folderText: string) => {
    setBusy(true)
    try {
      const next = await previewRename(paramsFor(folderText))
      setPlan(next)
      setResult(null)
      setMode('preview')
      setStale(false)
      setError(null)
      setFolderError(null)
    } catch (err) {
      const message = errorMessage(err)
      setPlan(null)
      setError(message)
      setFolderError(isFolderProblem(message) ? message : null)
    } finally {
      setBusy(false)
    }
  }

  const chooseFolder = async () => {
    setBusy(true)
    let picked = ''
    try {
      picked = await pickRenameFolder()
    } catch (err) {
      setError(errorMessage(err))
      return
    } finally {
      setBusy(false)
    }
    // A dismissed dialog gives an empty string. That is not a failure, so the
    // field is left exactly as it was.
    if (picked === '') return
    setFolder(picked)
    await runPreview(picked)
  }

  const runApply = async () => {
    if (plan === null) return
    setBusy(true)
    try {
      const outcome = await applyRename(plan)
      setResult(outcome)
      setMode('applied')
      setPlan(null)
      setError(null)
      if (outcome.renamed > 0) {
        await writeDoc(RENAMER_NAMESPACE, {
          version: RENAMER_DOC_VERSION,
          batch: outcome.batch,
        }).catch(() => {})
        toast.push('success', `Renamed ${outcome.renamed} files.`)
      }
      await refreshSaved()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const runUndo = async () => {
    if (saved === null) return
    setBusy(true)
    try {
      const outcome = await undoRename(saved)
      setResult(outcome)
      setMode('applied')
      setPlan(null)
      setError(null)
      // The batch is cleared whatever the outcome, so the same one can never be
      // undone twice.
      await writeDoc(RENAMER_NAMESPACE, { version: RENAMER_DOC_VERSION, batch: null }).catch(
        () => {},
      )
      toast.push('success', `Put ${outcome.renamed} names back.`)
      await refreshSaved()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const previewColumns = useMemo<Column<RenameRow>[]>(
    () => [
      {
        key: 'old',
        header: 'Current name',
        render: (row) => <span className="font-mono text-xs">{row.old}</span>,
      },
      {
        key: 'new',
        header: 'New name',
        render: (row) =>
          row.action === 'blocked' || row.action === 'skipped' ? (
            <span className="font-mono text-xs text-fg-muted line-through">{row.new}</span>
          ) : (
            <span className="font-mono text-xs">{row.new}</span>
          ),
      },
      {
        key: 'action',
        header: 'What happens',
        width: '9rem',
        value: (row) => ACTION_WORDS[row.action],
        render: (row) => (
          <StatusDot status={ACTION_TONES[row.action]} label={ACTION_WORDS[row.action]} />
        ),
      },
      { key: 'reason', header: 'Why' },
    ],
    [],
  )

  const appliedColumns = useMemo<Column<ApplyItem>[]>(
    () => [
      {
        key: 'old',
        header: 'Was called',
        render: (item) => <span className="font-mono text-xs">{item.old}</span>,
      },
      {
        key: 'new',
        header: 'Now called',
        render: (item) => <span className="font-mono text-xs">{item.new}</span>,
      },
      {
        key: 'state',
        header: 'Result',
        width: '9rem',
        value: (item) => STATE_WORDS[item.state],
        render: (item) => (
          <StatusDot status={STATE_TONES[item.state]} label={STATE_WORDS[item.state]} />
        ),
      },
      { key: 'reason', header: 'Why' },
    ],
    [],
  )

  const changed = plan?.changed ?? 0
  const blocked = plan?.blocked ?? 0
  const applyDisabled = plan === null || stale || blocked > 0 || changed === 0 || busy
  const csvFolder = safeName(plan?.folder ?? result?.folder ?? folder)

  const ruleChanged = () => setStale(true)

  return (
    <ToolShell title="Bulk File Renamer" description={DESCRIPTION} help={HELP}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <TextInput
              label="Folder"
              value={folder}
              onChange={(event) => {
                setFolder(event.target.value)
                ruleChanged()
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  if (!busy) void runPreview(folder)
                }
              }}
              placeholder="C:\Users\tech\Documents\Scans"
              hint="Only the files directly in this folder. Sub-folders are never touched."
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
              error={folderError ?? undefined}
            />
          </div>
          <Button
            onClick={() => void chooseFolder()}
            disabled={busy}
            icon={<FolderOpen size={14} aria-hidden />}
          >
            Choose folder
          </Button>
        </div>

        <section className="flex flex-col gap-3 rounded border border-border bg-surface-2 p-3">
          <h2 className="text-sm font-semibold text-fg">Renaming rules</h2>

          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1">
              <TextInput
                label="Find"
                value={find}
                onChange={(event) => {
                  setFind(event.target.value)
                  ruleChanged()
                }}
                spellCheck={false}
                autoComplete="off"
                className="font-mono"
                hint={useRegex ? PATTERN_HINT : FIND_HINT}
              />
            </div>
            <div className="min-w-56 flex-1">
              <TextInput
                label="Replace with"
                value={replace}
                onChange={(event) => {
                  setReplace(event.target.value)
                  ruleChanged()
                }}
                spellCheck={false}
                autoComplete="off"
                className="font-mono"
                hint="Leave empty to remove the text you found."
              />
            </div>
          </div>

          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={useRegex}
              onChange={(event) => {
                setUseRegex(event.target.checked)
                ruleChanged()
              }}
              className="size-4 accent-[var(--accent)]"
            />
            Use a pattern (regular expression)
          </label>

          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1">
              <Select
                label="Change case"
                options={CASE_OPTIONS}
                value={caseOption}
                onChange={(event) => {
                  setCaseOption(event.target.value)
                  ruleChanged()
                }}
              />
            </div>
            <div className="min-w-56 flex-1">
              <TextInput
                label="Add at the start"
                value={prefix}
                onChange={(event) => {
                  setPrefix(event.target.value)
                  ruleChanged()
                }}
                spellCheck={false}
                autoComplete="off"
                className="font-mono"
              />
            </div>
            <div className="min-w-56 flex-1">
              <TextInput
                label="Add at the end"
                value={suffix}
                onChange={(event) => {
                  setSuffix(event.target.value)
                  ruleChanged()
                }}
                spellCheck={false}
                autoComplete="off"
                className="font-mono"
              />
            </div>
          </div>

          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={numbering}
              onChange={(event) => {
                setNumbering(event.target.checked)
                ruleChanged()
              }}
              className="size-4 accent-[var(--accent)]"
            />
            Add sequential numbers
          </label>

          {numbering && (
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-end gap-2">
                <TextInput
                  label="Start at"
                  type="number"
                  min={0}
                  max={999999}
                  className="w-28"
                  value={start}
                  onChange={(event) => {
                    setStart(event.target.value)
                    ruleChanged()
                  }}
                />
                <TextInput
                  label="Step"
                  type="number"
                  min={1}
                  max={1000}
                  className="w-28"
                  value={step}
                  onChange={(event) => {
                    setStep(event.target.value)
                    ruleChanged()
                  }}
                />
                <TextInput
                  label="Pad to"
                  type="number"
                  min={0}
                  max={10}
                  className="w-28"
                  value={padding}
                  onChange={(event) => {
                    setPadding(event.target.value)
                    ruleChanged()
                  }}
                  hint="Digits. 3 gives 001, 002."
                />
              </div>
              <p className="text-xs text-fg-muted">
                Put {'{n}'} anywhere in Add at the start, Add at the end or Replace with to say where
                the number goes. Without it the number is added to the end of the name.
              </p>
            </div>
          )}

          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={keepExtension}
              onChange={(event) => {
                setKeepExtension(event.target.checked)
                ruleChanged()
              }}
              className="size-4 accent-[var(--accent)]"
            />
            Keep the file extension
          </label>
          <p className="text-xs text-fg-muted">
            On, the part after the last dot is left alone. Off, the rules change the whole name
            including the extension.
          </p>
        </section>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            onClick={() => void runPreview(folder)}
            disabled={busy}
            icon={<Eye size={14} aria-hidden />}
          >
            Preview
          </Button>
          <Button
            variant="primary"
            onClick={() => void runApply()}
            disabled={applyDisabled}
            icon={<PenLine size={14} aria-hidden />}
          >
            {changed === 1 ? 'Apply 1 rename' : `Apply ${changed} renames`}
          </Button>
        </div>

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {plan !== null && stale && (
          <p className="flex items-center gap-1.5 text-sm text-warn">
            <TriangleAlert size={14} aria-hidden />
            The rules changed. Press Preview to see what would happen now.
          </p>
        )}

        {plan !== null && blocked > 0 && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {blocked === 1
              ? '1 of these files cannot be renamed safely, so Apply is switched off. Change the rules above until nothing is marked Blocked.'
              : `${blocked} of these files cannot be renamed safely, so Apply is switched off. Change the rules above until nothing is marked Blocked.`}
          </p>
        )}

        {plan !== null && plan.note !== '' && <p className="text-xs text-warn">{plan.note}</p>}

        {mode === 'preview' ? (
          <div className={stale ? 'opacity-50' : undefined}>
            <ResultsTable
              columns={previewColumns}
              rows={plan?.rows ?? []}
              getRowId={(row) => row.old}
              rowStatus={(row) =>
                row.action === 'blocked' ? 'danger' : row.action === 'rename' ? 'ok' : undefined
              }
              csvName={`rename-plan-${csvFolder}`}
              emptyMessage="Choose a folder and press Preview to see what would happen."
            />
          </div>
        ) : (
          <ResultsTable
            columns={appliedColumns}
            rows={result?.items ?? []}
            getRowId={(item) => item.old}
            rowStatus={(item) =>
              item.state === 'failed' ? 'danger' : item.state === 'renamed' ? 'ok' : undefined
            }
            csvName={`rename-done-${csvFolder}`}
            emptyMessage="Nothing was renamed."
          />
        )}

        {result !== null && (
          <p className="text-sm text-fg">
            Renamed {result.renamed} of {result.renamed + result.failed + result.skipped} files.
          </p>
        )}
        {result !== null && result.note !== '' && <p className="text-xs text-warn">{result.note}</p>}

        {saved !== null && (
          <section className="flex flex-wrap items-center gap-2 rounded border border-border bg-surface-2 px-3 py-2">
            <p className="flex-1 text-xs text-fg-muted">
              Last rename: {saved.renames.length} files in {saved.folder},{' '}
              {saved.appliedAt === ''
                ? 'time not recorded'
                : new Date(saved.appliedAt).toLocaleString()}
              . Only the most recent batch can be undone.
            </p>
            <Button
              onClick={() => void runUndo()}
              disabled={busy}
              icon={<Undo2 size={14} aria-hidden />}
            >
              Undo last batch
            </Button>
          </section>
        )}
      </div>
    </ToolShell>
  )
}
