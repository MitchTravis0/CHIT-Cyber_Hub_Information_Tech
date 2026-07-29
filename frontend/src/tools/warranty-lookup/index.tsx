import { useCallback, useEffect, useMemo, useState } from 'react'
import { ExternalLink, Pencil, Plus, RotateCcw, Trash2 } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  TextInput,
  ToolShell,
  useToast,
  type Column,
} from '../../components'
import { getAppInfo, readDoc, writeDoc } from '../../shell/bindings'
import { openExternal } from './open'
import {
  addHistory,
  buildUrl,
  builtInVendors,
  docWarning,
  guessVendor,
  migrateDoc,
  newLookupId,
  newVendorId,
  stampOf,
  validateSerial,
  validateVendor,
  GUESS_MESSAGE,
  NO_APP_MESSAGE,
  NO_GUESS_MESSAGE,
  NO_STORE_MESSAGE,
  NO_VENDORS_MESSAGE,
  PASTE_TOAST,
  SAVE_FAILED_MESSAGE,
  WARRANTY_DOC_VERSION,
  WARRANTY_NAMESPACE,
  type Lookup,
  type Vendor,
  type VendorErrors,
  type WarrantyDoc,
} from './vendors'

const HELP = (
  <>
    <p>
      Type the serial, check the vendor is right, and press Open. Your normal browser opens the
      vendor's own warranty page with the serial already in it, so there is nothing to type twice.
    </p>
    <p className="mt-1.5">
      Dell calls it a Service Tag and it is seven characters. Lenovo is normally eight. Apple is ten
      or twelve. If you are not sure, wmic bios get serialnumber in a Command Prompt reads it off the
      machine, and CHIT will guess the vendor from the shape of what you type.
    </p>
    <p className="mt-1.5">
      HP and Microsoft do not let a link carry a serial. For those, CHIT opens the lookup page and
      puts the serial on your clipboard so you can paste it into their box. The panel above the link
      always tells you which kind you are dealing with.
    </p>
    <p className="mt-1.5">
      CHIT does not check the warranty itself and never reports a date. Every vendor keeps that
      behind a login or a paid API, so this tool gets you to their page fast and stops there. If a
      vendor changes their address, edit the link under Vendors and links and it is fixed for good.
    </p>
  </>
)

interface VendorDraft {
  id: string | null
  name: string
  url: string
  note: string
  carriesSerial: boolean
}

export default function WarrantyLookupPage() {
  const toast = useToast()

  const [vendors, setVendors] = useState<Vendor[]>(builtInVendors())
  const [history, setHistory] = useState<Lookup[]>([])
  const [storeReady, setStoreReady] = useState(true)
  const [docNote, setDocNote] = useState('')

  const [serial, setSerial] = useState('')
  const [serialError, setSerialError] = useState<string>()
  const [vendorId, setVendorId] = useState('dell')
  const [pickedByHand, setPickedByHand] = useState(false)
  const [noOpener, setNoOpener] = useState(false)
  const [confirmClear, setConfirmClear] = useState(false)

  const [draft, setDraft] = useState<VendorDraft | null>(null)
  const [draftErrors, setDraftErrors] = useState<VendorErrors>({})

  useEffect(() => {
    void getAppInfo().then((info) => setStoreReady(info !== null))
    readDoc<unknown>(WARRANTY_NAMESPACE)
      .then((raw) => {
        if (raw === null) return
        setDocNote(docWarning(raw))
        const doc = migrateDoc(raw)
        setVendors(doc.vendors)
        setHistory(doc.history)
        if (doc.vendors.length > 0) setVendorId(doc.vendors[0].id)
      })
      .catch(() => setDocNote(''))
  }, [])

  const persist = useCallback(
    (nextVendors: Vendor[], nextHistory: Lookup[]) => {
      setVendors(nextVendors)
      setHistory(nextHistory)
      writeDoc(WARRANTY_NAMESPACE, {
        version: WARRANTY_DOC_VERSION,
        vendors: nextVendors,
        history: nextHistory,
      } satisfies WarrantyDoc).catch(() => toast.push('error', SAVE_FAILED_MESSAGE))
    },
    [toast],
  )

  const guess = useMemo(() => guessVendor(serial), [serial])

  // The guess moves the picker until the tech overrides it, and then stops.
  useEffect(() => {
    if (pickedByHand || guess === '') return
    if (vendors.some((vendor) => vendor.id === guess)) setVendorId(guess)
  }, [guess, pickedByHand, vendors])

  const vendorOptions = useMemo(
    () =>
      vendors.length === 0
        ? [{ value: '', label: NO_VENDORS_MESSAGE }]
        : vendors.map((vendor) => ({ value: vendor.id, label: vendor.name })),
    [vendors],
  )

  const vendor = vendors.find((entry) => entry.id === vendorId) ?? null
  const url = vendor === null ? '' : buildUrl(vendor.url, serial)

  const columns = useMemo<Column<Lookup>[]>(
    () => [
      {
        key: 'serial',
        header: 'Serial',
        width: '12rem',
        render: (row) => <span className="font-mono text-xs">{row.serial}</span>,
      },
      { key: 'vendorName', header: 'Vendor', width: '10rem' },
      { key: 'stamp', header: 'Looked up', width: '12rem' },
      {
        key: 'actions',
        header: '',
        width: '7rem',
        sortable: false,
        value: () => null,
        render: (row) => (
          <Button
            size="sm"
            onClick={() => {
              setSerial(row.serial)
              setVendorId(row.vendorId)
              setPickedByHand(true)
            }}
          >
            Open again
          </Button>
        ),
      },
    ],
    [],
  )

  const onOpen = () => {
    const problem = validateSerial(serial)
    if (problem !== undefined || vendor === null) {
      setSerialError(problem)
      return
    }
    setSerialError(undefined)

    const opened = openExternal(buildUrl(vendor.url, serial))
    setNoOpener(!opened)
    if (opened && !vendor.carriesSerial) {
      void navigator.clipboard?.writeText(serial.trim()).catch(() => {})
      toast.push('info', PASTE_TOAST)
    }

    persist(
      vendors,
      addHistory(history, {
        id: newLookupId(),
        serial: serial.trim().toUpperCase(),
        vendorId: vendor.id,
        vendorName: vendor.name,
        stamp: stampOf(new Date()),
      }),
    )
  }

  const saveVendor = () => {
    if (draft === null) return
    const errors = validateVendor(draft)
    if (Object.keys(errors).length > 0) {
      setDraftErrors(errors)
      return
    }
    setDraftErrors({})
    const fields = {
      name: draft.name.trim(),
      url: draft.url.trim(),
      note: draft.note.trim(),
      carriesSerial: draft.carriesSerial,
    }
    const next =
      draft.id === null
        ? [...vendors, { id: newVendorId(), ...fields }]
        : vendors.map((entry) => (entry.id === draft.id ? { ...entry, ...fields } : entry))
    persist(next, history)
    if (draft.id === null) setVendorId(next[next.length - 1].id)
    setDraft(null)
  }

  return (
    <ToolShell
      title="Warranty / Serial Lookup Helper"
      description="Paste a serial number and open the right vendor warranty page, already filled in."
      help={HELP}
    >
      <div className="flex max-w-4xl flex-col gap-4">
        {!storeReady && <p className="text-xs text-warn">{NO_STORE_MESSAGE}</p>}
        {docNote !== '' && (
          <p role="alert" className="text-xs text-danger">
            {docNote}
          </p>
        )}

        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            onOpen()
          }}
        >
          <div className="min-w-56 flex-1">
            <TextInput
              label="Serial number or service tag"
              value={serial}
              onChange={(event) => setSerial(event.target.value)}
              placeholder="7XKQ2H3"
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
              error={serialError}
              hint="Read it off the sticker or run wmic bios get serialnumber."
            />
          </div>
          <div className="min-w-48">
            <Select
              label="Vendor"
              options={vendorOptions}
              value={vendorId}
              onChange={(event) => {
                setVendorId(event.target.value)
                setPickedByHand(true)
              }}
              hint={
                serial.trim() === '' ? undefined : guess === '' ? NO_GUESS_MESSAGE : GUESS_MESSAGE
              }
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={vendors.length === 0}
            icon={<ExternalLink size={14} aria-hidden />}
          >
            Open
          </Button>
          <CopyButton value={serial.trim()} label="Copy serial" />
        </form>

        {vendor !== null && serial.trim() !== '' && (
          <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 px-3 py-2">
            <p className="text-xs text-fg-muted">{vendor.note}</p>
            <p className="font-mono text-xs break-all text-fg">{url}</p>
            <div>
              <CopyButton value={url} label="Copy link" />
            </div>
            {noOpener && <p className="text-xs text-warn">{NO_APP_MESSAGE}</p>}
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-semibold text-fg">Recent lookups</h2>
          {history.length > 0 &&
            (confirmClear ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-fg">Clear all {history.length} recent lookups?</span>
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => {
                    persist(vendors, [])
                    setConfirmClear(false)
                  }}
                >
                  Clear
                </Button>
                <Button size="sm" onClick={() => setConfirmClear(false)}>
                  Cancel
                </Button>
              </div>
            ) : (
              <Button size="sm" variant="ghost" onClick={() => setConfirmClear(true)}>
                Clear history
              </Button>
            ))}
        </div>

        <ResultsTable
          columns={columns}
          rows={history}
          getRowId={(row) => row.id}
          csvName="warranty-lookups"
          emptyMessage="Nothing looked up yet. Type a serial above and press Open."
        />

        <details className="rounded border border-border bg-surface-2">
          <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
            Vendors and links
          </summary>
          <div className="flex flex-col gap-2 px-3 pt-1 pb-3">
            {vendors.map((entry) => (
              <div
                key={entry.id}
                className="flex flex-wrap items-start justify-between gap-2 rounded border border-border bg-surface px-2 py-1.5"
              >
                <div className="min-w-0">
                  <p className="text-sm text-fg">{entry.name}</p>
                  <p className="font-mono text-xs break-all text-fg-muted">{entry.url}</p>
                  {!entry.carriesSerial && (
                    <p className="text-xs text-fg-muted">
                      This link does not carry the serial, so CHIT copies it for you.
                    </p>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Edit ${entry.name}`}
                    onClick={() => {
                      setDraft({ ...entry })
                      setDraftErrors({})
                    }}
                    icon={<Pencil size={14} aria-hidden />}
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Remove ${entry.name}`}
                    onClick={() => {
                      const next = vendors.filter((item) => item.id !== entry.id)
                      persist(next, history)
                      if (vendorId === entry.id) setVendorId(next[0]?.id ?? '')
                    }}
                    icon={<Trash2 size={14} aria-hidden />}
                  />
                </div>
              </div>
            ))}

            <div className="flex flex-wrap gap-2">
              <Button
                onClick={() => {
                  setDraft({ id: null, name: '', url: '', note: '', carriesSerial: true })
                  setDraftErrors({})
                }}
                icon={<Plus size={14} aria-hidden />}
              >
                Add vendor
              </Button>
              <Button
                onClick={() => {
                  const restored = builtInVendors()
                  persist(restored, history)
                  setVendorId(restored[0].id)
                }}
                icon={<RotateCcw size={14} aria-hidden />}
              >
                Restore the built-in list
              </Button>
            </div>

            {draft !== null && (
              <form
                className="flex flex-col gap-3 rounded border border-border bg-surface px-3 py-3"
                onSubmit={(event) => {
                  event.preventDefault()
                  saveVendor()
                }}
              >
                <TextInput
                  label="Name"
                  value={draft.name}
                  onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                  placeholder="Acme"
                  error={draftErrors.name}
                />
                <TextInput
                  label="URL template"
                  value={draft.url}
                  onChange={(event) => setDraft({ ...draft, url: event.target.value })}
                  placeholder="https://support.acme.com/warranty?sn={serial}"
                  spellCheck={false}
                  autoComplete="off"
                  className="font-mono text-xs"
                  error={draftErrors.url}
                  hint="Put {serial} where the serial number goes."
                />
                <TextInput
                  label="Note"
                  value={draft.note}
                  onChange={(event) => setDraft({ ...draft, note: event.target.value })}
                  placeholder="Where to find the serial on this vendor's kit."
                />
                <label className="flex items-center gap-2 text-sm text-fg">
                  <input
                    type="checkbox"
                    checked={draft.carriesSerial}
                    onChange={(event) => setDraft({ ...draft, carriesSerial: event.target.checked })}
                    className="size-4 accent-[var(--accent)]"
                  />
                  This link carries the serial
                </label>
                <div className="flex flex-wrap gap-2">
                  <Button type="submit" variant="primary">
                    Save vendor
                  </Button>
                  <Button
                    onClick={() => {
                      setDraft(null)
                      setDraftErrors({})
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            )}
          </div>
        </details>
      </div>
    </ToolShell>
  )
}
