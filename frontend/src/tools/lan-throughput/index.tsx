import { useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowDownUp, QrCode as QrCodeIcon, Square } from 'lucide-react'
import {
  Button,
  CopyButton,
  ProgressBar,
  ResultsTable,
  Select,
  StatusDot,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { formatBytes } from '../../lib/format'
import { useJob } from '../../lib/useJob'
// The Wi-Fi QR tool's pure renderer, imported rather than copied so there is
// only one QR path builder in the app. Approved in Phase 4 for label-maker and
// used again by lan-file-drop. This file is never edited.
import { modulesToPath } from '../wifi-qr/render'
import {
  generateQr,
  lanSpeedSession,
  speedAddresses,
  startLanSpeed,
  type Address,
  type Pull,
  type QrCode,
} from './api'
import {
  curlFor,
  linkForAddress,
  localTime,
  mbpsText,
  pullId,
  secondsText,
  statusLabel,
  summaryLine,
  validPort,
} from './reading'

const SIZES = [
  { value: '50', label: '50 MB' },
  { value: '200', label: '200 MB' },
  { value: '500', label: '500 MB' },
  { value: '1000', label: '1000 MB' },
]

export default function LanThroughputPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Pull>()

  const [port, setPort] = useState('8740')
  const [portError, setPortError] = useState<string | null>(null)
  const [sizeMb, setSizeMb] = useState('200')
  const [addresses, setAddresses] = useState<Address[]>([])
  const [chosenIp, setChosenIp] = useState('')
  const [jobId, setJobId] = useState<string | null>(null)
  const [url, setUrl] = useState('')
  const [showQr, setShowQr] = useState(false)
  const [qr, setQr] = useState<QrCode | null>(null)

  useEffect(() => {
    void speedAddresses().then((found) => {
      setAddresses(found)
      if (found.length > 0) setChosenIp((current) => (current === '' ? found[0].ip : current))
    })
  }, [])

  // useJob.start does not hand the job id back, and the page needs it to ask for
  // the link. The starter callback is the one place it is visible, so it is
  // captured there. Reported as a gap in useJob (LAN File Drop does the same).
  useEffect(() => {
    if (jobId === null) return
    void lanSpeedSession(jobId).then((session) => {
      if (session === null || session.url === '') return
      setUrl(session.url)
    })
  }, [jobId])

  const onStart = useCallback(async () => {
    const parsed = validPort(port)
    if (!parsed.ok) {
      setPortError(parsed.error)
      return
    }
    setPortError(null)
    setUrl('')
    await start(async () => {
      const id = await startLanSpeed({ port: parsed.port, sizeMb: Number(sizeMb) })
      setJobId(id)
      return id
    })
  }, [port, sizeMb, start])

  const pulls = useMemo(() => results.slice().reverse(), [results])

  const summary = done?.summary ?? {}
  const num = (key: string) => (typeof summary[key] === 'number' ? (summary[key] as number) : 0)
  const note = typeof summary.note === 'string' ? summary.note : ''

  const columns = useMemo<Column<Pull>[]>(
    () => [
      { key: 'time', header: 'Time', width: '7rem', value: (row) => localTime(row.time) },
      { key: 'peer', header: 'Machine', width: '12rem' },
      {
        key: 'mbps',
        header: 'Speed',
        align: 'right',
        width: '8rem',
        render: (row) => mbpsText(row.mbps),
      },
      { key: 'reading', header: 'Reading' },
      {
        key: 'bytes',
        header: 'Transferred',
        align: 'right',
        width: '8rem',
        render: (row) => formatBytes(row.bytes),
      },
      {
        key: 'seconds',
        header: 'Took',
        align: 'right',
        width: '6rem',
        render: (row) => secondsText(row.seconds),
      },
      {
        key: 'status',
        header: 'Result',
        width: '9rem',
        value: (row) => statusLabel(row.status),
        render: (row) => (
          <StatusDot status={row.status === 'ok' ? 'ok' : 'warn'} label={statusLabel(row.status)} />
        ),
      },
    ],
    [],
  )

  const addressOptions = addresses.map((address) => ({
    value: address.ip,
    label: `${address.ip} (${address.adapter})`,
  }))

  // The backend builds the link from the address it decided to offer first. When
  // the tech picks a different one, only the host part changes.
  const shownUrl = useMemo(() => linkForAddress(url, chosenIp), [url, chosenIp])

  // Nothing is generated until the tech asks for the code, and the payload is
  // the link on screen rather than the one the backend built, so picking a
  // different address regenerates it. The guard drops a slow answer for an
  // address that has already been changed again.
  useEffect(() => {
    if (!showQr || shownUrl === '') {
      setQr(null)
      return
    }
    let current = true
    void generateQr(shownUrl).then((code) => {
      if (current) setQr(code)
    })
    return () => {
      current = false
    }
  }, [showQr, shownUrl])

  return (
    <ToolShell
      title="LAN Throughput Test"
      description="Measure the real transfer speed between this machine and another one on the same network."
      help={
        <>
          <p>
            The Speed Test in CHIT measures your internet connection. This one measures the link
            between two machines on your own network, which is a different question and usually the
            one behind "copying from the server is slow". Press Start here, then open the address it
            gives you on the other machine and click the link on that page.
          </p>
          <p className="mt-1.5">
            The number is megabits per second in one direction: from this computer to the other one.
            Roughly 940 Mbps is a healthy gigabit link. Anything hovering near 94 Mbps almost always
            means a port negotiated at 100 Mbps, which is a damaged cable, a bad patch panel port, or
            an old switch. Anything below that on a wired link is worth chasing; on Wi-Fi it is
            normal.
          </p>
          <p className="mt-1.5">
            These figures are approximate. The other machine's disk, its browser, its antivirus and
            whatever else is on the wire all affect them, so run it twice before drawing a
            conclusion. Nothing is written to disk on either machine: the data is generated as it is
            sent and the other machine throws it away.
          </p>
          <p className="mt-1.5">
            The port is open to the whole network while the test is running. Press Stop when you are
            done.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (!running) void onStart()
          }}
        >
          <div className="w-40">
            <TextInput
              label="Port"
              value={port}
              inputMode="numeric"
              spellCheck={false}
              autoComplete="off"
              disabled={running}
              onChange={(event) => setPort(event.target.value)}
              error={portError ?? undefined}
              hint="1024 to 65535."
            />
          </div>
          <div className="w-44">
            <Select
              label="Test size"
              options={SIZES}
              value={sizeMb}
              disabled={running}
              onChange={(event) => setSizeMb(event.target.value)}
              hint="Bigger is more accurate."
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={running}
            icon={<ArrowDownUp size={14} aria-hidden />}
          >
            Start
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Stop
            </Button>
          )}
        </form>

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {running && shownUrl !== '' && (
          <div className="rounded border border-accent bg-surface-2 px-3 py-3">
            <p className="text-sm font-medium text-fg">Open this on the other machine</p>
            {addressOptions.length > 1 && (
              <div className="mt-2 max-w-xs">
                <Select
                  label="Address to use"
                  options={addressOptions}
                  value={chosenIp}
                  onChange={(event) => setChosenIp(event.target.value)}
                  hint="A laptop on wifi and a cable at once has two."
                />
              </div>
            )}
            <p className="mt-2 font-mono text-sm break-all text-fg">{shownUrl}</p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <CopyButton value={shownUrl} label="Copy link" />
              <CopyButton value={curlFor(shownUrl)} label="Copy curl line" />
              <Button
                size="sm"
                onClick={() => setShowQr((shown) => !shown)}
                icon={<QrCodeIcon size={14} aria-hidden />}
              >
                {showQr ? 'Hide QR code' : 'Show QR code'}
              </Button>
            </div>
            {showQr && qr !== null && (
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <QrPanel code={qr} />
                <p className="max-w-xs text-xs text-fg-muted">
                  Scanning this with a phone measures the phone's wifi, not the link you are testing.
                  Use it to get the address onto the other computer without typing it.
                </p>
              </div>
            )}
            <p className="mt-2 text-xs text-fg-muted">
              Open the link, then click Start the test. The result appears here, not there.
            </p>
          </div>
        )}

        {running && (
          <>
            <ProgressBar
              value={progress.done}
              max={progress.total}
              label={progress.message === '' ? 'Opening the port' : progress.message}
            />
            <p className="flex items-center gap-2 text-sm text-warn">
              <StatusDot status="busy" />
              This port is open to the whole network while the test is running. Press Stop when you
              are done.
            </p>
          </>
        )}

        {done !== null && (
          <div className="rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-sm font-medium text-fg">
              {summaryLine(num('pulls'), num('bestMbps'), num('bytesOut'), done.durationMs)}
              {done.cancelled && <span className="font-normal text-fg-muted"> (stopped early)</span>}
            </p>
            {note !== '' && <p className="mt-1 text-xs text-warn">{note}</p>}
          </div>
        )}

        <ResultsTable
          columns={columns}
          rows={pulls}
          getRowId={pullId}
          csvName="lan-throughput"
          emptyMessage={
            running
              ? 'Waiting for the other machine to pull the test file.'
              : 'Press Start, then open the link it gives you on the other machine.'
          }
          rowStatus={(row) => (row.status === 'ok' ? 'ok' : 'warn')}
        />

        <p className="text-xs text-fg-muted">
          Every figure here is approximate: it is what this computer could push down the link while
          it was also doing everything else.
        </p>
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
      aria-label="QR code for the throughput test link"
      className="shrink-0 rounded bg-white"
    >
      <path d={modulesToPath(code.modules, code.size, code.quiet)} fill="#000" />
    </svg>
  )
}
