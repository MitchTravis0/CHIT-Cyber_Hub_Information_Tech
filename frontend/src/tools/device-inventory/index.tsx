import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FileDown, FileUp, Pencil, Plus, Trash2 } from 'lucide-react'
import {
  Button,
  ResultsTable,
  Select,
  TextInput,
  ToolShell,
  useToast,
  type Column,
} from '../../components'
import { downloadJson } from '../../lib/download'
import { getAppInfo, readDoc, writeDoc } from '../../shell/bindings'
import {
  csvName,
  docWarning,
  draftOf,
  ensureIds,
  exportDoc,
  exportFileName,
  filterDevices,
  mergeDevices,
  migrateDoc,
  newDeviceId,
  readImport,
  siteNames,
  sortDevices,
  validateDevice,
  EMPTY_DRAFT,
  INVENTORY_DOC_VERSION,
  INVENTORY_NAMESPACE,
  NO_STORE_MESSAGE,
  SAVE_FAILED_MESSAGE,
  type Device,
  type DeviceDraft,
  type DeviceErrors,
  type InventoryDoc,
} from './devices'

const HELP = (
  <>
    <p>
      This is the notebook page that does not get lost: what is at each site, what address it was
      given, and why. It is deliberately not a full asset database. Name, site and address are enough
      to answer "what did we set the printer to last time?".
    </p>
    <p className="mt-1.5">
      To fill it from a scan, run the IP Range Scanner, press Export CSV under the results, then
      press Import here and pick that file. You choose which site the rows belong to as you import
      them. An import never overwrites anything you have typed: it fills in blanks and adds devices
      it has not seen, so running it again after a second scan is safe.
    </p>
    <p className="mt-1.5">
      Export writes one JSON file holding whatever the filter is currently showing. That file is how
      the team shares a site: send it to a colleague, they press Import, and both machines hold the
      same list. A Wake-on-LAN device list exported from that tool imports here too.
    </p>
  </>
)

interface Staged {
  fileName: string
  devices: Device[]
  site: string
}

export default function DeviceInventoryPage() {
  const toast = useToast()

  const [devices, setDevices] = useState<Device[]>([])
  const [storeReady, setStoreReady] = useState(true)
  const [docNote, setDocNote] = useState('')

  const [query, setQuery] = useState('')
  const [site, setSite] = useState('')
  const [draft, setDraft] = useState<DeviceDraft | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [errors, setErrors] = useState<DeviceErrors>({})
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const [staged, setStaged] = useState<Staged | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    void getAppInfo().then((info) => setStoreReady(info !== null))
    readDoc<unknown>(INVENTORY_NAMESPACE)
      .then((raw) => {
        if (raw === null) return
        setDocNote(docWarning(raw))
        setDevices(sortDevices(ensureIds(migrateDoc(raw).devices, newDeviceId)))
      })
      .catch(() => setDocNote(''))
  }, [])

  const persist = useCallback(
    (next: Device[]) => {
      setDevices(next)
      writeDoc(INVENTORY_NAMESPACE, {
        version: INVENTORY_DOC_VERSION,
        devices: next,
      } satisfies InventoryDoc).catch(() => toast.push('error', SAVE_FAILED_MESSAGE))
    },
    [toast],
  )

  const shown = useMemo(() => filterDevices(devices, query, site), [devices, query, site])

  const siteOptions = useMemo(
    () => [
      { value: '', label: 'All sites' },
      ...siteNames(devices).map((name) => ({ value: name, label: name })),
    ],
    [devices],
  )

  const remove = useCallback(
    (id: string) => {
      persist(devices.filter((device) => device.id !== id))
      setConfirmId(null)
    },
    [devices, persist],
  )

  const openEdit = useCallback((device: Device) => {
    setDraft(draftOf(device))
    setEditingId(device.id)
    setErrors({})
  }, [])

  const columns = useMemo<Column<Device>[]>(
    () => [
      { key: 'name', header: 'Name' },
      {
        key: 'site',
        header: 'Site',
        width: '9rem',
        render: (row) =>
          row.site === '' ? <span className="text-fg-muted">No site</span> : row.site,
      },
      { key: 'ip', header: 'IP', width: '9rem' },
      {
        key: 'mac',
        header: 'MAC',
        width: '11rem',
        render: (row) => <span className="font-mono text-xs">{row.mac}</span>,
      },
      { key: 'vendor', header: 'Vendor' },
      { key: 'kind', header: 'Kind', width: '8rem' },
      { key: 'notes', header: 'Notes' },
      {
        key: 'actions',
        header: '',
        width: '6rem',
        sortable: false,
        value: () => null,
        render: (row) =>
          confirmId === row.id ? (
            <span className="flex items-center gap-1">
              <Button size="sm" variant="danger" onClick={() => remove(row.id)}>
                Delete
              </Button>
              <Button size="sm" onClick={() => setConfirmId(null)}>
                Cancel
              </Button>
            </span>
          ) : (
            <span className="flex items-center gap-1">
              <Button
                size="sm"
                variant="ghost"
                aria-label={`Edit ${row.name}`}
                onClick={() => openEdit(row)}
                icon={<Pencil size={14} aria-hidden />}
              />
              <Button
                size="sm"
                variant="ghost"
                aria-label={`Delete ${row.name}`}
                onClick={() => setConfirmId(row.id)}
                icon={<Trash2 size={14} aria-hidden />}
              />
            </span>
          ),
      },
    ],
    [confirmId, openEdit, remove],
  )

  const openAdd = () => {
    setDraft({ ...EMPTY_DRAFT, site })
    setEditingId(null)
    setErrors({})
  }

  const saveDraft = () => {
    if (draft === null) return
    const checked = validateDevice(draft)
    if (!checked.ok) {
      setErrors(checked.errors)
      return
    }
    const clash = devices.find(
      (device) =>
        device.mac !== '' &&
        device.mac === checked.fields.mac &&
        device.site === checked.fields.site &&
        device.id !== editingId,
    )
    if (clash !== undefined) {
      setErrors({ mac: `${clash.name} is already saved at that site with that MAC address.` })
      return
    }
    setErrors({})

    const at = new Date().toISOString()
    const next =
      editingId === null
        ? [...devices, { id: newDeviceId(), ...checked.fields, addedAt: at, updatedAt: at }]
        : devices.map((device) =>
            device.id === editingId ? { ...device, ...checked.fields, updatedAt: at } : device,
          )
    persist(sortDevices(next))
    setDraft(null)
    setEditingId(null)
  }

  const onFile = async (file: File) => {
    const result = readImport(await file.text())
    if (!result.ok) {
      toast.push('error', result.error)
      return
    }
    setStaged({ fileName: file.name, devices: result.devices, site })
  }

  const applyImport = () => {
    if (staged === null) return
    const report = mergeDevices(
      devices,
      staged.devices,
      staged.site,
      newDeviceId,
      new Date().toISOString(),
    )
    persist(report.devices)
    setStaged(null)
    toast.push(
      'success',
      `Imported: ${report.added} added, ${report.updated} updated, ${report.unchanged} already had everything.`,
    )
  }

  const onExport = () => {
    const now = new Date().toISOString()
    downloadJson(exportFileName(site, now), exportDoc(shown, now))
  }

  return (
    <ToolShell
      title="Saved Device Inventory"
      description="Keep a list of the devices at each site, fed by a scan or by hand."
      help={HELP}
      actions={
        <>
          <Button
            variant="primary"
            onClick={openAdd}
            disabled={draft !== null}
            icon={<Plus size={14} aria-hidden />}
          >
            Add device
          </Button>
          <Button onClick={() => fileRef.current?.click()} icon={<FileUp size={14} aria-hidden />}>
            Import
          </Button>
          <Button
            onClick={onExport}
            disabled={shown.length === 0}
            icon={<FileDown size={14} aria-hidden />}
          >
            Export
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {!storeReady && <p className="text-xs text-warn">{NO_STORE_MESSAGE}</p>}
        {docNote !== '' && (
          <p role="alert" className="text-xs text-danger">
            {docNote}
          </p>
        )}

        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <TextInput
              label="Filter devices"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Filter by name, IP, MAC or note"
              spellCheck={false}
              autoComplete="off"
            />
          </div>
          <div className="min-w-40">
            <Select
              label="Site"
              options={siteOptions}
              value={site}
              onChange={(event) => setSite(event.target.value)}
            />
          </div>
        </div>

        <input
          ref={fileRef}
          type="file"
          accept=".json,.csv,.txt,application/json,text/csv"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0]
            event.target.value = ''
            if (file !== undefined) void onFile(file)
          }}
        />

        {staged !== null && (
          <form
            className="flex flex-col gap-3 rounded border border-border bg-surface-2 px-3 py-3"
            onSubmit={(event) => {
              event.preventDefault()
              applyImport()
            }}
          >
            <p className="text-sm text-fg">
              Read {staged.devices.length}{' '}
              {staged.devices.length === 1 ? 'device' : 'devices'} from {staged.fileName}.
            </p>
            <div className="max-w-sm">
              <TextInput
                label="Site for these devices"
                value={staged.site}
                onChange={(event) => setStaged({ ...staged, site: event.target.value })}
                placeholder="Head Office"
                hint="Every imported device without its own site is filed under this name."
              />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary">
                Add to inventory
              </Button>
              <Button onClick={() => setStaged(null)}>Cancel</Button>
            </div>
          </form>
        )}

        {draft !== null && (
          <form
            className="grid gap-3 rounded border border-border bg-surface-2 px-3 py-3 sm:grid-cols-2 lg:grid-cols-3"
            onSubmit={(event) => {
              event.preventDefault()
              saveDraft()
            }}
          >
            <p className="text-xs font-medium text-fg-muted sm:col-span-2 lg:col-span-3">
              {editingId === null ? 'Add a device' : 'Edit device'}
            </p>
            <TextInput
              label="Name"
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
              placeholder="Reception printer"
              error={errors.name}
            />
            <TextInput
              label="Site"
              value={draft.site}
              onChange={(event) => setDraft({ ...draft, site: event.target.value })}
              placeholder="Head Office"
              error={errors.site}
            />
            <TextInput
              label="IP address"
              value={draft.ip}
              onChange={(event) => setDraft({ ...draft, ip: event.target.value })}
              placeholder="192.168.1.50"
              spellCheck={false}
              autoComplete="off"
              error={errors.ip}
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
              label="Vendor"
              value={draft.vendor}
              onChange={(event) => setDraft({ ...draft, vendor: event.target.value })}
              placeholder="Hewlett Packard"
              error={errors.vendor}
            />
            <TextInput
              label="Kind"
              value={draft.kind}
              onChange={(event) => setDraft({ ...draft, kind: event.target.value })}
              placeholder="Printer"
              error={errors.kind}
            />
            <div className="sm:col-span-2 lg:col-span-3">
              <TextInput
                label="Notes"
                value={draft.notes}
                onChange={(event) => setDraft({ ...draft, notes: event.target.value })}
                placeholder="Static. Toner is 26X."
                error={errors.notes}
              />
            </div>
            <div className="flex flex-wrap gap-2 sm:col-span-2 lg:col-span-3">
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

        <ResultsTable
          columns={columns}
          rows={shown}
          getRowId={(row) => row.id}
          csvName={csvName(site)}
          emptyMessage={
            devices.length === 0
              ? 'No devices saved yet. Press Add device, or Import a scan you exported from the IP Range Scanner.'
              : 'No device matches that filter. Clear the filter box or pick another site.'
          }
        />

        <p className="text-xs text-fg-muted">
          {devices.length === shown.length
            ? `${devices.length} devices saved.`
            : `${devices.length} devices saved, ${shown.length} shown.`}
        </p>
      </div>
    </ToolShell>
  )
}
