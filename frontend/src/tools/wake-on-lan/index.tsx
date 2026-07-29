import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CircleCheck, FileDown, FileUp, Pencil, Plus, Power, Trash2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  Spinner,
  TextInput,
  ToolShell,
  useToast,
  type Column,
} from '../../components'
import { downloadJson } from '../../lib/download'
import { errorMessage } from '../../lib/format'
import { getAppInfo, readDoc, writeDoc } from '../../shell/bindings'
import {
  checkAwake,
  wakeDevice,
  wakeTargets,
  type Awake,
  type Send,
  type TargetList,
  type WakeResult,
} from './api'
import {
  canonicalMac,
  docWarning,
  draftOf,
  ensureIds,
  exportDoc,
  exportFileName,
  filterDevices,
  groupBySite,
  mergeDevices,
  migrateDoc,
  newDeviceId,
  siteNames,
  sortDevices,
  validateDevice,
  DEFAULT_PORT,
  EMPTY_DRAFT,
  MAC_MESSAGE,
  WOL_DOC_VERSION,
  WOL_NAMESPACE,
  type DeviceDraft,
  type DeviceErrors,
  type WolDevice,
  type WolDoc,
} from './devices'

const POLL_INTERVAL_MS = 3000
const POLL_LIMIT_MS = 120000

const NO_STORE_MESSAGE =
  'The saved list needs the desktop app. You can still wake a machine by typing its MAC address.'

const HELP = (
  <>
    <p>
      A magic packet is six 0xFF bytes followed by the target MAC address sixteen times, sent to the
      whole network. A sleeping machine has no IP address to aim at, which is why a MAC address, not
      an IP, is what wakes it.
    </p>
    <p className="mt-1.5">
      CHIT sends several copies on purpose: to the subnet broadcast and to 255.255.255.255, on
      ports 9 and 7, from every network this computer is on. Different network cards listen for
      different ones, and sending only one is the usual reason Wake-on-LAN looks like it does
      nothing.
    </p>
    <p className="mt-1.5">
      The saved list is a plain JSON file in the CHIT data folder. Export and Import are how a team
      keeps the same list of machines on every tech's laptop.
    </p>
  </>
)

export default function WakeOnLanPage() {
  const toast = useToast()

  const [devices, setDevices] = useState<WolDevice[]>([])
  const [storeReady, setStoreReady] = useState(true)
  const [docNote, setDocNote] = useState('')

  const [query, setQuery] = useState('')
  const [site, setSite] = useState('')
  const [draft, setDraft] = useState<DeviceDraft | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [errors, setErrors] = useState<DeviceErrors>({})
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const [macText, setMacText] = useState('')
  const [macError, setMacError] = useState<string>()
  const [secureOn, setSecureOn] = useState('')
  const [result, setResult] = useState<WakeResult | null>(null)
  const [wakeError, setWakeError] = useState('')
  const [targets, setTargets] = useState<TargetList | null>(null)

  const [watchIp, setWatchIp] = useState('')
  const [poll, setPoll] = useState<{ ip: string; startedAt: number } | null>(null)
  const [awake, setAwake] = useState<Awake | null>(null)
  const [waited, setWaited] = useState(0)
  const [watchError, setWatchError] = useState('')

  useEffect(() => {
    void getAppInfo().then((info) => setStoreReady(info !== null))
    readDoc<unknown>(WOL_NAMESPACE)
      .then((raw) => {
        if (raw === null) return
        setDocNote(docWarning(raw))
        setDevices(sortDevices(ensureIds(migrateDoc(raw).devices, newDeviceId)))
      })
      .catch(() => setDocNote(''))
  }, [])

  useEffect(() => {
    wakeTargets()
      .then(setTargets)
      .catch(() => setTargets(null))
  }, [])

  useEffect(() => {
    if (poll === null) return
    let stopped = false

    const tick = async () => {
      let answer: Awake
      try {
        answer = await checkAwake(poll.ip)
      } catch (err) {
        if (stopped) return
        setWatchError(errorMessage(err))
        setPoll(null)
        return
      }
      if (stopped) return
      const elapsed = Date.now() - poll.startedAt
      setWaited(Math.round(elapsed / 1000))
      if (answer.alive) {
        setAwake(answer)
        setPoll(null)
        return
      }
      if (elapsed >= POLL_LIMIT_MS) setPoll(null)
    }

    void tick()
    const timer = setInterval(() => void tick(), POLL_INTERVAL_MS)
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [poll])

  const persist = useCallback(
    (next: WolDevice[]) => {
      setDevices(next)
      writeDoc(WOL_NAMESPACE, { version: WOL_DOC_VERSION, devices: next } satisfies WolDoc).catch(
        () =>
          toast.push(
            'error',
            'The device list could not be saved. Check that the CHIT data folder is still there.',
          ),
      )
    },
    [toast],
  )

  const shown = useMemo(() => filterDevices(devices, query, site), [devices, query, site])
  const groups = useMemo(() => groupBySite(shown), [shown])
  const siteOptions = useMemo(
    () => [
      { value: '', label: 'All sites' },
      ...siteNames(devices).map((name) => ({ value: name, label: name })),
    ],
    [devices],
  )

  const sends = useMemo(
    () => [...(result?.sent ?? []), ...(result?.failed ?? [])],
    [result],
  )

  const columns = useMemo<Column<Send>[]>(
    () => [
      { key: 'adapter', header: 'Adapter' },
      { key: 'from', header: 'From', width: '10rem' },
      { key: 'to', header: 'To', width: '10rem' },
      { key: 'port', header: 'Port', align: 'right', width: '5rem' },
      { key: 'error', header: 'Error' },
    ],
    [],
  )

  const targetLines = useMemo(() => {
    const lines: string[] = []
    for (const target of targets?.targets ?? []) {
      const line = `${target.adapter} to ${target.to}:${target.port}`
      if (!lines.includes(line)) lines.push(line)
    }
    return lines
  }, [targets])

  const openAdd = () => {
    setDraft({ ...EMPTY_DRAFT, site })
    setEditingId(null)
    setErrors({})
  }

  const openEdit = (device: WolDevice) => {
    setDraft(draftOf(device))
    setEditingId(device.id)
    setErrors({})
  }

  const remove = (id: string) => {
    persist(devices.filter((d) => d.id !== id))
    setConfirmId(null)
  }

  const saveDraft = () => {
    if (draft === null) return
    const checked = validateDevice(draft)
    if (!checked.ok) {
      setErrors(checked.errors)
      return
    }
    const clash = devices.find((d) => d.mac === checked.fields.mac && d.id !== editingId)
    if (clash !== undefined) {
      setErrors({ mac: `${clash.name} is already saved with that MAC address.` })
      return
    }
    setErrors({})

    const next =
      editingId === null
        ? [
            ...devices,
            {
              id: newDeviceId(),
              ...checked.fields,
              addedAt: new Date().toISOString(),
              lastWokenAt: '',
            },
          ]
        : devices.map((d) => (d.id === editingId ? { ...d, ...checked.fields } : d))
    persist(sortDevices(next))
    setDraft(null)
    setEditingId(null)
  }

  const wake = async (device: WolDevice | null) => {
    const mac = device === null ? macText.trim() : device.mac
    if (canonicalMac(mac) === null) {
      setMacError(MAC_MESSAGE)
      return
    }
    setMacError(undefined)
    setMacText(mac)
    setWatchIp(device?.ip ?? '')
    setPoll(null)
    setAwake(null)
    setWatchError('')
    setWaited(0)

    try {
      const woken = await wakeDevice({
        mac,
        broadcast: device?.broadcast ?? '',
        port: device?.port ?? DEFAULT_PORT,
        password: secureOn.trim(),
      })
      setResult(woken)
      setWakeError('')
      if (device !== null && (woken.sent ?? []).length > 0) {
        const at = new Date().toISOString()
        persist(devices.map((d) => (d.id === device.id ? { ...d, lastWokenAt: at } : d)))
      }
    } catch (err) {
      setResult(null)
      setWakeError(errorMessage(err))
    }
  }

  const onImport = async (file: File) => {
    let parsed: unknown = null
    try {
      parsed = JSON.parse(await file.text())
    } catch {
      parsed = null
    }
    const report = mergeDevices(devices, parsed, newDeviceId, new Date().toISOString())
    if (report.error !== '') {
      toast.push('error', report.error)
      return
    }
    persist(report.devices)
    toast.push(
      'success',
      `Imported: ${report.added} added, ${report.updated} updated, ${report.skipped} skipped.`,
    )
  }

  const onExport = () => {
    const now = new Date().toISOString()
    downloadJson(exportFileName(site, now), exportDoc(shown, now))
  }

  const sentCount = (result?.sent ?? []).length
  const networkCount = new Set((result?.sent ?? []).map((send) => send.adapter)).size

  return (
    <ToolShell
      title="Wake-on-LAN"
      description="Wake a machine by MAC address and keep a saved list per site."
      help={HELP}
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
        <section className="flex flex-col gap-3">
          <h2 className="text-sm font-semibold text-fg">Saved devices</h2>

          {!storeReady && <p className="text-xs text-warn">{NO_STORE_MESSAGE}</p>}
          {docNote !== '' && (
            <p role="alert" className="text-xs text-danger">
              {docNote}
            </p>
          )}

          <TextInput
            label="Filter devices"
            placeholder="Filter devices"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            spellCheck={false}
            autoComplete="off"
          />
          <Select
            label="Site"
            options={siteOptions}
            value={site}
            onChange={(event) => setSite(event.target.value)}
          />

          <div className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-2">
            {groups.length === 0 ? (
              <p className="py-4 text-center text-xs text-fg-muted">
                {devices.length === 0
                  ? 'No devices saved yet. Add one so you can wake it by name next time.'
                  : 'No device matches that filter. Clear the filter box or pick another site.'}
              </p>
            ) : (
              groups.map((group) => (
                <div key={group.site} className="flex flex-col gap-1">
                  <p className="text-xs font-medium text-fg-muted">
                    {group.site === '' ? 'No site' : group.site}
                  </p>
                  {group.devices.map((device) => (
                    <div
                      key={device.id}
                      className="rounded border border-border bg-surface px-2 py-1.5"
                    >
                      {confirmId === device.id ? (
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="text-sm text-fg">Delete "{device.name}"?</span>
                          <Button size="sm" variant="danger" onClick={() => remove(device.id)}>
                            Delete
                          </Button>
                          <Button size="sm" onClick={() => setConfirmId(null)}>
                            Cancel
                          </Button>
                        </div>
                      ) : (
                        <div className="flex items-center gap-2">
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-sm text-fg">{device.name}</p>
                            <p className="truncate font-mono text-xs text-fg-muted">
                              {device.mac}
                              {device.ip !== '' && ` / ${device.ip}`}
                            </p>
                          </div>
                          <Button
                            size="sm"
                            variant="primary"
                            onClick={() => void wake(device)}
                            icon={<Power size={14} aria-hidden />}
                          >
                            Wake
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={`Edit ${device.name}`}
                            onClick={() => openEdit(device)}
                            icon={<Pencil size={14} aria-hidden />}
                          />
                          <Button
                            size="sm"
                            variant="ghost"
                            aria-label={`Delete ${device.name}`}
                            onClick={() => setConfirmId(device.id)}
                            icon={<Trash2 size={14} aria-hidden />}
                          />
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              ))
            )}
          </div>

          <div className="flex flex-wrap gap-2">
            <Button onClick={openAdd} disabled={!storeReady} icon={<Plus size={14} aria-hidden />}>
              Add device
            </Button>
            <Button
              onClick={onExport}
              disabled={shown.length === 0}
              icon={<FileDown size={14} aria-hidden />}
            >
              Export
            </Button>
            <Button
              onClick={() => fileRef.current?.click()}
              disabled={!storeReady}
              icon={<FileUp size={14} aria-hidden />}
            >
              Import
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0]
                event.target.value = ''
                if (file !== undefined) void onImport(file)
              }}
            />
          </div>

          {draft !== null && (
            <form
              className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
              onSubmit={(event) => {
                event.preventDefault()
                saveDraft()
              }}
            >
              <p className="text-xs font-medium text-fg-muted">
                {editingId === null ? 'Add a device' : 'Edit device'}
              </p>
              <TextInput
                label="Name"
                value={draft.name}
                onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                placeholder="Reception PC"
                error={errors.name}
              />
              <TextInput
                label="MAC address"
                value={draft.mac}
                onChange={(event) => setDraft({ ...draft, mac: event.target.value })}
                placeholder="AA:BB:CC:DD:EE:FF"
                spellCheck={false}
                autoComplete="off"
                className="font-mono"
                error={errors.mac}
              />
              <TextInput
                label="IP address (optional)"
                value={draft.ip}
                onChange={(event) => setDraft({ ...draft, ip: event.target.value })}
                placeholder="192.168.1.42"
                spellCheck={false}
                autoComplete="off"
                error={errors.ip}
                hint="Used to check whether the machine came back up."
              />
              <TextInput
                label="Site (optional)"
                value={draft.site}
                onChange={(event) => setDraft({ ...draft, site: event.target.value })}
                placeholder="Head Office"
                error={errors.site}
              />
              <TextInput
                label="Notes (optional)"
                value={draft.notes}
                onChange={(event) => setDraft({ ...draft, notes: event.target.value })}
                placeholder="Wired NIC. WoL enabled in the BIOS."
                error={errors.notes}
              />

              <details className="rounded border border-border bg-surface">
                <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
                  Advanced
                </summary>
                <div className="flex flex-col gap-3 px-3 pt-1 pb-3">
                  <TextInput
                    label="Broadcast address (optional)"
                    value={draft.broadcast}
                    onChange={(event) => setDraft({ ...draft, broadcast: event.target.value })}
                    placeholder="192.168.1.255"
                    spellCheck={false}
                    autoComplete="off"
                    error={errors.broadcast}
                    hint="Leave empty and CHIT works it out from this computer's networks."
                  />
                  <TextInput
                    label="Port"
                    value={draft.port}
                    onChange={(event) => setDraft({ ...draft, port: event.target.value })}
                    inputMode="numeric"
                    error={errors.port}
                    hint="9 is normal. CHIT also tries port 7 when this is 9."
                  />
                  <TextInput
                    label="SecureOn password (optional)"
                    value={secureOn}
                    onChange={(event) => setSecureOn(event.target.value)}
                    placeholder="11-22-33-44"
                    spellCheck={false}
                    autoComplete="off"
                    className="font-mono"
                    hint="A few Realtek and Intel cards need this. It is not saved to the list, it applies to the wake-ups you send now."
                  />
                </div>
              </details>

              <div className="flex flex-wrap gap-2">
                <Button type="submit" variant="primary">
                  Save device
                </Button>
                <Button
                  onClick={() => {
                    setDraft(null)
                    setEditingId(null)
                    setErrors({})
                  }}
                >
                  Cancel
                </Button>
              </div>
            </form>
          )}
        </section>

        <section className="flex flex-col gap-4">
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(event) => {
              event.preventDefault()
              void wake(null)
            }}
          >
            <div className="min-w-56 flex-1">
              <TextInput
                label="MAC address"
                value={macText}
                onChange={(event) => setMacText(event.target.value)}
                placeholder="AA:BB:CC:DD:EE:FF"
                spellCheck={false}
                autoComplete="off"
                className="font-mono"
                error={macError}
                hint="The address of the machine you want to wake, not this one."
              />
            </div>
            <Button type="submit" variant="primary" icon={<Power size={14} aria-hidden />}>
              Wake
            </Button>
          </form>

          {targets !== null && (
            <p className="text-xs text-fg-muted">
              {targetLines.length === 0
                ? targets.note
                : `Packets will go out on: ${targetLines.join(', ')}`}
            </p>
          )}

          {wakeError !== '' && (
            <p
              role="alert"
              className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-danger"
            >
              {wakeError}
            </p>
          )}

          {result !== null && (
            <div className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3">
              <p className="flex items-center gap-2 text-sm text-fg">
                <CircleCheck size={16} className="shrink-0 text-ok" aria-hidden />
                Sent to {sentCount} {sentCount === 1 ? 'address' : 'addresses'} from {networkCount}{' '}
                {networkCount === 1 ? 'network' : 'networks'}.
                <span className="font-mono text-xs text-fg-muted">{result.mac}</span>
                {result.vendor !== '' && (
                  <span className="text-xs text-fg-muted">{result.vendor}</span>
                )}
              </p>

              <ResultsTable
                columns={columns}
                rows={sends}
                getRowId={(send) => `${send.from}-${send.to}-${send.port}`}
                csvName="wol-packets"
                emptyMessage="No packet was sent."
                rowStatus={(send) => (send.error === '' ? undefined : 'danger')}
              />

              {result.note !== '' && <p className="text-xs text-fg-muted">{result.note}</p>}

              <details className="rounded border border-border bg-surface">
                <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
                  What was sent
                </summary>
                <div className="flex flex-col gap-2 px-3 pt-1 pb-3">
                  <p className="font-mono text-xs break-all text-fg">{result.packetHex}</p>
                  <div>
                    <CopyButton value={result.packetHex} label="Copy packet" />
                  </div>
                </div>
              </details>
            </div>
          )}

          {watchIp !== '' && (
            <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  onClick={() => {
                    setAwake(null)
                    setWatchError('')
                    setWaited(0)
                    setPoll({ ip: watchIp, startedAt: Date.now() })
                  }}
                  disabled={poll !== null}
                >
                  Check if it is up
                </Button>
                {poll !== null && <Spinner label="Checking" />}
              </div>

              {poll !== null && (
                <p className="text-xs text-fg-muted">
                  Waiting for {watchIp} to answer, {waited} s so far. Machines take 20 to 60 seconds
                  to boot.
                </p>
              )}
              {awake !== null && (
                <p className="text-xs text-ok">
                  {awake.ip} is answering on {awake.via} after {waited} s.
                </p>
              )}
              {poll === null && awake === null && waited > 0 && watchError === '' && (
                <p className="text-xs text-warn">
                  {watchIp} still is not answering after 2 minutes. See the checklist below.
                </p>
              )}
              {watchError !== '' && (
                <p role="alert" className="text-xs text-danger">
                  {watchError}
                </p>
              )}
            </div>
          )}

          <div className="rounded border border-border bg-surface-2 px-3 py-2 text-xs text-fg-muted">
            <p className="font-medium text-fg">If the machine did not wake:</p>
            <ol className="mt-1 flex list-decimal flex-col gap-1 pl-4">
              <li>
                Wake-on-LAN has to be turned on in the BIOS or UEFI, usually called "Power On by
                PCIe" or "Wake on LAN".
              </li>
              <li>
                In Windows, the network card's properties need "Allow this device to wake the
                computer" ticked, and Fast Startup turned off, or a shut down machine will ignore
                the packet.
              </li>
              <li>
                Wi-Fi rarely works. Use the wired address, and make sure the cable is still live
                when the machine is asleep (the port light stays on).
              </li>
              <li>
                The packet only crosses a router if that router forwards directed broadcasts, which
                is off by default. From another site, wake it from a machine on the same network.
              </li>
              <li>
                A machine that is fully powered off at the wall, or whose power supply cut, cannot
                be woken by anything.
              </li>
            </ol>
          </div>
        </section>
      </div>
    </ToolShell>
  )
}
