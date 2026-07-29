import { useEffect, useMemo, useState } from 'react'
import { FileCheck, FolderOpen, Square, TriangleAlert } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  StatusDot,
  TextInput,
  ToolShell,
  type StatusTone,
} from '../../components'
import { cn, errorMessage, formatBytes, formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { compareHash, pickHashFile, startFileHash, type Digests, type Verdict } from './api'

const DESCRIPTION =
  'Get the MD5, SHA-1 and SHA-256 of a file and check it against the value the vendor published.'

const EMPTY_PATH = 'Choose a file to check, or type the full path to it.'

const BANNER: Record<'match' | 'mismatch' | 'unknown', {
  container: string
  tone: StatusTone
  heading: string
}> = {
  match: {
    container: 'rounded border border-ok bg-ok/10 px-3 py-2',
    tone: 'ok',
    heading: 'Match',
  },
  mismatch: {
    container: 'rounded border-2 border-danger bg-danger/10 px-3 py-3',
    tone: 'danger',
    heading: 'Does not match',
  },
  unknown: {
    container: 'rounded border border-warn bg-warn/10 px-3 py-2',
    tone: 'warn',
    heading: 'Not a hash',
  },
}

/** The block the "Copy all" button puts on the clipboard. */
function summaryBlock(d: Digests): string {
  return [
    `File: ${d.name}`,
    `Size: ${d.bytes} bytes`,
    `MD5: ${d.md5}`,
    `SHA-1: ${d.sha1}`,
    `SHA-256: ${d.sha256}`,
    '',
  ].join('\n')
}

function HashRow({
  label,
  value,
  highlighted,
}: {
  label: string
  value: string
  highlighted: boolean
}) {
  return (
    <div
      className={cn(
        'flex items-center justify-between gap-1 rounded border border-border bg-surface-2 px-2 py-1.5',
        highlighted && 'ring-1 ring-accent',
      )}
    >
      <div className="min-w-0">
        <div className="text-xs text-fg-muted">
          {label}
          {highlighted && ' (the one you pasted)'}
        </div>
        <div className="font-mono text-xs break-all text-fg">{value}</div>
      </div>
      <CopyButton value={value} />
    </div>
  )
}

export default function FileHashPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Digests>()

  const [path, setPath] = useState('')
  const [pathError, setPathError] = useState<string | null>(null)
  const [expected, setExpected] = useState('')
  const [verdict, setVerdict] = useState<Verdict | null>(null)

  const digests = useMemo(() => results.at(-1) ?? null, [results])

  useEffect(() => {
    if (digests === null || expected.trim() === '') {
      setVerdict(null)
      return
    }
    let stale = false
    // CompareHash is pure and always returns a nil error, so the only way this
    // rejects is the page running outside the desktop app, where there are no
    // digests to compare in the first place.
    compareHash(expected, digests)
      .then((answer) => {
        if (!stale) setVerdict(answer)
      })
      .catch(() => {
        if (!stale) setVerdict(null)
      })
    return () => {
      stale = true
    }
  }, [expected, digests])

  const choose = async () => {
    try {
      const chosen = await pickHashFile()
      // A dismissed dialog returns an empty string. That is not a failure, so
      // the field is left exactly as it was.
      if (chosen !== '') {
        setPath(chosen)
        setPathError(null)
      }
    } catch (err) {
      setPathError(errorMessage(err))
    }
  }

  const run = async () => {
    const text = path.trim()
    if (text === '') {
      setPathError(EMPTY_PATH)
      return
    }
    setPathError(null)
    await start(async () => {
      try {
        return await startFileHash({ path: text })
      } catch (err) {
        // A rejected start is always about the path, so it belongs on the field.
        setPathError(errorMessage(err))
        throw err
      }
    })
  }

  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''
  const banner = verdict === null || verdict.state === 'none' ? null : BANNER[verdict.state]

  return (
    <ToolShell
      title="File Hash Checker"
      description={DESCRIPTION}
      help={
        <>
          <p>
            A hash is a short fingerprint of a file. Change one byte anywhere in a 4 GB ISO and the
            hash changes completely, so comparing the hash the vendor published against the hash of
            your copy is the only reliable way to know your download finished and arrived intact.
            Pick the file, press Get hashes, and all three of MD5, SHA-1 and SHA-256 are worked out
            in a single read of the file.
          </p>
          <p className="mt-1.5">
            To check a download, copy the value from the vendor's page into the Expected hash box.
            You do not need to know which of the three it is: the length gives it away, and the tool
            works it out. Upper or lower case does not matter, and you can paste the whole line from{' '}
            <code>sha256sum</code> or <code>certutil</code> including the file name. A match means
            the file is complete and unaltered. A mismatch means it is not the file the vendor
            published, and you should not install it.
          </p>
          <p className="mt-1.5">
            A big file takes a while and there is no way to speed it up: every byte has to be read.
            The bar shows how far through it is and Cancel stops it straight away. Hashing a file
            that is still being downloaded gives a hash of the part that has arrived so far, which
            will not match anything, so wait for the download to finish first.
          </p>
          <p className="mt-1.5">
            MD5 and SHA-1 are old and can be deliberately worked around by someone who wants to fool
            them, so for anything that matters use the SHA-256 value if the vendor publishes one.
            Also remember what a hash does not tell you: it proves the bytes match the value you
            were given, not that whoever gave you the value can be trusted.
          </p>
        </>
      }
      actions={
        digests === null ? undefined : (
          <CopyButton value={summaryBlock(digests)} label="Copy all" />
        )
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void run()
          }}
        >
          <div className="min-w-56 flex-1">
            <TextInput
              label="File to check"
              value={path}
              onChange={(event) => setPath(event.target.value)}
              placeholder="C:\Users\tech\Downloads\server.iso"
              hint="Press Choose file, or paste the full path."
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
              error={pathError ?? undefined}
              autoFocus
            />
          </div>
          <Button
            onClick={() => void choose()}
            disabled={running}
            icon={<FolderOpen size={14} aria-hidden />}
          >
            Choose file
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={running}
            icon={<FileCheck size={14} aria-hidden />}
          >
            Get hashes
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Cancel
            </Button>
          )}
        </form>

        {running && (
          <ProgressBar
            value={progress.done}
            max={progress.total}
            label={
              progress.message === ''
                ? 'Opening the file'
                : `${progress.message} (${formatBytes(progress.done)} of ${formatBytes(progress.total)})`
            }
          />
        )}

        {error !== null && pathError === null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {digests !== null && verdict !== null && banner !== null && (
          <div
            className={banner.container}
            role={verdict.state === 'mismatch' ? 'alert' : 'status'}
          >
            <StatusDot
              status={banner.tone}
              label={banner.heading}
              className="text-sm font-semibold text-fg"
            />
            <p className="text-sm text-fg">{verdict.message}</p>
            {(verdict.state === 'match' || verdict.state === 'mismatch') && (
              <div className="mt-1 font-mono text-xs break-all text-fg-muted">
                <div>You pasted: {verdict.expected}</div>
                <div>This file:&nbsp; {verdict.actual}</div>
              </div>
            )}
          </div>
        )}

        {digests !== null && (
          <>
            <p className="text-sm text-fg">
              {digests.name}, {formatBytes(digests.bytes)}
            </p>

            <div className="grid gap-2">
              <HashRow
                label="MD5"
                value={digests.md5}
                highlighted={verdict?.algorithm === 'MD5'}
              />
              <HashRow
                label="SHA-1"
                value={digests.sha1}
                highlighted={verdict?.algorithm === 'SHA-1'}
              />
              <HashRow
                label="SHA-256"
                value={digests.sha256}
                highlighted={verdict?.algorithm === 'SHA-256'}
              />
            </div>
          </>
        )}

        <section className="flex flex-col gap-2 border-t border-border pt-4">
          <TextInput
            label="Expected hash (optional)"
            value={expected}
            onChange={(event) => setExpected(event.target.value)}
            placeholder="Paste the value from the vendor's page"
            hint="MD5, SHA-1 or SHA-256. The whole sha256sum line works too, filename and all."
            spellCheck={false}
            autoComplete="off"
            className="font-mono"
          />
        </section>

        {digests !== null && done !== null && (
          <p className="text-sm text-fg">
            Read {formatBytes(digests.bytes)} in {formatDuration(done.durationMs)}
            {done.cancelled && <span className="text-fg-muted"> (stopped early)</span>}
          </p>
        )}
        {digests !== null && note !== '' && (
          <p className="flex items-start gap-1 text-xs text-warn">
            <TriangleAlert size={14} aria-hidden className="mt-0.5 shrink-0" />
            {note}
          </p>
        )}

        {digests === null &&
          (done?.cancelled === true ? (
            <p className="text-sm text-fg-muted">
              Stopped before the whole file was read, so there is no hash. Nothing is wrong with the
              file.
            </p>
          ) : (
            !running &&
            error === null && (
              <p className="text-sm text-fg-muted">
                Pick a file and press Get hashes. Nothing is sent anywhere: the file is read on this
                machine only.
              </p>
            )
          ))}
      </div>
    </ToolShell>
  )
}
