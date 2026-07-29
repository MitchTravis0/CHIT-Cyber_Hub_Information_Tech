import { useMemo, useRef, useState } from 'react'
import { ArrowLeftRight, Eraser, FolderOpen, TriangleAlert } from 'lucide-react'
import { Button, CopyButton, Textarea, ToolShell } from '../../components'
import {
  changedText,
  collapse,
  CONTEXT_LINES,
  diffTexts,
  MAX_RENDERED_ROWS,
  splitLines,
  type DiffRow,
} from './diff'

const EMPTY =
  'Paste the working version on the left and the current one on the right. The lines that differ appear here.'

const TRUNCATED =
  'Only the first 10,000 lines of each side were compared. Split the file, or use the Log File Viewer to find the part you care about.'

const TOO_DIFFERENT =
  'These two texts differ by more than 3,000 lines, so they were not lined up one by one. Everything between the matching start and end is shown as removed and then added. Check you have not pasted two different files.'

const CAPPED =
  'Only the first 2,000 lines of the comparison are shown. Use "Copy the changes" to get all of them.'

const FILE_ERROR =
  'That file could not be read. It may be open in another program, or it may not be a text file.'

const ACCEPT = '.txt,.log,.cfg,.conf,.ini,.xml,.json,.csv,.yml,.yaml,text/*'

function plural(count: number, word: string): string {
  return `${count} ${word}${count === 1 ? '' : 's'}`
}

function lineHint(text: string): string {
  const count = splitLines(text).length
  return count === 0 ? 'Empty' : plural(count, 'line')
}

function Row({ row }: { row: DiffRow }) {
  const tone = row.kind === 'removed' ? 'bg-danger/10' : row.kind === 'added' ? 'bg-ok/10' : ''
  const marker = row.kind === 'removed' ? '-' : row.kind === 'added' ? '+' : ' '
  return (
    <div className={`flex gap-2 whitespace-pre px-2 py-0.5 ${tone}`}>
      <span className="w-12 shrink-0 text-right text-fg-muted">{row.left ?? ''}</span>
      <span className="w-12 shrink-0 text-right text-fg-muted">{row.right ?? ''}</span>
      <span className="w-4 shrink-0 text-fg-muted">{marker}</span>
      <span className="text-fg">{row.text === '' ? ' ' : row.text}</span>
    </div>
  )
}

export default function TextDiffPage() {
  const [left, setLeft] = useState('')
  const [right, setRight] = useState('')
  const [ignoreWhitespace, setIgnoreWhitespace] = useState(false)
  const [ignoreCase, setIgnoreCase] = useState(false)
  const [fileError, setFileError] = useState<string | null>(null)
  const leftFile = useRef<HTMLInputElement>(null)
  const rightFile = useRef<HTMLInputElement>(null)

  const result = useMemo(
    () => diffTexts(left, right, { ignoreWhitespace, ignoreCase }),
    [left, right, ignoreWhitespace, ignoreCase],
  )

  const blocks = useMemo(() => collapse(result.rows, CONTEXT_LINES), [result.rows])

  // The clipboard gets everything; the screen stops at the cap so a 3,000 line
  // change cannot lock the page up.
  const shown = useMemo(() => {
    let budget = MAX_RENDERED_ROWS
    const out: typeof blocks = []
    for (const block of blocks) {
      if (block.kind === 'gap') {
        out.push(block)
        continue
      }
      if (budget <= 0) break
      out.push(block.rows.length <= budget ? block : { ...block, rows: block.rows.slice(0, budget) })
      budget -= block.rows.length
    }
    return out
  }, [blocks])

  const capped =
    blocks.reduce((total, block) => total + block.rows.length, 0) > MAX_RENDERED_ROWS

  const onFile = async (event: React.ChangeEvent<HTMLInputElement>, set: (text: string) => void) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (file === undefined) return
    try {
      set(await file.text())
      setFileError(null)
    } catch {
      setFileError(FILE_ERROR)
    }
  }

  const swap = () => {
    setLeft(right)
    setRight(left)
  }

  const bothEmpty = left === '' && right === ''
  const changes = result.added + result.removed

  return (
    <ToolShell
      title="Text Diff"
      description="Compare two versions of a config or an export and see exactly which lines changed."
      help={
        <>
          <p>
            Paste the version that worked on the left and the version you have now on the right, or
            open each from a file. The lines that differ appear underneath: a "-" line is in the
            left text only, a "+" line is in the right text only, and the numbers on the left of
            each row are the line numbers in each version. Long runs of identical lines are folded
            away so you only read the parts that matter.
          </p>
          <p className="mt-2">
            "Ignore indentation and spacing" treats lines that differ only in tabs and spaces as the
            same, which is what you want when one file was reformatted or came out of a different
            export tool. "Ignore capitalisation" does the same for upper and lower case. Both are
            off to start with, because a config file can care about both.
          </p>
          <p className="mt-2">
            Nothing here is sent anywhere and nothing is saved: closing the tool loses both texts,
            which is deliberate, because what gets pasted in here is usually a customer's
            configuration. If the two texts look completely different, check you have not pasted the
            same file into both boxes, or swapped a config for a log.
          </p>
        </>
      }
      actions={
        changes > 0 ? (
          <CopyButton value={changedText(result.rows)} label="Copy the changes" />
        ) : undefined
      }
    >
      <div className="flex flex-col gap-4">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="flex flex-col gap-1">
            <Textarea
              label="Before"
              value={left}
              onChange={(event) => setLeft(event.target.value)}
              rows={12}
              className="font-mono"
              spellCheck={false}
              autoFocus
              placeholder="Paste the older version here"
              hint={lineHint(left)}
            />
            <div className="flex gap-2">
              <input
                ref={leftFile}
                type="file"
                accept={ACCEPT}
                className="hidden"
                onChange={(event) => void onFile(event, setLeft)}
              />
              <Button
                size="sm"
                onClick={() => leftFile.current?.click()}
                icon={<FolderOpen size={14} aria-hidden />}
              >
                Open a file
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={left === ''}
                onClick={() => setLeft('')}
                icon={<Eraser size={14} aria-hidden />}
              >
                Clear
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-1">
            <Textarea
              label="After"
              value={right}
              onChange={(event) => setRight(event.target.value)}
              rows={12}
              className="font-mono"
              spellCheck={false}
              placeholder="Paste the newer version here"
              hint={lineHint(right)}
            />
            <div className="flex gap-2">
              <input
                ref={rightFile}
                type="file"
                accept={ACCEPT}
                className="hidden"
                onChange={(event) => void onFile(event, setRight)}
              />
              <Button
                size="sm"
                onClick={() => rightFile.current?.click()}
                icon={<FolderOpen size={14} aria-hidden />}
              >
                Open a file
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={right === ''}
                onClick={() => setRight('')}
                icon={<Eraser size={14} aria-hidden />}
              >
                Clear
              </Button>
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-4">
          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={ignoreWhitespace}
              onChange={(event) => setIgnoreWhitespace(event.target.checked)}
              className="size-4 accent-[var(--accent)]"
            />
            Ignore indentation and spacing
          </label>
          <label className="flex items-center gap-2 text-sm text-fg">
            <input
              type="checkbox"
              checked={ignoreCase}
              onChange={(event) => setIgnoreCase(event.target.checked)}
              className="size-4 accent-[var(--accent)]"
            />
            Ignore capitalisation
          </label>
          <Button size="sm" onClick={swap} icon={<ArrowLeftRight size={14} aria-hidden />}>
            Swap sides
          </Button>
        </div>

        {fileError !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {fileError}
          </p>
        )}

        {!bothEmpty && (
          <p className="text-sm text-fg">
            {result.identical
              ? result.differsOnlyByRules
                ? 'The two texts match once the ignore rules are applied.'
                : 'The two texts are identical.'
              : `${plural(result.added, 'line')} added, ${result.removed} removed, ${result.same} unchanged.`}
          </p>
        )}

        {result.truncated && (
          <p className="flex items-start gap-1.5 text-xs text-warn">
            <TriangleAlert size={14} aria-hidden className="mt-0.5 shrink-0" />
            {TRUNCATED}
          </p>
        )}
        {result.tooDifferent && (
          <p className="flex items-start gap-1.5 text-xs text-warn">
            <TriangleAlert size={14} aria-hidden className="mt-0.5 shrink-0" />
            {TOO_DIFFERENT}
          </p>
        )}
        {capped && (
          <p className="flex items-start gap-1.5 text-xs text-warn">
            <TriangleAlert size={14} aria-hidden className="mt-0.5 shrink-0" />
            {CAPPED}
          </p>
        )}

        {bothEmpty ? (
          <div className="rounded border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">
            {EMPTY}
          </div>
        ) : (
          <div className="overflow-x-auto rounded border border-border bg-surface-2 font-mono text-xs">
            {shown.map((block, index) =>
              block.kind === 'gap' ? (
                <div
                  key={`gap-${index}`}
                  className="bg-surface-3 px-2 py-1 text-center text-fg-muted"
                >
                  {plural(block.skipped, 'identical line')}
                </div>
              ) : (
                <div key={`rows-${index}`}>
                  {block.rows.map((row) => (
                    <Row key={`${row.left ?? 'x'}-${row.right ?? 'x'}-${row.kind}`} row={row} />
                  ))}
                </div>
              ),
            )}
          </div>
        )}
      </div>
    </ToolShell>
  )
}
