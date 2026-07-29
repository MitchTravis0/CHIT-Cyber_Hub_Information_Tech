import { useCallback, useEffect, useState } from 'react'
import { ChevronRight, RefreshCw } from 'lucide-react'
import { Button, Spinner, ToolShell } from '../../components'
import { errorMessage } from '../../lib/format'
import { AdapterCard } from './AdapterCard'
import { listAdapters, type Adapter, type AdapterReport } from './api'

/** An adapter is worth showing up front when it is the primary one or a
 *  connected physical adapter. Everything else is a laptop's usual clutter. */
function isInteresting(a: Adapter): boolean {
  return a.primary || (a.up && !a.virtual && !a.loopback)
}

export default function AdapterInfoPage() {
  const [report, setReport] = useState<AdapterReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setReport(await listAdapters())
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

  const adapters = report?.adapters ?? []
  const shown = adapters.filter(isInteresting)
  const hidden = adapters.filter((a) => !isInteresting(a))
  const searchDomains = report?.searchDomains ?? []

  return (
    <ToolShell
      title="Network Adapter Info"
      description="This machine's adapters: IP, subnet, gateway, DNS, DHCP and MAC."
      help={
        <>
          <p>
            The same information as <code>ipconfig /all</code> or <code>ip addr</code>, without the
            wall of text. The connected adapter carrying this machine's traffic is marked Primary.
          </p>
          <p className="mt-1.5">
            Virtual, loopback and disconnected adapters are folded away at the bottom. Fields the
            operating system does not report are labelled rather than guessed at.
          </p>
        </>
      }
      actions={
        <Button
          onClick={() => void load()}
          disabled={loading}
          icon={<RefreshCw size={14} className={loading ? 'animate-spin' : undefined} />}
        >
          Refresh
        </Button>
      }
    >
      {error !== null && (
        <p className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg">{error}</p>
      )}

      {error === null && loading && report === null && (
        <div className="flex items-center gap-2 text-sm text-fg-muted">
          <Spinner /> Reading adapters
        </div>
      )}

      {report !== null && (
        <div className="space-y-4">
          <p className="text-xs text-fg-muted">
            <span className="font-medium text-fg">{report.hostname}</span>
            {' on '}
            {report.os === 'windows' ? 'Windows' : report.os === 'darwin' ? 'macOS' : 'Linux'}
            {' · '}
            {adapters.length} adapter{adapters.length === 1 ? '' : 's'}
            {searchDomains.length > 0 && ` · search ${searchDomains.join(', ')}`}
          </p>

          {shown.length === 0 && (
            <p className="text-sm text-fg-muted">
              No connected adapter was found. Check the cable or Wi-Fi connection, then refresh.
            </p>
          )}

          {shown.map((a) => (
            <AdapterCard key={a.name + a.index} adapter={a} report={report} />
          ))}

          {hidden.length > 0 && (
            <details className="group rounded border border-border bg-surface-2">
              <summary className="flex cursor-pointer list-none items-center gap-1.5 px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
                <ChevronRight
                  size={14}
                  className="shrink-0 transition-transform group-open:rotate-90"
                  aria-hidden
                />
                Show {hidden.length} virtual, disconnected and loopback adapter
                {hidden.length === 1 ? '' : 's'}
              </summary>
              <div className="space-y-3 border-t border-border p-3">
                {hidden.map((a) => (
                  <AdapterCard key={a.name + a.index} adapter={a} report={report} />
                ))}
              </div>
            </details>
          )}
        </div>
      )}
    </ToolShell>
  )
}
