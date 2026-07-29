import type { FormEvent, ReactNode } from 'react'
import { useState } from 'react'
import { CircleCheck, Printer, Search, ShieldAlert, TriangleAlert } from 'lucide-react'
import {
  Button,
  CopyButton,
  Select,
  Spinner,
  TextInput,
  ToolShell,
} from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { queryPrinter, sendPrinterTestPage, type PrinterResult } from './api'
import { onlineLabel, parsePort } from './format'

const HELP = (
  <>
    <p>
      This talks straight to a network printer's raw port, the one printers call JetDirect or "raw
      9100". No driver, no queue and no print server are involved, which is exactly what makes it
      useful: if a page comes out of here, the printer and the network are both fine and the fault
      is further back in the print server or the driver.
    </p>
    <p className="mt-2">
      <strong>Ask the printer</strong> opens a connection and asks the printer what it is. It cannot
      make paper move. <strong>Print a test page</strong> sends a few lines of text and a form feed,
      and the printer will produce one sheet. Use the first one first: if the printer will not even
      answer, there is no point sending it a page.
    </p>
    <p className="mt-2">
      Port 9100 is the standard. Some printers offer 9101 and 9102 as a second and third port. If
      the connection is refused, raw printing is often turned off in the printer's web page under a
      name like "Port 9100 printing" or "Raw print", and some models only enable it when a driver
      has been installed once.
    </p>
    <p className="mt-2">
      A status code with no explanation next to it is normal: printers report a number and their own
      display text, and the numbers differ between manufacturers. CHIT shows you exactly what the
      printer said rather than guessing at a meaning. Look the code up in that model's manual. A
      printer that answers with nothing at all is also normal: plenty of cheap printers accept the
      connection and speak no PJL, and they will still print the test page.
    </p>
  </>
)

const TIMEOUTS = [
  { value: '2000', label: '2 seconds' },
  { value: '5000', label: '5 seconds' },
  { value: '15000', label: '15 seconds' },
]

function bannerTone(level: string): { box: string; icon: string; Icon: typeof CircleCheck } {
  if (level === 'ok') return { box: 'border-ok bg-ok/10', icon: 'text-ok', Icon: CircleCheck }
  if (level === 'warn')
    return { box: 'border-warn bg-warn/10', icon: 'text-warn', Icon: TriangleAlert }
  return { box: 'border-danger bg-danger/10', icon: 'text-danger', Icon: ShieldAlert }
}

function localTime(stamp: string): string {
  if (stamp === '') return ''
  const when = new Date(stamp)
  return Number.isNaN(when.getTime()) ? stamp : when.toLocaleTimeString()
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-xs text-fg-muted">{label}</dt>
      <dd className="min-w-0 text-fg">{value}</dd>
    </div>
  )
}

export default function PrinterTestPage() {
  const [host, setHost] = useState('')
  const [hostError, setHostError] = useState<string | null>(null)
  const [port, setPort] = useState('9100')
  const [portError, setPortError] = useState<string | null>(null)
  const [timeoutMs, setTimeoutMs] = useState('5000')
  const [result, setResult] = useState<PrinterResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [running, setRunning] = useState(false)

  const run = async (send: boolean) => {
    if (running) return
    const text = host.trim()
    if (text === '') {
      setHostError("Type the printer's address, for example 192.168.1.50.")
      return
    }
    setHostError(null)
    const parsed = parsePort(port)
    if (!parsed.ok) {
      setPortError(parsed.error)
      return
    }
    setPortError(null)
    setRunning(true)
    try {
      const params = { host: text, port: parsed.port, timeoutMs: Number(timeoutMs) }
      setResult(send ? await sendPrinterTestPage(params) : await queryPrinter(params))
      setError(null)
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    } finally {
      setRunning(false)
    }
  }

  // Enter always runs the safe action. A button that produces paper must not be
  // reachable by pressing a key.
  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    void run(false)
  }

  const tone = bannerTone(result?.level ?? 'ok')
  const online = result === null ? null : onlineLabel(result.online)
  const disabled = running || host.trim() === ''

  return (
    <ToolShell
      title="Raw Printer Test"
      description="Send a plain text page straight to a network printer's port 9100, with no driver installed."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <p className="flex items-start gap-2 rounded border border-warn bg-warn/10 px-3 py-2 text-sm text-fg">
          <TriangleAlert size={16} className="mt-0.5 shrink-0 text-warn" aria-hidden />
          <span>
            The <strong>Print a test page</strong> button makes the printer produce a sheet of
            paper. <strong>Ask the printer</strong> does not.
          </span>
        </p>

        <form className="flex flex-wrap items-end gap-2" onSubmit={onSubmit}>
          <div className="min-w-56 flex-1">
            <TextInput
              label="Printer address"
              value={host}
              onChange={(event) => setHost(event.target.value)}
              placeholder="192.168.1.50"
              spellCheck={false}
              autoComplete="off"
              error={hostError ?? undefined}
              hint="The printer's IP address or name."
            />
          </div>
          <div className="w-28">
            <TextInput
              label="Port"
              value={port}
              onChange={(event) => setPort(event.target.value)}
              inputMode="numeric"
              spellCheck={false}
              autoComplete="off"
              error={portError ?? undefined}
              hint="9100 unless you know otherwise."
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={disabled}
            icon={<Search size={14} aria-hidden />}
          >
            Ask the printer
          </Button>
          <Button
            type="button"
            variant="danger"
            disabled={disabled}
            onClick={() => void run(true)}
            icon={<Printer size={14} aria-hidden />}
          >
            Print a test page
          </Button>
        </form>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Connection options</summary>
          <div className="mt-3 max-w-56">
            <Select
              label="Wait for the printer"
              options={TIMEOUTS}
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </div>
        </details>

        {running && <Spinner label="Talking to the printer" />}

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {result !== null && (
          <>
            <div className={cn('flex items-start gap-2 rounded border px-3 py-2', tone.box)}>
              <tone.Icon size={16} className={cn('mt-0.5 shrink-0', tone.icon)} aria-hidden />
              <div className="min-w-0 flex-1">
                <p className="text-sm text-fg">{result.headline}</p>
                {result.advice !== '' && (
                  <p className="mt-1 text-sm text-fg-muted">{result.advice}</p>
                )}
              </div>
            </div>

            <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-1 rounded border border-border bg-surface-2 p-3 text-sm">
              {result.address !== '' && <Row label="Connected to" value={result.address} />}
              {result.connected && (
                <Row label="Took" value={`${Math.round(result.connectMs)} ms`} />
              )}
              {result.model !== '' && <Row label="Model" value={result.model} />}
              {result.statusCode !== '' && <Row label="Status code" value={result.statusCode} />}
              {result.display !== '' && <Row label="Display says" value={result.display} />}
              {online !== null && <Row label="Online" value={online} />}
              {result.printed && <Row label="Sent" value={`${result.bytesSent} bytes`} />}
            </dl>

            {result.reply !== '' && (
              <details className="rounded border border-border bg-surface-2 px-3 py-2">
                <summary className="cursor-pointer text-sm text-fg">
                  What the printer sent back
                </summary>
                <div className="mt-2 flex items-start gap-2">
                  <pre className="min-w-0 flex-1 overflow-x-auto text-xs text-fg">
                    {result.reply}
                  </pre>
                  <CopyButton value={result.reply} />
                </div>
              </details>
            )}

            <p className="text-sm text-fg-muted">Checked at {localTime(result.checkedAt)}.</p>
          </>
        )}

        {result === null && error === null && !running && (
          <p className="text-sm text-fg-muted">
            Type the printer's address and press Ask the printer. Nothing is printed by that button.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
