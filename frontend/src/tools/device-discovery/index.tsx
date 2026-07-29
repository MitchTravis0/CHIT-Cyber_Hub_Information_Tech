import { useMemo, useState } from 'react'
import { RadioTower, Square } from 'lucide-react'
import {
  Button,
  ProgressBar,
  ResultsTable,
  Select,
  ToolShell,
  type Column,
} from '../../components'
import { formatDuration } from '../../lib/format'
import { useJob } from '../../lib/useJob'
import { startDeviceDiscovery, type Device } from './api'
import { detailsCell, friendlyService, mergeDevices } from './services'

const HELP = (
  <>
    <p>
      Printers, TVs, NAS boxes, cameras and casting devices announce themselves on the network
      several times a minute using two protocols: mDNS (which Apple calls Bonjour) and SSDP (part of
      UPnP). This tool asks both and lists what answers. Nothing is connected to and nothing is
      changed: it only listens to what devices already volunteer.
    </p>
    <p className="mt-2">
      Press Listen and wait. Four seconds catches most things; ten catches the slow ones. The same
      device often appears more than once, once per service it offers, which is normal: a printer
      that answers both "who prints?" and "who has a web page?" is telling you two true things.
    </p>
    <p className="mt-2">
      <strong>Silence is not absence.</strong> A device that advertises nothing will never appear
      here, plenty of business kit has this turned off, and multicast is very commonly blocked
      between VLANs and on guest Wi-Fi. If this list is empty and the IP Range Scanner shows
      occupied addresses, the network is blocking the announcements, not missing the devices.
    </p>
    <p className="mt-2">
      This tells you what a device claims to be. The IP Range Scanner tells you which addresses are
      in use and, from the ARP cache, the manufacturer behind each one. The two together are how you
      turn an unlabelled network into a list you can write down.
    </p>
  </>
)

const WINDOWS = [
  { value: '2000', label: '2 seconds' },
  { value: '4000', label: '4 seconds' },
  { value: '10000', label: '10 seconds' },
  { value: '30000', label: '30 seconds' },
]

export default function DeviceDiscoveryPage() {
  const { running, progress, results, error, done, start, cancel } = useJob<Device>()
  const [timeoutMs, setTimeoutMs] = useState('4000')

  // The backend re-emits a device when a later reply carries more than the
  // first one did, so the UI folds by key with last write winning.
  const rows = useMemo(() => mergeDevices(results), [results])

  const columns = useMemo<Column<Device>[]>(
    () => [
      { key: 'ip', header: 'Address', width: '10rem' },
      {
        key: 'name',
        header: 'Name',
        render: (row) =>
          row.name === '' ? <span className="text-fg-muted">(no name given)</span> : row.name,
      },
      {
        key: 'service',
        header: 'What it says it is',
        width: '14rem',
        value: (row) => friendlyService(row.service),
      },
      {
        key: 'port',
        header: 'Port',
        align: 'right',
        width: '6rem',
        value: (row) => (row.port === 0 ? null : row.port),
      },
      { key: 'protocol', header: 'Heard by', width: '7rem' },
      { key: 'details', header: 'Details', value: (row) => detailsCell(row) },
      { key: 'adapter', header: 'Adapter', width: '8rem' },
    ],
    [],
  )

  const note = typeof done?.summary.note === 'string' ? done.summary.note : ''
  const adapters = typeof done?.summary.adapters === 'number' ? done.summary.adapters : 0

  const onListen = async () => {
    await start(() => startDeviceDiscovery({ timeoutMs: Number(timeoutMs) }))
  }

  return (
    <ToolShell
      title="Device Discovery"
      description="Listen for printers, TVs, NAS boxes and casting devices announcing themselves on the network."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <Button
            variant="primary"
            disabled={running}
            onClick={() => void onListen()}
            icon={<RadioTower size={14} aria-hidden />}
          >
            Listen
          </Button>
          {running && (
            <Button variant="danger" onClick={cancel} icon={<Square size={14} aria-hidden />}>
              Cancel
            </Button>
          )}
        </div>

        <details className="rounded border border-border bg-surface-2 px-3 py-2">
          <summary className="cursor-pointer text-sm text-fg">Listening options</summary>
          <div className="mt-3 max-w-56">
            <Select
              label="Listen for"
              options={WINDOWS}
              value={timeoutMs}
              onChange={(event) => setTimeoutMs(event.target.value)}
            />
          </div>
        </details>

        {running && (
          <ProgressBar
            value={progress.done}
            max={0}
            label={progress.message === '' ? 'Sending the questions' : progress.message}
          />
        )}

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {(rows.length > 0 || done !== null) && (
          <ResultsTable
            columns={columns}
            rows={rows}
            getRowId={(row) => row.key}
            csvName="devices"
            emptyMessage="Nothing announced itself. That does not mean the network is empty: see the note below."
          />
        )}

        {done !== null && (
          <>
            <p className="text-sm text-fg">
              Heard {rows.length} {rows.length === 1 ? 'device' : 'devices'} on {adapters}{' '}
              {adapters === 1 ? 'adapter' : 'adapters'} in {formatDuration(done.durationMs)}
              {done.cancelled && <span className="text-fg-muted"> (stopped early)</span>}
            </p>
            {note !== '' && <p className="text-xs text-warn">{note}</p>}
          </>
        )}

        {rows.length === 0 && done === null && !running && error === null && (
          <p className="text-sm text-fg-muted">
            Press Listen. Devices that announce themselves will appear as they are heard.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
