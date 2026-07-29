import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ChevronDown,
  ChevronUp,
  ChevronsDown,
  ChevronsUp,
  FileText,
  Pause,
  Play,
  Search,
  Square,
} from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  ResultsTable,
  TextInput,
  ToolShell,
  useToast,
  type Column,
} from '../../components'
import { cn, errorMessage, formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import {
  openLog,
  pickLogFile,
  readLog,
  startLogSearch,
  type Chunk,
  type Info,
  type Match,
} from './api'
import { fileFacts, gutterLabel, levelClass, levelTag, splitMatch, visibleText, windowLabel } from './lines'

// Follow re-reads the tail on this interval. One second is fast enough to watch
// a service restart and slow enough not to hammer a network share.
const FOLLOW_MS = 1000

export default function LogViewerPage() {
  const toast = useToast()
  const search = useJob<Match>()

  const [path, setPath] = useState('')
  const [info, setInfo] = useState<Info | null>(null)
  const [chunk, setChunk] = useState<Chunk | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [matchCase, setMatchCase] = useState(false)
  const [following, setFollowing] = useState(false)
  const [jumpedTo, setJumpedTo] = useState<number | null>(null)

  const paneRef = useRef<HTMLDivElement>(null)

  const load = useCallback(
    async (target: string, where: string, offset: number) => {
      try {
        const next = await readLog({ path: target, where, offset, lines: 500 })
        setChunk(next)
        setError(null)
        if (next.shrank) {
          toast.push('info', 'The log was rotated, so this is the new file from the start.')
        }
        return next
      } catch (err) {
        setError(errorMessage(err))
        return null
      }
    },
    [toast],
  )

  const open = useCallback(
    async (target: string) => {
      const text = target.trim()
      if (text === '') {
        setError('Choose a log file, or type the full path to it.')
        return
      }
      setFollowing(false)
      setJumpedTo(null)
      search.reset()
      try {
        const opened = await openLog(text)
        setInfo(opened)
        setPath(text)
        setError(null)
        await load(text, 'tail', 0)
      } catch (err) {
        setInfo(null)
        setChunk(null)
        setError(errorMessage(err))
      }
    },
    [load, search],
  )

  // Follow mode: re-read the tail on a timer and stay pinned to the bottom.
  useEffect(() => {
    if (!following || info === null) return
    let stopped = false
    const timer = setInterval(() => {
      void readLog({ path: info.path, where: 'tail', offset: 0, lines: 500 })
        .then((next) => {
          if (stopped) return
          setChunk(next)
          const pane = paneRef.current
          if (pane) pane.scrollTop = pane.scrollHeight
        })
        .catch(() => {
          if (stopped) return
          setFollowing(false)
          toast.push('error', 'Following stopped: that file is not there any more.')
        })
    }, FOLLOW_MS)
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [following, info, toast])

  const runSearch = useCallback(async () => {
    if (info === null) return
    if (query.trim() === '') {
      setError('Type something to look for.')
      return
    }
    setError(null)
    await search.start(() => startLogSearch({ path: info.path, query, matchCase }))
  }, [info, matchCase, query, search])

  const jumpTo = useCallback(
    async (offset: number) => {
      if (info === null) return
      setFollowing(false)
      const next = await load(info.path, 'at', offset)
      if (next === null) {
        toast.push('error', 'That part of the file is gone: it has been rotated or trimmed since the search.')
        return
      }
      setJumpedTo(offset)
    },
    [info, load, toast],
  )

  const lines = chunk?.lines ?? []
  const matches = useMemo(() => {
    const byOffset = new Map<number, Match>()
    for (const match of search.results) byOffset.set(match.offset, match)
    return Array.from(byOffset.values())
  }, [search.results])

  const searchSummary = search.done?.summary ?? {}
  const searchNote = typeof searchSummary.note === 'string' ? searchSummary.note : ''

  const columns = useMemo<Column<Match>[]>(
    () => [
      {
        key: 'number',
        header: 'Line',
        align: 'right',
        width: '7rem',
        value: (row) => (row.number > 0 ? row.number : null),
      },
      { key: 'level', header: 'Level', width: '5rem', value: (row) => levelTag(row.level) },
      {
        key: 'text',
        header: 'Line text',
        render: (row) => {
          const [before, hit, after] = splitMatch(row.text, row.col, query.length)
          return (
            <button
              type="button"
              onClick={() => void jumpTo(row.offset)}
              className="text-left font-mono text-xs hover:underline"
              title="Show this part of the file"
            >
              {before}
              <mark className="bg-accent/30 text-fg">{hit}</mark>
              {after}
            </button>
          )
        },
      },
    ],
    [jumpTo, query],
  )

  return (
    <ToolShell
      title="Log File Viewer"
      description="Open a huge log file instantly, jump to the end, search it and watch new lines arrive."
      help={
        <>
          <p>
            Pick a log file and the last 500 lines appear straight away, however big the file is:
            only the part you are looking at is read, so a 400 MB <code>setupact.log</code> opens as
            fast as a small one. Start, Older, Newer and End move a window through the file.
          </p>
          <p className="mt-1.5">
            Find all searches the <strong>whole</strong> file and lists every matching line with its
            line number. Click a result to jump there. The search is plain text, not a pattern, so
            there is nothing to get wrong and nothing that can hang on a huge file. Turn Match case
            on when the capitals matter.
          </p>
          <p className="mt-1.5">
            Follow re-reads the end of the file every second, which is <code>tail -f</code> for
            Windows. Lines are tinted and tagged ERR, WRN or INF from words in the line itself, so
            it is a hint rather than a promise. The file is only ever opened for reading: CHIT
            cannot change a log.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            void open(path)
          }}
        >
          <div className="min-w-56 flex-1">
            <TextInput
              label="Log file"
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder="/var/log/syslog"
              spellCheck={false}
              autoComplete="off"
            />
          </div>
          <Button type="submit" variant="primary" icon={<FileText size={14} aria-hidden />}>
            Open
          </Button>
          <Button
            onClick={() => {
              void pickLogFile().then((chosen) => {
                if (chosen !== null && chosen !== '') void open(chosen)
              })
            }}
          >
            Choose file
          </Button>
        </form>

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {info !== null && (
          <>
            <p className="text-xs text-fg-muted">
              {fileFacts(info.name, info.bytes, info.modified, info.crlf)}
            </p>

            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                onClick={() => void load(info.path, 'head', 0)}
                icon={<ChevronsUp size={14} aria-hidden />}
              >
                Start
              </Button>
              <Button
                size="sm"
                disabled={chunk?.atStart !== false}
                onClick={() => void load(info.path, 'before', chunk?.start ?? 0)}
                icon={<ChevronUp size={14} aria-hidden />}
              >
                Older
              </Button>
              <Button
                size="sm"
                disabled={chunk?.atEnd !== false}
                onClick={() => void load(info.path, 'at', chunk?.end ?? 0)}
                icon={<ChevronDown size={14} aria-hidden />}
              >
                Newer
              </Button>
              <Button
                size="sm"
                onClick={() => void load(info.path, 'tail', 0)}
                icon={<ChevronsDown size={14} aria-hidden />}
              >
                End
              </Button>
              <Button
                size="sm"
                variant={following ? 'primary' : 'secondary'}
                aria-pressed={following}
                onClick={() => setFollowing((on) => !on)}
                icon={
                  following ? <Pause size={14} aria-hidden /> : <Play size={14} aria-hidden />
                }
              >
                {following ? 'Following' : 'Follow'}
              </Button>
              <CopyButton value={visibleText(lines)} label="Copy visible lines" />
            </div>

            <form
              className="flex flex-col gap-2"
              onSubmit={(event) => {
                event.preventDefault()
                if (!search.running) void runSearch()
              }}
            >
              <div className="flex flex-wrap items-end gap-2">
                <div className="min-w-48 flex-1">
                  <TextInput
                    label="Find"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder="error"
                    spellCheck={false}
                    autoComplete="off"
                  />
                </div>
                <Button
                  type="submit"
                  variant="primary"
                  disabled={search.running}
                  icon={<Search size={14} aria-hidden />}
                >
                  Find all
                </Button>
                {search.running && (
                  <Button
                    variant="danger"
                    onClick={search.cancel}
                    icon={<Square size={14} aria-hidden />}
                  >
                    Cancel
                  </Button>
                )}
              </div>
              <details className="rounded border border-border bg-surface-2">
                <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
                  Search options
                </summary>
                <div className="px-3 pt-1 pb-3">
                  <label className="flex items-center gap-2 text-sm text-fg">
                    <input
                      type="checkbox"
                      checked={matchCase}
                      onChange={(event) => setMatchCase(event.target.checked)}
                      className="size-4 accent-[var(--accent)]"
                    />
                    Match case
                  </label>
                </div>
              </details>
            </form>

            {search.running && (
              <ProgressBar
                value={search.progress.done}
                max={search.progress.total}
                label={search.progress.message === '' ? 'Searching' : search.progress.message}
              />
            )}

            {search.error !== null && (
              <p
                role="alert"
                className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
              >
                {search.error}
              </p>
            )}

            {search.done !== null && (
              <div className="rounded border border-border bg-surface-2 px-3 py-2">
                <p className="text-sm text-fg">
                  {matches.length === 1 ? '1 matching line' : `${matches.length} matching lines`} in{' '}
                  {formatDuration(search.done.durationMs)}
                  {search.done.cancelled && (
                    <span className="text-fg-muted"> (stopped early)</span>
                  )}
                </p>
                {searchNote !== '' && <p className="mt-1 text-xs text-warn">{searchNote}</p>}
              </div>
            )}

            {(search.done !== null || search.running) && (
              <ResultsTable
                columns={columns}
                rows={matches}
                getRowId={(row) => String(row.offset)}
                csvName="log-search"
                emptyMessage={
                  search.running ? 'Searching, matches appear as they are found.' : 'Nothing matched.'
                }
                rowStatus={(row) =>
                  row.level === 'error' ? 'danger' : row.level === 'warn' ? 'warn' : undefined
                }
              />
            )}

            <div
              ref={paneRef}
              className="max-h-[60vh] overflow-auto rounded border border-border bg-surface-2 p-2 font-mono text-xs"
            >
              {lines.length === 0 ? (
                <p className="px-1 py-6 text-center text-fg-muted">
                  That file is empty. If a program is meant to be writing to it, it has not written
                  anything yet.
                </p>
              ) : (
                lines.map((line) => (
                  <div
                    key={line.offset}
                    className={cn(
                      'flex gap-2 px-1',
                      jumpedTo === line.offset && 'bg-accent/20',
                    )}
                  >
                    <span className="w-24 shrink-0 text-right tabular-nums text-fg-muted">
                      {gutterLabel(line)}
                    </span>
                    <span className="w-8 shrink-0 text-fg-muted">{levelTag(line.level)}</span>
                    <span className={cn('min-w-0 flex-1 break-all whitespace-pre-wrap', levelClass(line.level))}>
                      {line.text}
                      {line.truncated && (
                        <span className="text-fg-muted"> … (line cut at 4000 characters)</span>
                      )}
                    </span>
                  </div>
                ))
              )}
            </div>

            {chunk !== null && <p className="text-xs text-fg-muted">{windowLabel(chunk)}</p>}
          </>
        )}

        {info === null && error === null && (
          <p className="rounded border border-border bg-surface-2 px-3 py-8 text-center text-sm text-fg-muted">
            Choose a log file to read. Even a multi-gigabyte file opens straight away, because only
            the part you are looking at is read.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
