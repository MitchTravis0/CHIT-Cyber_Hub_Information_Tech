import type { ReactNode } from 'react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Spinner,
  ToolShell,
  type Column,
} from '../../components'
import { errorMessage, formatBytes } from '../../lib/format'
import { systemInfo, type Disk, type Report } from './api'
import { fieldText, formatUptime, NOT_AVAILABLE, reportText } from './report'

export default function SystemInfoPage() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [readAt, setReadAt] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await systemInfo())
      setReadAt(new Date().toLocaleTimeString())
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

  const columns = useMemo<Column<Disk>[]>(
    () => [
      { key: 'mount', header: 'Drive', width: '10rem' },
      { key: 'fs', header: 'Type', width: '8rem' },
      {
        key: 'total',
        header: 'Size',
        align: 'right',
        width: '7rem',
        render: (row) => formatBytes(row.total),
      },
      {
        key: 'used',
        header: 'Used',
        align: 'right',
        width: '7rem',
        render: (row) => formatBytes(row.used),
      },
      {
        key: 'free',
        header: 'Free',
        align: 'right',
        width: '7rem',
        render: (row) => formatBytes(row.free),
      },
      {
        key: 'usedPct',
        header: 'Full',
        align: 'right',
        width: '6rem',
        render: (row) => `${row.usedPct}%`,
      },
    ],
    [],
  )

  const unsupported = report?.unsupported ?? []
  const disks = report?.disks ?? []
  const field = (value: string, id: string) => fieldText(value, id, unsupported)

  return (
    <ToolShell
      title="System Info Snapshot"
      description="Read this machine's OS, CPU, memory, disks, uptime and serial number in one place."
      help={
        <>
          <p>
            This is a snapshot of the computer CHIT itself is running on, which is the one you are
            standing at. Nothing here is read from the network and nothing is changed. Press Refresh
            after plugging a drive in or after a reboot.
          </p>
          <p className="mt-1.5">
            Some of these facts are only readable by an administrator, and CHIT never asks for
            admin rights. Anything this operating system would not hand over says "not available on
            this OS" rather than showing a blank or a guess. On Linux the machine serial number is
            one of those: it lives in a file only root can read.
          </p>
          <p className="mt-1.5">
            A drive at 90% is tinted amber and at 95% red. That is usually the real answer to "the
            computer is slow", because Windows needs free space to page and to install updates.
            Press Copy report to paste the whole snapshot into a ticket.
          </p>
        </>
      }
      actions={
        <div className="flex items-center gap-2">
          {report !== null && <CopyButton value={reportText(report)} label="Copy report" />}
          <Button
            onClick={() => void load()}
            disabled={loading}
            icon={<RefreshCw size={14} aria-hidden className={loading ? 'animate-spin' : undefined} />}
          >
            Refresh
          </Button>
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        {error !== null && (
          <p role="alert" className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg">
            {error}
          </p>
        )}

        {error === null && loading && report === null && (
          <div className="flex items-center gap-2 text-sm text-fg-muted">
            <Spinner /> Reading this machine
          </div>
        )}

        {report !== null && (
          <>
            <Card title="This machine">
              <Row label="Computer name" value={field(report.hostname, 'hostname')} />
              <Row label="Signed in as" value={field(report.user, 'user')} />
              <Row label="Operating system" value={field(report.osName, 'osName')} />
              <Row label="Version" value={field(report.osVersion, 'osVersion')} />
              <Row label="Architecture" value={field(report.arch, 'arch')} />
              <Row
                label="Up for"
                value={
                  unsupported.includes('uptime') ? NOT_AVAILABLE : formatUptime(report.uptimeS)
                }
              />
              <Row label="Started" value={field(report.bootTime, 'bootTime')} />
            </Card>

            <Card title="Hardware">
              <Row label="Manufacturer" value={field(report.manufacturer, 'manufacturer')} />
              <Row label="Model" value={field(report.model, 'model')} />
              <Row
                label="Serial number"
                value={field(report.serial, 'serial')}
                extra={report.serial !== '' && <CopyButton value={report.serial} />}
              />
              <Row label="Processor" value={field(report.cpuModel, 'cpuModel')} />
              <Row label="Processor cores" value={String(report.cpuCores)} />
              <Row
                label="Memory fitted"
                value={report.memoryTotal > 0 ? formatBytes(report.memoryTotal) : 'not reported'}
              />
              <Row
                label="Memory free"
                value={
                  unsupported.includes('memoryFree')
                    ? NOT_AVAILABLE
                    : formatBytes(report.memoryFree)
                }
              />
            </Card>

            <ResultsTable
              columns={columns}
              rows={disks}
              getRowId={(row) => row.mount}
              csvName="system-disks"
              emptyMessage="This operating system did not report any drives."
              rowStatus={(row) =>
                row.usedPct >= 95 ? 'danger' : row.usedPct >= 90 ? 'warn' : undefined
              }
            />

            <p className="text-xs text-fg-muted">
              Read at {readAt}. CHIT {report.appVersion} on {report.os}/{report.arch}.
            </p>
          </>
        )}
      </div>
    </ToolShell>
  )
}

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded border border-border bg-surface-2 px-3 py-2">
      <h2 className="mb-1.5 text-xs font-semibold tracking-wide text-fg-muted uppercase">
        {title}
      </h2>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">{children}</dl>
    </div>
  )
}

function Row({ label, value, extra }: { label: string; value: string; extra?: ReactNode }) {
  const missing = value === NOT_AVAILABLE || value === 'not reported'
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className={missing ? 'text-fg-muted italic' : 'text-fg'}>
        <span className="inline-flex items-center gap-2">
          {value}
          {extra}
        </span>
      </dd>
    </>
  )
}
