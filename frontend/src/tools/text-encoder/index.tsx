import { Fragment, useId, useMemo, useState } from 'react'
import { ArrowUpDown, TriangleAlert } from 'lucide-react'
import { Button, CopyButton, Select, StatusDot, Textarea, ToolShell } from '../../components'
import { convert, type Conversion, type Direction } from './codecs'
import { decodeJwt } from './jwt'

const CONVERSIONS = [
  { value: 'base64', label: 'Base64' },
  { value: 'base64url', label: 'Base64 (URL-safe)' },
  { value: 'url', label: 'URL / percent encoding' },
  { value: 'hex', label: 'Hex' },
  { value: 'html', label: 'HTML entities' },
  { value: 'jwt', label: 'JWT (decode only)' },
]

const DIRECTIONS = [
  { value: 'encode', label: 'Encode' },
  { value: 'decode', label: 'Decode' },
]

const PLACEHOLDERS: Record<Conversion, Record<Direction, string>> = {
  base64: { encode: 'Hello world', decode: 'SGVsbG8gd29ybGQ=' },
  base64url: { encode: 'Hello world', decode: 'SGVsbG8gd29ybGQ' },
  url: { encode: 'name=Ana & Bob/Smith', decode: 'name%3DAna%20%26%20Bob' },
  hex: { encode: 'Hello', decode: '48656c6c6f' },
  html: { encode: '<b>Bob & "Ana"</b>', decode: '&lt;b&gt;Bob &amp; &quot;Ana&quot;&lt;/b&gt;' },
  jwt: { encode: '', decode: 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.c2ln' },
}

const PRE_CLASS =
  'max-h-80 overflow-auto rounded border border-border bg-surface-2 px-3 py-2 font-mono text-sm break-words whitespace-pre-wrap text-fg'

function PaneHeading({ label, copy }: { label: string; copy?: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-xs font-medium text-fg-muted">{label}</span>
      {copy !== undefined && <CopyButton value={copy} />}
    </div>
  )
}

export default function TextEncoderPage() {
  const inputId = useId()
  const [input, setInput] = useState('')
  const [conversion, setConversion] = useState<Conversion>('base64')
  const [direction, setDirection] = useState<Direction>('encode')

  const result = useMemo(
    () => (conversion === 'jwt' ? null : convert(input, conversion, direction)),
    [input, conversion, direction],
  )
  // A fresh clock on every render, so "expired 4 hours ago" is right whenever
  // the page is looked at.
  const token = useMemo(
    () => (conversion === 'jwt' ? decodeJwt(input, new Date()) : null),
    [input, conversion],
  )

  const error = result?.error ?? token?.error ?? null
  const canReuse = result !== null && result.ok && result.text !== ''

  return (
    <ToolShell
      title="Text Encoder / Decoder"
      description="Base64, URL, hex and HTML entities both ways, plus a read-only JWT decoder."
      help={
        <>
          <p>
            Pick what the value is, pick which way you are going, and paste it in. The answer appears
            as you type. "Use output as input" moves the result back up into the box and flips the
            direction, so you can check a conversion by running it back the other way.
          </p>
          <p className="mt-2">
            Base64 comes in two flavours. The plain kind uses + and / and pads the end with =, and is
            what you find in config files and email headers. The URL-safe kind uses - and _ instead
            and usually drops the padding, because + and / mean something else in a web address. If
            one of them says the value has a character it never uses, try the other one: that message
            is telling you which flavour you actually have.
          </p>
          <p className="mt-2">
            Everything here is treated as UTF-8, so accented letters and emoji survive the round
            trip. If a decode says the result is not readable text, that is usually correct rather
            than a fault: you have decoded a file, an image or something encrypted, and there is
            nothing to read.
          </p>
          <p className="mt-2">
            JWT is decode only. It shows you the header, the payload and when the token expires, and
            it deliberately does not check the signature: doing that properly needs the issuer's key.
            A token you can read is not a token you can trust, so use this to answer "has it expired"
            and "which account is this for", not "is it genuine".
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <Select
            label="Conversion"
            className="w-56"
            options={CONVERSIONS}
            value={conversion}
            onChange={(event) => {
              const next = event.target.value as Conversion
              setConversion(next)
              if (next === 'jwt') setDirection('decode')
            }}
          />
          <Select
            label="Direction"
            className="w-40"
            options={DIRECTIONS}
            value={direction}
            disabled={conversion === 'jwt'}
            hint={conversion === 'jwt' ? 'A token can only be read, not built.' : undefined}
            onChange={(event) => setDirection(event.target.value as Direction)}
          />
        </div>

        <Textarea
          id={inputId}
          label="Input"
          hint={
            conversion === 'url' && direction === 'decode'
              ? 'A + is left as a plus. Web forms use + for a space, so replace them first if that is what you have.'
              : undefined
          }
          rows={8}
          value={input}
          onChange={(event) => setInput(event.target.value)}
          placeholder={PLACEHOLDERS[conversion][direction]}
          spellCheck={false}
          autoComplete="off"
          autoFocus
          className="resize-y font-mono"
        />

        <div className="flex flex-wrap items-center gap-2">
          <Button
            icon={<ArrowUpDown size={14} aria-hidden />}
            disabled={!canReuse}
            onClick={() => {
              if (result === null) return
              setInput(result.text)
              setDirection(direction === 'encode' ? 'decode' : 'encode')
            }}
          >
            Use output as input
          </Button>
          <Button variant="ghost" onClick={() => setInput('')}>
            Clear
          </Button>
        </div>

        {conversion === 'jwt' && (
          <div className="flex items-start gap-2 rounded border border-warn bg-warn/10 px-3 py-2 text-sm text-fg">
            <TriangleAlert size={16} aria-hidden className="mt-0.5 shrink-0" />
            <span>
              This shows you what is inside the token. It does not check the signature, so nothing
              here proves the token is genuine or that nobody has altered it. Treat it as something
              you are reading, not something you trust.
            </span>
          </div>
        )}

        {error !== null && (
          <p role="alert" className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg">
            {error}
          </p>
        )}

        {result !== null && error === null && (
          <div className="flex flex-col gap-1">
            <PaneHeading label="Output" copy={result.text} />
            {input === '' ? (
              <p className="rounded border border-border bg-surface-2 px-3 py-2 text-sm text-fg-muted">
                Paste something above and the converted text appears here.
              </p>
            ) : (
              <pre className={PRE_CLASS}>{result.text}</pre>
            )}
            {result.note !== null && (
              <p className="flex items-center gap-1 text-xs text-warn">
                <TriangleAlert size={14} aria-hidden />
                {result.note}
              </p>
            )}
          </div>
        )}

        {token !== null && error === null && !token.ok && (
          <p className="text-sm text-fg-muted">Paste a token above. The banner stays whatever you paste.</p>
        )}

        {token !== null && token.ok && (
          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              {token.state === 'expired' && <StatusDot status="danger" label="Expired" />}
              {token.state === 'valid' && <StatusDot status="ok" label="Not expired" />}
              {token.state === 'none' && <StatusDot status="warn" label="No expiry" />}
              <span className="text-fg">{token.verdict}</span>
            </div>

            {token.notYet !== null && <p className="text-xs text-warn">{token.notYet}</p>}

            {token.times.length > 0 && (
              <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[10rem_1fr] text-sm">
                {token.times.map((time) => (
                  <Fragment key={time.claim}>
                    <dt className="text-fg-muted">{time.label}</dt>
                    <dd className="text-fg">{time.absolute}</dd>
                  </Fragment>
                ))}
              </dl>
            )}

            <div className="flex flex-col gap-1">
              <PaneHeading label="Header" copy={token.header} />
              <pre className={PRE_CLASS}>{token.header}</pre>
            </div>

            <div className="flex flex-col gap-1">
              <PaneHeading label="Payload" copy={token.payload} />
              <pre className={PRE_CLASS}>{token.payload}</pre>
            </div>

            <div className="flex flex-col gap-1">
              <PaneHeading label="Signature (not checked)" />
              <p className="rounded border border-border bg-surface-2 px-3 py-2 font-mono text-xs break-all text-fg-muted">
                {token.signature === ''
                  ? 'This token has no signature at all. The third block is empty, which means nothing signed it.'
                  : token.signature}
              </p>
            </div>
          </div>
        )}
      </div>
    </ToolShell>
  )
}
