import type { FormEvent, KeyboardEvent } from 'react'
import { useMemo, useState } from 'react'
import { FileBadge, FolderOpen, Link2, TriangleAlert, X } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  StatusDot,
  Textarea,
  ToolShell,
  type Column,
  type RowTone,
  type StatusTone,
} from '../../components'
import { errorMessage } from '../../lib/format'
import { CertCard } from './CertCard'
import { decodeCertificateFile, decodeCertificateText, pickCertFile, type CertResult } from './api'
import type { DecodedCert } from './api'

const HELP = (
  <>
    <p>
      This reads a certificate and tells you what it is for, who issued it, which addresses it covers
      and when it runs out. It replaces the <code>openssl x509</code> incantation nobody remembers.
      Paste the text, or open the file directly if it is a binary <code>.cer</code> or{' '}
      <code>.der</code>. Nothing leaves this machine and nothing is saved.
    </p>
    <p className="mt-2">
      Paste the whole bundle when a supplier sends several certificates. A web server needs its own
      certificate <strong>and</strong> every intermediate certificate above it, in that order, and
      the commonest certificate fault there is comes from installing the first one without the rest.
      The line above the results tells you whether the pieces in your file fit together and what the
      right order is.
    </p>
    <p className="mt-2">
      The tool checks signatures <strong>only between the certificates you gave it</strong>. It
      deliberately does not ask this laptop whether it trusts the certificate, because this laptop is
      not the machine that matters and its answer would send you the wrong way: a browser here can
      trust a certificate that a phone refuses, and the other way round. To find out whether a live
      site is actually trusted, use the Website / Service Up Checker, which asks the real server.
    </p>
    <p className="mt-2">
      Two things worth knowing. A certificate that signed itself is normal on a printer, a NAS or a
      firewall admin page and wrong on a public website. And a certificate with no subject
      alternative names does not cover anything as far as a modern browser is concerned, however good
      the common name looks: it has to be reissued, not renewed.
    </p>
  </>
)

export default function CertDecoderPage() {
  const [text, setText] = useState('')
  const [result, setResult] = useState<CertResult | null>(null)
  const [error, setError] = useState<string>()
  const [pasteError, setPasteError] = useState<string>()

  const certs = result?.certificates ?? []
  const warnings = result?.warnings ?? []

  const columns = useMemo<Column<DecodedCert>[]>(
    () => [
      { key: 'index', header: '#', align: 'right', width: '3.5rem', value: (row) => row.index + 1 },
      {
        key: 'subject',
        header: 'Subject',
        value: (row) => row.subject.commonName || row.subjectLine,
      },
      {
        key: 'issuer',
        header: 'Issued by',
        value: (row) => row.issuer.commonName || row.issuerLine,
      },
      {
        key: 'kind',
        header: 'Kind',
        width: '8rem',
        value: (row) => (row.isCa ? 'Authority' : 'End entity'),
      },
      { key: 'notAfter', header: 'Expires', width: '12rem' },
      {
        key: 'daysRemaining',
        header: 'Days left',
        align: 'right',
        width: '7rem',
        render: (row) =>
          row.daysRemaining < 0 ? `${Math.abs(row.daysRemaining)} ago` : String(row.daysRemaining),
      },
      {
        key: 'statusLabel',
        header: 'Status',
        width: '10rem',
        render: (row) => <StatusDot status={row.status as StatusTone} label={row.statusLabel} />,
      },
    ],
    [],
  )

  const decodePasted = async () => {
    if (text.trim() === '') {
      setPasteError('Paste a certificate first, or use "Choose file" to open one.')
      return
    }
    setPasteError(undefined)
    try {
      setResult(await decodeCertificateText(text))
      setError(undefined)
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    }
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    void decodePasted()
  }

  const onChooseFile = async () => {
    try {
      const path = await pickCertFile()
      // An empty path means the dialog was cancelled, which is not a failure:
      // nothing on screen changes, so nothing is cleared either.
      if (path === '') return
      setPasteError(undefined)
      setResult(await decodeCertificateFile(path))
      setError(undefined)
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    }
  }

  const onClear = () => {
    setText('')
    setResult(null)
    setError(undefined)
    setPasteError(undefined)
  }

  // A textarea swallows Enter, which is right, so the keyboard shortcut for
  // submitting the form has to be wired up by hand.
  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault()
      void decodePasted()
    }
  }

  return (
    <ToolShell
      title="Certificate Decoder"
      description="Paste or open a certificate and read its subject, issuer, names, expiry and chain."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <form className="rounded border border-border bg-surface-2 p-3" onSubmit={onSubmit}>
          <Textarea
            id="cert-decoder-paste"
            label="Paste a certificate"
            hint="PEM text with the BEGIN and END lines, several of them for a chain, or just the base64 in between."
            error={pasteError}
            rows={8}
            value={text}
            onChange={(event) => setText(event.target.value)}
            onKeyDown={onKeyDown}
            className="font-mono text-xs"
            spellCheck={false}
            autoComplete="off"
            placeholder={'-----BEGIN CERTIFICATE-----\nMIID...'}
          />

          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Button type="submit" variant="primary" icon={<FileBadge size={14} aria-hidden />}>
              Decode
            </Button>
            <Button onClick={onChooseFile} icon={<FolderOpen size={14} aria-hidden />}>
              Choose file
            </Button>
            {(result !== null || error !== undefined) && (
              <Button variant="ghost" onClick={onClear} icon={<X size={14} aria-hidden />}>
                Clear
              </Button>
            )}
          </div>

          {result !== null && (
            <p className="mt-2 text-xs text-fg-muted">
              {result.source === 'pasted text'
                ? `Read as ${result.format}.`
                : `Read from ${result.source} as ${result.format}.`}
            </p>
          )}
        </form>

        {error !== undefined && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {warnings.length > 0 && (
          <ul className="rounded border border-warn bg-warn/10 px-3 py-2 text-xs text-fg">
            {warnings.map((warning) => (
              <li key={warning} className="flex items-start gap-2">
                <TriangleAlert size={14} className="shrink-0 text-warn" aria-hidden />
                {warning}
              </li>
            ))}
          </ul>
        )}

        {result !== null && result.chainNote !== '' && (
          <p className="flex items-start gap-2 text-sm text-fg">
            {result.inOrder ? (
              <Link2 size={16} aria-hidden />
            ) : (
              <TriangleAlert size={16} className="text-warn" aria-hidden />
            )}
            {result.chainNote}
          </p>
        )}

        {certs.length > 1 && (
          <ResultsTable
            columns={columns}
            rows={certs}
            getRowId={(row) => String(row.index)}
            csvName="certificates"
            emptyMessage="Nothing decoded yet."
            rowStatus={(row) => row.status as RowTone}
          />
        )}

        {certs.map((cert) => (
          <CertCard key={cert.index} cert={cert} total={certs.length} />
        ))}

        {certs.length > 1 && (
          <div>
            <CopyButton value={certs.map((cert) => cert.pem).join('')} label="Copy all as PEM" />
          </div>
        )}

        {result === null && error === undefined && (
          <p className="rounded border border-dashed border-border px-4 py-8 text-center text-sm text-fg-muted">
            Paste a certificate above, or press "Choose file" to open a .pem, .crt, .cer or .der
            file. Nothing is sent anywhere and nothing is saved.
          </p>
        )}
      </div>
    </ToolShell>
  )
}
