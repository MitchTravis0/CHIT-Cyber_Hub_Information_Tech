import type { ReactNode } from 'react'
import { TriangleAlert } from 'lucide-react'
import { CopyButton, StatusDot, type StatusTone } from '../../components'
import type { DecodedCert } from './api'

interface CertCardProps {
  cert: DecodedCert
  total: number
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="contents">
      <dt className="text-xs text-fg-muted sm:pt-1">{label}</dt>
      <dd className="min-w-0 break-words text-fg">{children}</dd>
    </div>
  )
}

function Missing({ text }: { text: string }) {
  return <span className="text-fg-muted italic">{text}</span>
}

/** A fingerprint or a serial: worth retyping somewhere else, so it gets copied. */
function Hex({ value }: { value: string }) {
  return (
    <span className="flex flex-wrap items-center gap-1">
      <span className="font-mono text-xs break-all">{value}</span>
      <CopyButton value={value} />
    </span>
  )
}

/** Every address the certificate says it covers, in one list a tech can copy. */
function Covers({ cert }: { cert: DecodedCert }) {
  const names = [
    ...(cert.dnsNames ?? []),
    ...(cert.ipAddresses ?? []),
    ...(cert.emailAddresses ?? []),
    ...(cert.uris ?? []),
  ]
  if (names.length === 0) return <Missing text="None. See the warning above." />

  return (
    <span className="flex flex-wrap items-center gap-1">
      {names.map((name) => (
        <span key={name} className="rounded border border-border px-1.5 py-0.5 font-mono text-xs">
          {name}
        </span>
      ))}
      <CopyButton value={names.join(', ')} />
    </span>
  )
}

export function CertCard({ cert, total }: CertCardProps) {
  const title = cert.subject.commonName !== '' ? cert.subject.commonName : cert.subjectLine
  const notes = cert.notes ?? []
  const keyUsage = cert.keyUsage ?? []
  const extendedKeyUsage = cert.extendedKeyUsage ?? []
  const remaining =
    cert.daysRemaining < 0
      ? `(${Math.abs(cert.daysRemaining)} days ago)`
      : `(${cert.daysRemaining} days left)`

  return (
    <article className="rounded border border-border bg-surface-2">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border px-4 py-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-fg-muted">
            Certificate {cert.index + 1} of {total}
          </p>
          <p className="mt-0.5 text-lg font-semibold break-words text-fg">{title}</p>
        </div>
        <CopyButton value={cert.pem} label="Copy as PEM" />
      </div>

      <div className="flex items-start gap-2 px-4 pt-3">
        <StatusDot status={cert.status as StatusTone} />
        <p className="text-sm text-fg">{cert.headline}</p>
      </div>

      {notes.length > 0 && (
        <ul className="px-4 pt-2 text-xs text-fg-muted">
          {notes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      )}

      <dl className="grid gap-x-4 gap-y-1 px-4 py-3 text-sm sm:grid-cols-[11rem_1fr]">
        <Row label="Issued to">{cert.subjectLine}</Row>
        <Row label="Issued by">{cert.issuerLine}</Row>
        <Row label="Covers">
          <Covers cert={cert} />
        </Row>
        <Row label="Valid from">{cert.notBefore}</Row>
        <Row label="Valid until">
          {cert.notAfter} <span className="text-fg-muted">{remaining}</span>
        </Row>
        <Row label="Serial number">
          <Hex value={cert.serialNumber} />
        </Row>
        <Row label="Signature">
          <span className="flex flex-wrap items-center gap-1">
            {cert.signatureAlgorithm}
            {cert.weakSignature && (
              <>
                <TriangleAlert size={14} className="text-danger" aria-hidden />
                <span className="text-danger">weak</span>
              </>
            )}
          </span>
        </Row>
        <Row label="Key">{cert.publicKeyLabel}</Row>
        <Row label="Key may be used for">
          {keyUsage.length > 0 ? (
            keyUsage.join(', ')
          ) : (
            <Missing text="Not stated, so it is not limited to any one use." />
          )}
        </Row>
        <Row label="Issued for">
          {extendedKeyUsage.length > 0 ? (
            extendedKeyUsage.join(', ')
          ) : (
            <Missing text="Not stated, so it is not limited to any one purpose." />
          )}
        </Row>
        <Row label="Authority">
          {cert.isCa ? `Yes. ${cert.pathLenText}` : 'No, this is an end certificate.'}
        </Row>
        <Row label="Signed itself">{cert.selfSigned ? 'Yes' : 'No'}</Row>
        <Row label="SHA-1 fingerprint">
          <Hex value={cert.sha1Fingerprint} />
        </Row>
        <Row label="SHA-256 fingerprint">
          <Hex value={cert.sha256Fingerprint} />
        </Row>
        <Row label="Subject key id">
          {cert.subjectKeyId !== '' ? (
            <span className="font-mono text-xs break-all">{cert.subjectKeyId}</span>
          ) : (
            <Missing text="Not present." />
          )}
        </Row>
        <Row label="Authority key id">
          {cert.authorityKeyId !== '' ? (
            <span className="font-mono text-xs break-all">{cert.authorityKeyId}</span>
          ) : (
            <Missing text="Not present." />
          )}
        </Row>
        <Row label="X.509 version">{cert.version}</Row>
      </dl>
    </article>
  )
}
