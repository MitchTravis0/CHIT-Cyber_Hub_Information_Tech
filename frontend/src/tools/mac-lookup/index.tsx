import type { FormEvent } from 'react'
import { useEffect, useState } from 'react'
import { Info, Search, TriangleAlert } from 'lucide-react'
import { Button, CopyButton, TextInput, ToolShell } from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { lookupMac, macDatabase, type MacDatabase, type MacInfo } from './api'

const HELP = (
  <>
    <p>
      Paste a MAC address in any form: <code>AA:BB:CC:DD:EE:FF</code>, <code>aa-bb-cc-dd-ee-ff</code>,{' '}
      <code>aabb.ccdd.eeff</code> or plain <code>aabbccddeeff</code>.
    </p>
    <p className="mt-2">
      The first three bytes are the manufacturer prefix. Some vendors share a prefix and get a
      smaller slice of it, so the answer can come from a longer match than the first three bytes.
    </p>
    <p className="mt-2">
      An address with no vendor is usually not a broken lookup. Phones and laptops randomize their
      address on purpose, and this tool says so when it sees one.
    </p>
  </>
)

/** The forms a tech might need to paste somewhere else, all built from the canonical one. */
function alternateForms(mac: string): Array<{ label: string; value: string }> {
  const bare = mac.replace(/:/g, '')
  return [
    { label: 'Colon', value: mac },
    { label: 'Hyphen', value: bare.replace(/(..)(?=.)/g, '$1-') },
    { label: 'Cisco', value: (bare.match(/.{4}/g) ?? []).join('.').toLowerCase() },
    { label: 'Plain', value: bare },
  ]
}

function tags(info: MacInfo): Array<{ text: string; tone: string }> {
  const out: Array<{ text: string; tone: string }> = []
  if (info.broadcast) out.push({ text: 'Broadcast', tone: 'border-accent text-accent' })
  else if (info.multicast) out.push({ text: 'Multicast', tone: 'border-accent text-accent' })
  if (info.randomized) out.push({ text: 'Randomized / private', tone: 'border-warn text-warn' })
  else if (info.locallyAdministered)
    out.push({ text: 'Locally administered', tone: 'border-warn text-warn' })
  if (info.found) out.push({ text: 'Registered prefix', tone: 'border-ok text-ok' })
  return out
}

function Result({ info }: { info: MacInfo }) {
  const warn = info.randomized || !info.found

  return (
    <div className="mt-4 rounded border border-border bg-surface-2">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border px-4 py-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-fg-muted">Manufacturer</p>
          <p
            className={cn(
              'mt-0.5 text-lg font-semibold break-words',
              info.found ? 'text-fg' : 'text-fg-muted',
            )}
          >
            {info.found ? info.vendor : 'No manufacturer on record'}
          </p>
        </div>
        {info.found && <CopyButton value={info.vendor} label="Copy name" />}
      </div>

      {tags(info).length > 0 && (
        <div className="flex flex-wrap gap-1.5 px-4 pt-3">
          {tags(info).map((tag) => (
            <span
              key={tag.text}
              className={cn('rounded border px-1.5 py-0.5 text-[0.6875rem] font-medium', tag.tone)}
            >
              {tag.text}
            </span>
          ))}
        </div>
      )}

      {info.note !== '' && (
        <div className="flex items-start gap-2 px-4 pt-3 text-xs leading-relaxed text-fg-muted">
          {warn ? (
            <TriangleAlert size={14} className="mt-px shrink-0 text-warn" aria-hidden />
          ) : (
            <Info size={14} className="mt-px shrink-0 text-accent" aria-hidden />
          )}
          <p>{info.note}</p>
        </div>
      )}

      <dl className="grid gap-x-4 gap-y-1 px-4 py-3 text-sm sm:grid-cols-[7rem_1fr]">
        <dt className="text-xs text-fg-muted sm:pt-1">Vendor prefix</dt>
        <dd className="font-mono text-fg">{info.oui}</dd>
        {alternateForms(info.mac).map((form) => (
          <div key={form.label} className="contents">
            <dt className="text-xs text-fg-muted sm:pt-1">{form.label}</dt>
            <dd className="flex min-w-0 items-center gap-1">
              <span className="truncate font-mono text-fg">{form.value}</span>
              <CopyButton value={form.value} />
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

export default function MacLookup() {
  const [input, setInput] = useState('')
  const [info, setInfo] = useState<MacInfo | null>(null)
  const [error, setError] = useState<string>()
  const [db, setDb] = useState<MacDatabase | null>(null)

  useEffect(() => {
    macDatabase()
      .then(setDb)
      .catch(() => setDb(null))
  }, [])

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    const text = input.trim()
    if (text === '') {
      setInfo(null)
      setError(undefined)
      return
    }
    try {
      setInfo(await lookupMac(text))
      setError(undefined)
    } catch (err) {
      setInfo(null)
      setError(errorMessage(err))
    }
  }

  return (
    <ToolShell
      title="MAC / OUI Vendor Lookup"
      description="Paste a MAC address and find out who made the device."
      help={HELP}
    >
      <form onSubmit={onSubmit} className="flex flex-wrap items-end gap-2">
        <div className="min-w-56 flex-1">
          <TextInput
            label="MAC address"
            placeholder="AA:BB:CC:DD:EE:FF"
            spellCheck={false}
            autoComplete="off"
            autoFocus
            value={input}
            aria-invalid={error !== undefined || undefined}
            onChange={(event) => setInput(event.target.value)}
            className="font-mono"
          />
        </div>
        <Button type="submit" variant="primary" icon={<Search size={14} />}>
          Look up
        </Button>
      </form>

      {error !== undefined && (
        <p role="alert" className="mt-2 text-xs text-danger">
          {error}
        </p>
      )}

      {info !== null && <Result info={info} />}

      {db !== null && (
        <p className="mt-6 text-xs text-fg-muted">
          {db.records.toLocaleString()} vendor prefixes, IEEE registry copied {db.fetched}. Refresh
          it with <code>go run internal/ouidb/gen.go</code>.
        </p>
      )}
    </ToolShell>
  )
}
