import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Spinner,
  StatusDot,
  ToolShell,
  type Column,
} from '../../components'
import { errorMessage } from '../../lib/format'
import { usbDevices, type Device, type Report } from './api'
import { countLine, filterDevices, firstSeenLabel, kindLabel, sortDevices, vidPid, type Filter } from './devices'

export default function UsbHistoryPage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<Filter>('all')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await usbDevices())
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const history = report?.history === true
  const devices = useMemo(() => sortDevices(report?.devices ?? []), [report])
  const shown = useMemo(() => filterDevices(devices, filter), [devices, filter])

  // The First seen column is dropped rather than rendered empty on an OS that
  // keeps no history: a whole blank column reads as a bug.
  const columns = useMemo<Column<Device>[]>(() => {
    const base: Column<Device>[] = [
      { key: 'name', header: 'Device' },
      {
        key: 'manufacturer',
        header: 'Made by',
        width: '12rem',
        render: (row) =>
          row.manufacturer === '' ? (
            <span className="text-fg-muted italic">not reported</span>
          ) : (
            row.manufacturer
          ),
      },
      {
        key: 'ids',
        header: 'IDs',
        width: '9rem',
        value: (row) => vidPid(row),
        render: (row) => <span className="font-mono text-xs">{vidPid(row)}</span>,
      },
      { key: 'kind', header: 'Kind', width: '7rem', value: (row) => kindLabel(row.kind) },
      {
        key: 'serial',
        header: 'Serial',
        width: '14rem',
        render: (row) =>
          row.serial === '' ? (
            ''
          ) : (
            <span className="inline-flex items-center gap-1.5">
              <span className="font-mono text-xs">{row.serial}</span>
              <CopyButton value={row.serial} />
            </span>
          ),
      },
      {
        key: 'connected',
        header: 'Now',
        width: '7rem',
        value: (row) => (row.connected ? 'Connected' : 'Not now'),
        render: (row) => (
          <StatusDot
            status={row.connected ? 'ok' : 'idle'}
            label={row.connected ? 'Connected' : 'Not now'}
          />
        ),
      },
    ]
    if (history) {
      base.push({
        key: 'firstSeen',
        header: 'First seen',
        width: '12rem',
        value: (row) => row.firstSeen,
        render: (row) => firstSeenLabel(row),
      })
    }
    return base
  }, [history])

  const filters: Array<{ id: Filter; label: string }> = [
    { id: 'all', label: 'All' },
    { id: 'connected', label: 'Connected now' },
    { id: 'storage', label: 'Storage only' },
  ]
  if (history) filters.push({ id: 'seen', label: 'Seen before' })

  return (
    <ToolShell
      title="USB Device History"
      description="List the USB devices plugged in now, and on Windows the ones that were plugged in before."
      help={
        <>
          <p>
            This lists the USB devices attached to the computer right now, with the vendor and
            product ID Device Manager hides behind three clicks. Those two four-digit numbers, shown
            as <code>1d6b:0002</code>, are what you paste into a search when Windows says "Unknown
            device" and you need the right driver.
          </p>
          <p className="mt-1.5">
            On Windows the list also includes devices the computer remembers from before, which is
            mostly memory sticks and external drives, with the serial number and the date each was
            first connected. That record is good for storage and patchy for everything else, and it
            only says a device was connected, never that anybody copied anything. macOS and Linux
            keep no such record, so on those the list is only what is plugged in now.
          </p>
          <p className="mt-1.5">
            Nothing here can be changed or removed by CHIT, and nothing is ejected. If a device you
            have just plugged in is missing, press Refresh: this is a snapshot, not a live view.
          </p>
        </>
      }
      actions={
        <Button
          onClick={() => void load()}
          disabled={loading}
          icon={<RefreshCw size={14} aria-hidden className={loading ? 'animate-spin' : undefined} />}
        >
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {error === null && loading && report === null && (
          <div className="flex items-center gap-2 text-sm text-fg-muted">
            <Spinner /> Reading USB devices
          </div>
        )}

        {report !== null && (
          <>
            <div>
              <p className="text-sm text-fg">{countLine(devices, history)}</p>
              <p className="mt-1 text-xs text-fg-muted">{report.note}</p>
            </div>

            <div className="flex flex-wrap gap-1">
              {filters.map((option) => (
                <Button
                  key={option.id}
                  size="sm"
                  variant={filter === option.id ? 'primary' : 'ghost'}
                  aria-pressed={filter === option.id}
                  onClick={() => setFilter(option.id)}
                >
                  {option.label}
                </Button>
              ))}
            </div>

            <ResultsTable
              columns={columns}
              rows={shown}
              getRowId={(row) =>
                `${row.source}|${row.vendorId}|${row.productId}|${row.serial}|${row.name}`
              }
              csvName="usb-devices"
              emptyMessage="No USB devices were found. On a virtual machine that is normal."
              rowStatus={(row) => (row.connected ? 'ok' : undefined)}
            />
          </>
        )}
      </div>
    </ToolShell>
  )
}
