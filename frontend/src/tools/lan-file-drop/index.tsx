import { useCallback, useEffect, useMemo, useState } from 'react'
import { FilePlus, FolderOpen, Send, Square, X } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  StatusDot,
  TextInput,
  ToolShell,
  useToast,
  type Column,
} from '../../components'
import { formatBytes } from '../../lib/format'
import { useJob } from '../../lib/useJob'
// The Wi-Fi QR tool's pure renderer, imported rather than copied so there is
// only one QR path builder in the app. Approved in Phase 4 for label-maker,
// which does exactly the same thing. This file is never edited.
import { modulesToPath } from '../wifi-qr/render'
import {
  fileDropAddresses,
  fileDropSession,
  generateQr,
  pickDropFiles,
  pickDropFolder,
  startFileDrop,
  type Address,
  type QrCode,
  type Transfer,
} from './api'
import {
  directionLabel,
  fileListLine,
  statusLabel,
  summaryLine,
  toPicked,
  transferId,
  transferTime,
  validPort,
  type PickedFile,
} from './session'

export default function LanFileDropPage() {
  const toast = useToast()
  const { running, results, error, done, start, cancel } = useJob<Transfer>()

  const [files, setFiles] = useState<PickedFile[]>([])
  const [port, setPort] = useState('8722')
  const [portError, setPortError] = useState<string | null>(null)
  const [allowUpload, setAllowUpload] = useState(false)
  const [receiveDir, setReceiveDir] = useState('')
  const [addresses, setAddresses] = useState<Address[]>([])
  const [chosenIp, setChosenIp] = useState('')
  const [jobId, setJobId] = useState<string | null>(null)
  const [url, setUrl] = useState('')
  const [qr, setQr] = useState<QrCode | null>(null)

  useEffect(() => {
    void fileDropAddresses().then((found) => {
      setAddresses(found)
      if (found.length > 0) setChosenIp((current) => (current === '' ? found[0].ip : current))
    })
  }, [])

  // useJob.start does not hand the job id back, and the page needs it to ask
  // for the link. The starter callback is the one place it is visible, so it is
  // captured there. Reported as a gap in useJob.
  useEffect(() => {
    if (jobId === null) return
    void fileDropSession(jobId).then((session) => {
      if (session === null || session.url === '') return
      setUrl(session.url)
      void generateQr(session.url).then(setQr)
    })
  }, [jobId])

  const totalBytes = 0 // sizes are not known until the backend stats them

  const onStart = useCallback(async () => {
    const parsed = validPort(port)
    if (!parsed.ok) {
      setPortError(parsed.error)
      return
    }
    setPortError(null)
    setUrl('')
    setQr(null)

    await start(async () => {
      const id = await startFileDrop({
        files: files.map((file) => file.path),
        port: parsed.port,
        allowUpload,
        receiveDir,
      })
      setJobId(id)
      return id
    })
  }, [allowUpload, files, port, receiveDir, start])

  const transfers = useMemo(() => results.slice().reverse(), [results])

  const summary = done?.summary ?? {}
  const num = (key: string) => (typeof summary[key] === 'number' ? (summary[key] as number) : 0)
  const note = typeof summary.note === 'string' ? summary.note : ''

  const columns = useMemo<Column<Transfer>[]>(
    () => [
      { key: 'time', header: 'Time', width: '8rem', value: (row) => transferTime(row) },
      {
        key: 'direction',
        header: 'Direction',
        width: '8rem',
        value: (row) => directionLabel(row.direction),
      },
      { key: 'name', header: 'File' },
      {
        key: 'bytes',
        header: 'Size',
        align: 'right',
        width: '8rem',
        render: (row) => formatBytes(row.bytes),
      },
      { key: 'peer', header: 'From', width: '10rem' },
      {
        key: 'status',
        header: 'Result',
        width: '8rem',
        value: (row) => statusLabel(row.status),
        render: (row) => (
          <span title={row.message}>
            <StatusDot
              status={row.status === 'ok' ? 'ok' : 'danger'}
              label={statusLabel(row.status)}
            />
          </span>
        ),
      },
    ],
    [],
  )

  const addressOptions = addresses.map((address) => ({
    value: address.ip,
    label: `${address.ip} (${address.adapter})`,
  }))

  return (
    <ToolShell
      title="LAN File Drop"
      description="Hand a file to another machine on the same network through its web browser, with no USB stick."
      help={
        <>
          <p>
            Pick the files you want to hand over and press Start sharing. CHIT puts them on a small
            web server on this machine and shows you a link. Type that link into the other machine's
            browser, or point its camera at the QR code, and the files are there to download. It
            works with anything that has a browser, including a phone or a machine with no CHIT on
            it at all.
          </p>
          <p className="mt-1.5">
            This is <strong>plain HTTP on your local network</strong>, with a random code in the
            link. Anyone on the same network who has the link can download those files, and nothing
            is encrypted in transit. That is fine for a printer driver or an installer, and it is
            not the right way to move a payroll spreadsheet. Press Stop sharing as soon as the file
            has arrived: the link stops working immediately.
          </p>
          <p className="mt-1.5">
            The first time you start it, Windows will ask whether to let CHIT accept connections.
            Say yes for private networks. If nothing happens on the other machine, check that both
            are on the same network (the same wifi, not one on guest wifi), and that the address in
            the link is the right one: a laptop on wifi and a cable at the same time has two, and
            the picker under Sharing options lets you choose.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="rounded border border-border bg-surface-2 px-3 py-2">
          <div className="flex flex-wrap items-center gap-2">
            <Button
              disabled={running}
              icon={<FilePlus size={14} aria-hidden />}
              onClick={() => {
                void pickDropFiles().then((chosen) => {
                  if (chosen === null || chosen.length === 0) return
                  setFiles(toPicked(chosen))
                })
              }}
            >
              Choose files
            </Button>
            <span className="text-sm text-fg-muted">{fileListLine(files, totalBytes)}</span>
          </div>

          {files.length > 0 && (
            <ul className="mt-2 flex flex-col gap-1">
              {files.map((file) => (
                <li key={file.path} className="flex items-center gap-2 text-sm">
                  <span className="min-w-0 flex-1 break-all text-fg">{file.name}</span>
                  {!running && (
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label={`Remove ${file.name}`}
                      icon={<X size={14} aria-hidden />}
                      onClick={() => setFiles((all) => all.filter((f) => f.path !== file.path))}
                    />
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>

        <details className="rounded border border-border bg-surface-2">
          <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
            Sharing options
          </summary>
          <div className="grid gap-3 px-3 pt-1 pb-3 sm:grid-cols-2">
            <TextInput
              label="Port"
              value={port}
              inputMode="numeric"
              onChange={(event) => setPort(event.target.value)}
              disabled={running}
              error={portError ?? undefined}
              hint="Change this only if something else is already using it."
            />
            {addressOptions.length > 1 && (
              <Select
                label="Address to show"
                options={addressOptions}
                value={chosenIp}
                onChange={(event) => setChosenIp(event.target.value)}
                disabled={running}
                hint="A laptop on wifi and a cable at once has two; only one reaches the other machine."
              />
            )}
            <div className="flex flex-col gap-2 text-sm">
              <label className="flex items-center gap-2 text-fg">
                <input
                  type="checkbox"
                  checked={allowUpload}
                  disabled={running}
                  onChange={(event) => setAllowUpload(event.target.checked)}
                  className="size-4 accent-[var(--accent)]"
                />
                Receive files too
              </label>
              {allowUpload && (
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    size="sm"
                    disabled={running}
                    icon={<FolderOpen size={14} aria-hidden />}
                    onClick={() => {
                      void pickDropFolder().then((chosen) => {
                        if (chosen === null || chosen === '') return
                        setReceiveDir(chosen)
                        toast.push('info', 'Files sent to you will land in ' + chosen)
                      })
                    }}
                  >
                    Choose receive folder
                  </Button>
                  <span className="text-xs break-all text-fg-muted">
                    {receiveDir === '' ? 'No folder chosen yet.' : receiveDir}
                  </span>
                </div>
              )}
            </div>
          </div>
        </details>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="primary"
            disabled={running || files.length === 0}
            onClick={() => void onStart()}
            icon={<Send size={14} aria-hidden />}
          >
            Start sharing
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Stop sharing
            </Button>
          )}
          {running && <StatusDot status="busy" label="Sharing" />}
        </div>

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {running && url !== '' && (
          <div className="flex flex-wrap items-center gap-4 rounded border border-accent bg-surface-2 px-3 py-3">
            <div className="min-w-56 flex-1">
              <p className="font-mono text-lg break-all text-fg">{url}</p>
              <div className="mt-2 flex items-center gap-2">
                <CopyButton value={url} label="Copy link" />
              </div>
              <p className="mt-2 text-xs text-fg-muted">
                Open this on the other machine's browser. It works on a phone too. Anyone on this
                network who has the link can download these files, so press Stop sharing when you
                are done.
              </p>
            </div>
            {qr !== null && <QrPanel code={qr} />}
          </div>
        )}

        {done !== null && (
          <div className="rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-sm font-medium text-fg">
              {summaryLine(
                {
                  downloads: num('downloads'),
                  uploads: num('uploads'),
                  bytesOut: num('bytesOut'),
                  bytesIn: num('bytesIn'),
                },
                done.durationMs,
              )}
              {done.cancelled && <span className="font-normal text-fg-muted"> (stopped early)</span>}
            </p>
            {note !== '' && <p className="mt-1 text-xs text-warn">{note}</p>}
          </div>
        )}

        <ResultsTable
          columns={columns}
          rows={transfers}
          getRowId={transferId}
          csvName="file-drop-log"
          emptyMessage={
            running
              ? 'Waiting for the other machine to open the link.'
              : 'Nothing has been transferred yet.'
          }
          rowStatus={(row) => (row.status === 'failed' ? 'danger' : undefined)}
        />
      </div>
    </ToolShell>
  )
}

function QrPanel({ code }: { code: QrCode }) {
  const span = code.size + code.quiet * 2
  return (
    <svg
      viewBox={`0 0 ${span} ${span}`}
      width={180}
      height={180}
      role="img"
      aria-label="QR code for the sharing link"
      className="shrink-0 rounded bg-white"
    >
      <path d={modulesToPath(code.modules, code.size, code.quiet)} fill="#000" />
    </svg>
  )
}
