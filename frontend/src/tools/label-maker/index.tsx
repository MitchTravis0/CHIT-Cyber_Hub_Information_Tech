import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Download, Printer } from 'lucide-react'
import { Button, Select, TextInput, ToolShell, useToast } from '../../components'
import { errorMessage } from '../../lib/format'
import { readDoc, writeDoc } from '../../shell/bindings'
import { generateQr, type QrCode } from './api'
import { downloadPng, printSheet } from './print'
import {
  DEFAULT_SETTINGS,
  LABEL_DOC_VERSION,
  LABEL_NAMESPACE,
  NO_QR_MESSAGE,
  PAGES,
  PNG_SCALES,
  PRESETS,
  PRINT_FAILED_MESSAGE,
  docWarning,
  labelSvg,
  layout,
  migrateDoc,
  pageById,
  pngFileName,
  presetById,
  previewCaption,
  sheetHtml,
  validateLabel,
  type Border,
  type LabelDoc,
  type LabelSettings,
  type PageId,
  type QrSide,
} from './labels'

const QR_DEBOUNCE_MS = 250
const SAVE_DEBOUNCE_MS = 500

const BORDERS = [
  { value: 'thin', label: 'Thin line' },
  { value: 'none', label: 'None' },
]

const QR_SIDES = [
  { value: 'right', label: 'Right' },
  { value: 'left', label: 'Left' },
]

const HELP = (
  <>
    <p>
      Fill in up to three lines, pick the size that matches the labels in your printer, and press
      Print. Line 1 is printed biggest, so put the asset tag or the hostname there. Anything you type
      in the QR box becomes a code a phone camera reads back, which is the quickest way to get a
      hostname off a machine you cannot log into.
    </p>
    <p className="mt-1.5">
      Print one page on plain paper first and hold it against a sheet of labels before you print on
      the real thing. Every printer places the page slightly differently and CHIT cannot know yours,
      so the first page is the calibration. Once it lines up, the rest of the box will too.
    </p>
    <p className="mt-1.5">
      Print goes through your normal print dialog, so "Save as PDF" there is how you get a PDF.
      Download PNG gives you one label as an image for pasting into a ticket or a document.
    </p>
    <p className="mt-1.5">
      QR codes are generated inside CHIT and nothing is sent anywhere. A code holding more than
      about 300 characters will not fit, and CHIT says so rather than printing a code that will not
      scan.
    </p>
  </>
)

export default function LabelMakerPage() {
  const toast = useToast()

  const [settings, setSettings] = useState<LabelSettings>(DEFAULT_SETTINGS)
  const [copiesText, setCopiesText] = useState(String(DEFAULT_SETTINGS.copies))
  const [docNote, setDocNote] = useState('')
  const [qr, setQr] = useState<QrCode | null>(null)
  const [qrError, setQrError] = useState('')
  const [qrAvailable, setQrAvailable] = useState(true)
  const loaded = useRef(false)

  useEffect(() => {
    readDoc<unknown>(LABEL_NAMESPACE)
      .then((raw) => {
        if (raw !== null) {
          setDocNote(docWarning(raw))
          const saved = migrateDoc(raw)
          setSettings(saved)
          setCopiesText(String(saved.copies))
        }
        loaded.current = true
      })
      .catch(() => {
        loaded.current = true
      })
  }, [])

  // Remembering a label size is a convenience, not data: a failed write is not
  // worth interrupting anyone over.
  useEffect(() => {
    if (!loaded.current) return
    const timer = setTimeout(() => {
      writeDoc(LABEL_NAMESPACE, {
        version: LABEL_DOC_VERSION,
        ...settings,
      } satisfies LabelDoc).catch(() => {})
    }, SAVE_DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [settings])

  // The QR code is a backend round trip, so it is debounced. The text lines are
  // local and redraw as fast as they are typed.
  useEffect(() => {
    const text = settings.qrText.trim()
    if (text === '') {
      setQr(null)
      setQrError('')
      return
    }
    let stopped = false
    const timer = setTimeout(() => {
      generateQr(text)
        .then((code) => {
          if (stopped) return
          setQrAvailable(code !== null)
          setQr(code)
          setQrError('')
        })
        .catch((err) => {
          if (stopped) return
          setQr(null)
          setQrError(errorMessage(err))
        })
    }, QR_DEBOUNCE_MS)
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  }, [settings.qrText])

  const preset = presetById(settings.presetId)
  const page = pageById(settings.page)
  const errors = validateLabel(settings, settings.qrText, copiesText)

  const matrix = useMemo(
    () => (qr === null ? null : { size: qr.size, modules: qr.modules, quiet: qr.quiet }),
    [qr],
  )

  const svg = useMemo(
    () =>
      labelSvg(settings, preset, matrix, { border: settings.border, qrSide: settings.qrSide }),
    [settings, preset, matrix],
  )

  const copies = errors.copies === undefined ? Number(copiesText) : settings.copies
  const plan = useMemo(() => layout(preset, copies, page), [preset, copies, page])

  const set = useCallback((change: Partial<LabelSettings>) => {
    setSettings((prev) => ({ ...prev, ...change }))
  }, [])

  const onPrint = () => {
    if (errors.copies !== undefined) return
    const labels = Array.from({ length: copies }, () => svg)
    if (!printSheet(sheetHtml(labels, preset, page))) {
      toast.push('error', PRINT_FAILED_MESSAGE)
    }
  }

  const onPng = () => {
    if (!downloadPng(svg, preset, settings.pngScale, pngFileName(settings.line1))) {
      toast.push('error', PRINT_FAILED_MESSAGE)
    }
  }

  return (
    <ToolShell
      title="Label Maker"
      description="Make printable equipment labels with an asset tag, a hostname, an address and a QR code."
      help={HELP}
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,24rem)_minmax(0,1fr)]">
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            onPrint()
          }}
        >
          {docNote !== '' && (
            <p role="alert" className="text-xs text-danger">
              {docNote}
            </p>
          )}

          <TextInput
            label="Line 1"
            value={settings.line1}
            onChange={(event) => set({ line1: event.target.value })}
            placeholder="CH-LT-042"
            error={errors.line1}
            hint="The biggest line. Usually the asset tag or the hostname."
          />
          <TextInput
            label="Line 2"
            value={settings.line2}
            onChange={(event) => set({ line2: event.target.value })}
            placeholder="Reception laptop"
            error={errors.line2}
          />
          <TextInput
            label="Line 3"
            value={settings.line3}
            onChange={(event) => set({ line3: event.target.value })}
            placeholder="192.168.1.42"
            error={errors.line3}
          />

          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-48 flex-1">
              <TextInput
                label="QR code contents"
                value={settings.qrText}
                onChange={(event) => set({ qrText: event.target.value })}
                placeholder="CH-LT-042"
                spellCheck={false}
                autoComplete="off"
                hint="Anything a phone camera should read back. Leave it empty for a label with no code."
              />
            </div>
            <Button onClick={() => set({ qrText: settings.line1 })}>Use line 1</Button>
          </div>

          {qrError !== '' && (
            <p role="alert" className="text-xs text-danger">
              {qrError}
            </p>
          )}
          {!qrAvailable && settings.qrText.trim() !== '' && (
            <p className="text-xs text-warn">{NO_QR_MESSAGE}</p>
          )}

          <Select
            label="Label size"
            options={PRESETS.map((entry) => ({ value: entry.id, label: entry.label }))}
            value={settings.presetId}
            onChange={(event) => set({ presetId: event.target.value })}
          />

          {preset.kind === 'sheet' && (
            <Select
              label="Page"
              options={PAGES.map((entry) => ({ value: entry.id, label: entry.label }))}
              value={settings.page}
              onChange={(event) => set({ page: event.target.value as PageId })}
            />
          )}

          <TextInput
            label="Copies"
            value={copiesText}
            onChange={(event) => {
              setCopiesText(event.target.value)
              const next = Number(event.target.value.trim())
              if (/^\d+$/.test(event.target.value.trim()) && next >= 1 && next <= 500) {
                set({ copies: next })
              }
            }}
            inputMode="numeric"
            error={errors.copies}
          />

          <details className="rounded border border-border bg-surface-2">
            <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
              Label options
            </summary>
            <div className="flex flex-col gap-3 px-3 pt-1 pb-3">
              <Select
                label="Border"
                options={BORDERS}
                value={settings.border}
                onChange={(event) => set({ border: event.target.value as Border })}
              />
              <Select
                label="QR position"
                options={QR_SIDES}
                value={settings.qrSide}
                onChange={(event) => set({ qrSide: event.target.value as QrSide })}
              />
              <Select
                label="PNG scale"
                options={PNG_SCALES.map((scale) => ({ value: String(scale), label: `${scale}x` }))}
                value={String(settings.pngScale)}
                onChange={(event) => set({ pngScale: Number(event.target.value) })}
              />
            </div>
          </details>
        </form>

        <section className="flex flex-col gap-3">
          <div className="rounded border border-border bg-surface-2 p-4">
            <div
              className="mx-auto w-full max-w-md"
              style={{ aspectRatio: `${preset.w} / ${preset.h}` }}
              // The preview is the same SVG that prints, so what is on screen is
              // what comes out of the printer.
              dangerouslySetInnerHTML={{ __html: svg }}
            />
          </div>

          <p className="text-xs text-fg-muted">{previewCaption(preset, plan, page)}</p>

          <div className="flex flex-wrap gap-2">
            <Button
              variant="primary"
              onClick={onPrint}
              disabled={errors.copies !== undefined}
              icon={<Printer size={14} aria-hidden />}
            >
              Print
            </Button>
            <Button onClick={onPng} icon={<Download size={14} aria-hidden />}>
              Download PNG
            </Button>
          </div>

          {preset.kind === 'sheet' && errors.copies === undefined && (
            <p className="text-xs text-fg-muted">
              {copies} {copies === 1 ? 'label' : 'labels'}, {plan.pages}{' '}
              {plan.pages === 1 ? 'page' : 'pages'}.
            </p>
          )}
        </section>
      </div>
    </ToolShell>
  )
}
