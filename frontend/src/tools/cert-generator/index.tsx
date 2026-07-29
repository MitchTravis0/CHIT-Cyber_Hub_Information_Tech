import { useMemo, useState } from 'react'
import { Download, FileKey, TriangleAlert } from 'lucide-react'
import {
  Button,
  CopyButton,
  ResultsTable,
  Select,
  Textarea,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { downloadText } from '../../lib/download'
import { errorMessage } from '../../lib/format'
import {
  fileNameFor,
  generateCsr,
  generateSelfSigned,
  splitNames,
  type CertResult,
  type FileKind,
} from './api'

const KEY_TYPES = [
  { value: 'ecdsa-p256', label: 'ECDSA P-256 (modern, small, fast)' },
  { value: 'rsa-2048', label: 'RSA 2048 (works with older kit)' },
]

const PEM_MIME = 'application/x-pem-file'

const KEY_WARNING =
  'This private key is shown once and is not saved anywhere. Download or copy it now: closing this page or generating again loses it, and there is no way to get it back.'

const EMPTY =
  'Fill in the name the device is reached by and press Generate. Nothing is sent anywhere: the key is made on this machine.'

const PFX_COMMAND = 'openssl pkcs12 -export -inkey nas.key -in nas.crt -out nas.pfx'

interface SummaryRow {
  field: string
  value: string
}

function PemPanel({
  title,
  body,
  fileName,
}: {
  title: string
  body: string
  fileName: string
}) {
  return (
    <div className="rounded border border-border bg-surface-2">
      <div className="flex items-center justify-between px-3 py-2">
        <span className="text-sm font-medium text-fg">{title}</span>
        <div className="flex gap-2">
          <CopyButton value={body} />
          <Button
            size="sm"
            onClick={() => downloadText(fileName, body, PEM_MIME)}
            icon={<Download size={14} aria-hidden />}
          >
            Download
          </Button>
        </div>
      </div>
      <div className="px-3 pb-3">
        <Textarea
          label={fileName}
          value={body}
          readOnly
          rows={10}
          className="font-mono text-xs"
          spellCheck={false}
        />
      </div>
    </div>
  )
}

export default function CertGeneratorPage() {
  const [mode, setMode] = useState<'self-signed' | 'csr'>('self-signed')
  const [commonName, setCommonName] = useState('')
  const [sansText, setSansText] = useState('')
  const [keyType, setKeyType] = useState('ecdsa-p256')
  const [days, setDays] = useState('397')
  const [organization, setOrganization] = useState('')
  const [orgUnit, setOrgUnit] = useState('')
  const [locality, setLocality] = useState('')
  const [state, setState] = useState('')
  const [country, setCountry] = useState('')
  const [email, setEmail] = useState('')
  const [result, setResult] = useState<CertResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const summary = useMemo<SummaryRow[]>(() => {
    if (result === null) return []
    const rows: SummaryRow[] = [
      { field: 'Subject', value: result.subject },
      { field: 'DNS names', value: result.dnsNames.join(', ') },
      { field: 'IP addresses', value: result.ipAddresses.join(', ') },
      { field: 'Key', value: result.keyLabel },
      { field: 'Valid from', value: result.notBefore },
      { field: 'Valid to', value: result.notAfter },
      { field: 'Days', value: result.days === 0 ? '' : String(result.days) },
      { field: 'Serial number', value: result.serialNumber },
      { field: 'SHA-256 fingerprint', value: result.fingerprint },
    ]
    return rows.filter((row) => row.value !== '')
  }, [result])

  const columns = useMemo<Column<SummaryRow>[]>(
    () => [
      { key: 'field', header: 'Field', width: '12rem' },
      {
        key: 'value',
        header: 'Value',
        render: (row) => <span className="font-mono break-all">{row.value}</span>,
      },
    ],
    [],
  )

  const switchMode = (next: 'self-signed' | 'csr') => {
    setMode(next)
    setResult(null)
    setError(null)
  }

  const onSubmit = async () => {
    setBusy(true)
    setError(null)
    const params = {
      commonName: commonName.trim(),
      sans: splitNames(sansText),
      organization,
      orgUnit,
      country,
      state,
      locality,
      email,
      keyType,
      days: mode === 'self-signed' ? Number(days) || 0 : 0,
    }
    try {
      setResult(mode === 'self-signed' ? await generateSelfSigned(params) : await generateCsr(params))
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const name = (kind: FileKind) => fileNameFor(result?.suggestedName ?? 'certificate', kind)

  return (
    <ToolShell
      title="Certificate & CSR Generator"
      description="Make a self-signed certificate or a signing request for a device, with no openssl commands."
      help={
        <>
          <p>
            A self-signed certificate is the quick answer for a device on your own network: a NAS, a
            switch GUI, an iDRAC, an internal wiki. It stops the traffic being unencrypted, but
            every browser will still warn about it until you import the certificate into the
            machines that need to trust it. A signing request (CSR) is the other route: you keep the
            private key here, send the request to your own certificate authority or a public one,
            and they hand back a certificate that browsers already trust.
          </p>
          <p className="mt-2">
            The name the device is reached by is the important field, and it must be the name people
            actually type. Browsers stopped looking at the common name years ago and only read the
            subject alternative names, so CHIT always adds the common name to that list for you. If
            the device is reached by more than one name, or by both a name and an IP address, put
            each on its own line in the second box.
          </p>
          <p className="mt-2">
            The private key is generated on this machine and is never saved, uploaded or written to
            disk by CHIT. It is shown once. Download it before you leave the page, keep it
            somewhere the customer controls, and never email it. If you generate again you get a
            completely new key and the old one is gone.
          </p>
          <p className="mt-2">
            Choose ECDSA P-256 unless something rejects it: it is smaller and faster and every
            current browser and OS takes it. Older printers, older switch firmware and anything from
            before about 2015 may only accept RSA, and that is what the RSA 2048 option is for. If a
            device wants a single .pfx file rather than two PEM files, the panel under the result has
            the one openssl command that converts them.
          </p>
        </>
      }
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault()
          if (!busy) void onSubmit()
        }}
      >
        <div className="flex gap-2">
          <Button
            size="sm"
            variant={mode === 'self-signed' ? 'primary' : 'secondary'}
            onClick={() => switchMode('self-signed')}
          >
            Self-signed certificate
          </Button>
          <Button
            size="sm"
            variant={mode === 'csr' ? 'primary' : 'secondary'}
            onClick={() => switchMode('csr')}
          >
            Signing request (CSR)
          </Button>
        </div>

        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <TextInput
              label="Name the device is reached by"
              value={commonName}
              onChange={(event) => setCommonName(event.target.value)}
              placeholder="nas.branch.local"
              spellCheck={false}
              autoComplete="off"
              autoFocus
              hint="The host name users type, or its IP address. This is added as a subject alternative name too."
            />
          </div>
          <Select
            label="Key type"
            options={KEY_TYPES}
            value={keyType}
            onChange={(event) => setKeyType(event.target.value)}
            hint="Choose RSA if the device refuses the certificate."
          />
          {mode === 'self-signed' && (
            <TextInput
              label="Valid for (days)"
              type="number"
              value={days}
              onChange={(event) => setDays(event.target.value)}
              className="w-32"
              hint="397 days is the longest most browsers accept."
            />
          )}
        </div>

        <Textarea
          label="Other names it answers to (optional)"
          value={sansText}
          onChange={(event) => setSansText(event.target.value)}
          rows={4}
          className="font-mono"
          spellCheck={false}
          hint="One per line. Host names or IP addresses, for example nas or 192.168.1.50."
        />

        <details className="rounded border border-border bg-surface-2">
          <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg">
            Who it belongs to (optional)
          </summary>
          <div className="grid gap-2 px-3 pb-3 sm:grid-cols-2">
            <TextInput
              label="Organisation"
              value={organization}
              onChange={(event) => setOrganization(event.target.value)}
              autoComplete="off"
            />
            <TextInput
              label="Department"
              value={orgUnit}
              onChange={(event) => setOrgUnit(event.target.value)}
              autoComplete="off"
            />
            <TextInput
              label="Town or city"
              value={locality}
              onChange={(event) => setLocality(event.target.value)}
              autoComplete="off"
            />
            <TextInput
              label="State or county"
              value={state}
              onChange={(event) => setState(event.target.value)}
              autoComplete="off"
            />
            <TextInput
              label="Country (2 letters)"
              value={country}
              onChange={(event) => setCountry(event.target.value)}
              autoComplete="off"
              maxLength={2}
            />
            <TextInput
              label="Email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="off"
            />
          </div>
        </details>

        <div>
          <Button
            type="submit"
            variant="primary"
            disabled={busy}
            icon={<FileKey size={14} aria-hidden />}
          >
            {mode === 'self-signed' ? 'Generate certificate' : 'Generate request'}
          </Button>
        </div>

        {error !== null && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {error}
          </p>
        )}

        {result === null ? (
          <div className="rounded border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">
            {EMPTY}
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            <p className="flex items-start gap-2 rounded border border-warn bg-warn/10 px-3 py-2 text-sm text-fg">
              <TriangleAlert size={16} aria-hidden className="mt-0.5 shrink-0" />
              {KEY_WARNING}
            </p>

            {result.warnings.map((warning) => (
              <p key={warning} className="text-xs text-warn">
                {warning}
              </p>
            ))}

            <ResultsTable
              columns={columns}
              rows={summary}
              getRowId={(row) => row.field}
              csvName={`certificate-${result.suggestedName}`}
              emptyMessage="Nothing to show."
            />

            <PemPanel
              title="Private key (keep this secret)"
              body={result.privateKeyPem}
              fileName={name('key')}
            />
            {result.certificatePem !== '' && (
              <PemPanel
                title="Certificate"
                body={result.certificatePem}
                fileName={name('crt')}
              />
            )}
            {result.csrPem !== '' && (
              <PemPanel title="Signing request" body={result.csrPem} fileName={name('csr')} />
            )}

            <details className="rounded border border-border bg-surface-2">
              <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg">
                What to do with these files
              </summary>
              <div className="flex flex-col gap-2 px-3 pb-3 text-xs text-fg-muted">
                <p>
                  Upload both files to the device: the key goes in the "private key" box and the
                  certificate in the "certificate" box. Keep the key somewhere the customer
                  controls, and delete your copy when you are done.
                </p>
                <p>
                  For a CSR, send only the request to the certificate authority. The private key
                  stays with you: anyone who has it can impersonate the device.
                </p>
                <p>
                  Browsers will keep warning about a self-signed certificate until it is trusted on
                  each machine. On Windows that is the Trusted Root Certification Authorities store
                  for the local machine.
                </p>
                <div className="flex flex-wrap items-center gap-2">
                  <span>Some devices only take a single .pfx file:</span>
                  <code className="rounded bg-surface-3 px-1.5 py-0.5 font-mono">{PFX_COMMAND}</code>
                  <CopyButton value={PFX_COMMAND} />
                </div>
              </div>
            </details>
          </div>
        )}
      </form>
    </ToolShell>
  )
}
