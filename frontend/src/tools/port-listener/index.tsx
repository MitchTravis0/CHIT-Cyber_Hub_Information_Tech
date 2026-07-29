import { useCallback, useEffect, useMemo, useState } from 'react'
import { Ear, Square } from 'lucide-react'
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
import { formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { listenerAddresses, startPortListener, type Address, type Hit } from './api'
import { arrivalLine, commandsFor, hitId, localTime, validPort } from './commands'

const PROTOCOLS = [
  { value: 'tcp', label: 'TCP' },
  { value: 'udp', label: 'UDP' },
  { value: 'both', label: 'TCP and UDP' },
]

export default function PortListenerPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Hit>()

  const [port, setPort] = useState('8730')
  const [portError, setPortError] = useState<string | null>(null)
  const [protocol, setProtocol] = useState('tcp')
  const [addresses, setAddresses] = useState<Address[]>([])
  const [chosenIp, setChosenIp] = useState('')

  useEffect(() => {
    void listenerAddresses().then((found) => {
      setAddresses(found)
      if (found.length > 0) setChosenIp((current) => (current === '' ? found[0].ip : current))
    })
  }, [])

  const onStart = useCallback(async () => {
    const parsed = validPort(port)
    if (!parsed.ok) {
      setPortError(parsed.error)
      return
    }
    setPortError(null)
    await start(() => startPortListener({ port: parsed.port, protocol }))
  }, [port, protocol, start])

  const hits = useMemo(() => results.slice().reverse(), [results])

  const summary = done?.summary ?? {}
  const num = (key: string) => (typeof summary[key] === 'number' ? (summary[key] as number) : 0)
  const note = typeof summary.note === 'string' ? summary.note : ''

  const columns = useMemo<Column<Hit>[]>(
    () => [
      { key: 'time', header: 'Time', width: '7rem', value: (row) => localTime(row.time) },
      {
        key: 'protocol',
        header: 'Protocol',
        width: '6rem',
        value: (row) => (row.protocol === 'udp' ? 'UDP' : 'TCP'),
      },
      { key: 'peer', header: 'From', width: '12rem' },
      { key: 'peerPort', header: 'Their port', align: 'right', width: '7rem' },
      { key: 'bytes', header: 'Sent', align: 'right', width: '6rem' },
      {
        key: 'preview',
        header: 'What arrived',
        render: (row) =>
          row.preview === '' ? (
            <span className="text-fg-muted">nothing, it just connected</span>
          ) : (
            <span className="font-mono text-xs break-all">{row.preview}</span>
          ),
      },
    ],
    [],
  )

  const addressOptions = addresses.map((address) => ({
    value: address.ip,
    label: `${address.ip} (${address.adapter})`,
  }))

  // An empty or unreadable field means the backend will use its own default, so
  // the commands shown must name that same default rather than nothing.
  const parsedPort = validPort(port)
  const listeningPort = parsedPort.ok && parsedPort.port !== 0 ? parsedPort.port : 8730
  const commands = chosenIp === '' ? [] : commandsFor(chosenIp, listeningPort, protocol)

  return (
    <ToolShell
      title="Port Listener"
      description="Open a port on this machine so a test from the other side proves the path through the firewall."
      help={
        <>
          <p>
            Every other network tool in CHIT tests <strong>towards</strong> something. This one
            makes CHIT the thing that answers, which is the only way to prove a firewall rule
            before the service it is for has been installed. Pick the port the real service will
            use, press Listen, and then run the command shown on the other machine.
          </p>
          <p className="mt-1.5">
            A row appearing means the traffic got all the way here. Nothing appearing means
            something in between dropped it: this computer's own firewall (Windows asks the first
            time CHIT listens), a firewall or ACL between the two machines, or a missing NAT rule.
            Ports below 1024 are not offered because binding one needs administrator rights, which
            CHIT never asks for.
          </p>
          <p className="mt-1.5">
            TCP and UDP answer different questions. On TCP a connection either completes or it does
            not, so an empty list is real evidence. UDP has no handshake and nothing acknowledges a
            blocked datagram, so on UDP an empty list only means nothing arrived, never that the
            port is closed.
          </p>
          <p className="mt-1.5">
            The port is open to the whole network for as long as you are listening. Press Stop when
            you are done.
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
          <div className="w-48">
            <Select
              label="Protocol"
              options={PROTOCOLS}
              value={protocol}
              disabled={running}
              onChange={(event) => setProtocol(event.target.value)}
            />
          </div>
          <Button type="submit" variant="primary" disabled={running} icon={<Ear size={14} aria-hidden />}>
            Listen
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

        {running && (
          <>
            <ProgressBar
              value={progress.done}
              max={progress.total}
              label={progress.message === '' ? 'Opening the port' : progress.message}
            />

            <div className="rounded border border-accent bg-surface-2 px-3 py-3">
              <p className="text-sm font-medium text-fg">Run this on the other machine</p>
              {addressOptions.length === 0 ? (
                <p className="mt-2 text-xs text-fg-muted">
                  This computer has no network address other than its own loopback, so only a test
                  from this same machine can reach the port.
                </p>
              ) : (
                <>
                  {addressOptions.length > 1 ? (
                    <div className="mt-2 max-w-xs">
                      <Select
                        label="This machine is"
                        options={addressOptions}
                        value={chosenIp}
                        onChange={(event) => setChosenIp(event.target.value)}
                        hint="A laptop on wifi and a cable at once has two."
                      />
                    </div>
                  ) : (
                    <p className="mt-1 text-xs text-fg-muted">
                      This machine is {addresses[0]?.ip} ({addresses[0]?.adapter})
                    </p>
                  )}
                  <ul className="mt-2 flex flex-col gap-2">
                    {commands.map((line) => (
                      <li key={line.label} className="flex flex-wrap items-center gap-2">
                        <span className="w-40 shrink-0 text-xs text-fg-muted">{line.label}</span>
                        <code className="min-w-0 flex-1 font-mono text-xs break-all text-fg">
                          {line.command}
                        </code>
                        <CopyButton value={line.command} />
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </div>

            <p className="flex items-center gap-2 text-sm text-warn">
              <StatusDot status="busy" />
              This port is open to the whole network while you are listening. Press Stop when the
              test is done.
            </p>

            <p className="text-xs text-fg-muted">{arrivalLine(results)}</p>
          </>
        )}

        {done !== null && (
          <div className="rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-sm font-medium text-fg">
              {arrivalLine(results)} in {formatDuration(done.durationMs)}
              {done.cancelled && <span className="font-normal text-fg-muted"> (stopped early)</span>}
            </p>
            <p className="mt-1 text-xs text-fg-muted">
              {num('tcp')} over TCP, {num('udp')} over UDP.
            </p>
            {note !== '' && <p className="mt-1 text-xs text-warn">{note}</p>}
          </div>
        )}

        <ResultsTable
          columns={columns}
          rows={hits}
          getRowId={hitId}
          csvName={`port-${listeningPort}-arrivals`}
          emptyMessage={
            running
              ? 'Listening. Rows appear the moment something connects.'
              : 'Choose a port and press Listen. Then test it from the other machine.'
          }
        />
      </div>
    </ToolShell>
  )
}
