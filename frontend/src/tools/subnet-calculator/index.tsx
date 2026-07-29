import { useMemo, useState } from 'react'
import {
  CopyButton,
  ResultsTable,
  Select,
  TextInput,
  ToolShell,
  type Column,
} from '../../components'
import { errorMessage } from '../../lib/format'
import {
  describe,
  groupDigits,
  parse,
  split,
  splitInto,
  usableHosts,
  type Prefix,
  type SubnetInfo,
  type SubnetRow,
} from './subnet'

const EXAMPLE = '192.168.1.10/24'

/** How many prefix lengths below the network the split picker offers. Twelve
 *  steps is 4096 subnets, the most the maths will list. */
const SPLIT_STEPS = 12

interface Parsed {
  prefix: Prefix | null
  info: SubnetInfo | null
  error: string | null
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-1 rounded border border-border bg-surface-2 px-2 py-1.5">
      <div className="min-w-0">
        <div className="text-xs text-fg-muted">{label}</div>
        <div className="truncate font-mono text-sm text-fg">{value}</div>
      </div>
      <CopyButton value={value} />
    </div>
  )
}

function Badge({ children }: { children: string }) {
  return (
    <span className="rounded border border-border bg-surface-2 px-1.5 py-0.5 text-xs text-fg-muted">
      {children}
    </span>
  )
}

function fieldsFor(info: SubnetInfo): Array<{ label: string; value: string }> {
  return [
    { label: 'Address', value: info.address },
    { label: 'Network', value: info.cidr },
    { label: 'Prefix length', value: `/${info.prefixLength}` },
    { label: 'Subnet mask', value: info.netmask },
    { label: 'Wildcard mask', value: info.wildcard },
    { label: 'Network address', value: info.network },
    { label: 'Broadcast address', value: info.broadcast },
    { label: 'First usable host', value: info.firstHost },
    { label: 'Last usable host', value: info.lastHost },
    { label: 'Usable hosts', value: groupDigits(info.usableHosts) },
    { label: 'Total addresses', value: groupDigits(info.totalAddresses) },
    { label: 'Class', value: info.class },
  ].filter((field) => field.value !== '')
}

export default function SubnetCalculator() {
  const [input, setInput] = useState(EXAMPLE)
  const [mode, setMode] = useState<'prefix' | 'count'>('prefix')
  const [pickedBits, setPickedBits] = useState<number | null>(null)
  const [count, setCount] = useState('4')

  const parsed = useMemo<Parsed>(() => {
    try {
      const prefix = parse(input)
      return { prefix, info: { ...describe(prefix), input: input.trim() }, error: null }
    } catch (err) {
      return { prefix: null, info: null, error: errorMessage(err) }
    }
  }, [input])

  const { prefix, info } = parsed
  const maxBits = prefix?.v6 === true ? 128 : 32
  const bits = prefix?.bits ?? 0
  const topBits = Math.min(bits + SPLIT_STEPS, maxBits)
  // A picked prefix that no longer fits the network falls back to halves.
  const newBits =
    pickedBits !== null && pickedBits >= bits && pickedBits <= topBits
      ? pickedBits
      : Math.min(bits + 1, maxBits)

  const splitResult = useMemo(() => {
    if (prefix === null) return { rows: [] as SubnetRow[], error: null as string | null }
    try {
      const rows =
        mode === 'prefix' ? split(prefix, newBits) : splitInto(prefix, Number(count.trim()))
      return { rows, error: null as string | null }
    } catch (err) {
      return { rows: [] as SubnetRow[], error: errorMessage(err) }
    }
  }, [prefix, mode, newBits, count])

  const prefixOptions = useMemo(() => {
    if (prefix === null) return []
    const options = []
    for (let b = bits; b <= topBits; b++) {
      const hosts = usableHosts({ value: 0n, bits: b, v6: prefix.v6 }).toString()
      const pieces = 2 ** (b - bits)
      options.push({
        value: String(b),
        label: `/${b} - ${groupDigits(String(pieces))} subnet${pieces === 1 ? '' : 's'} of ${groupDigits(hosts)} usable`,
      })
    }
    return options
  }, [prefix, bits, topBits])

  const columns = useMemo<Column<SubnetRow>[]>(() => {
    const v6 = prefix?.v6 === true
    const all: Column<SubnetRow>[] = [
      { key: 'index', header: '#', align: 'right', width: '3rem' },
      { key: 'cidr', header: 'Subnet' },
      { key: 'netmask', header: 'Netmask' },
      { key: 'firstHost', header: 'First host' },
      { key: 'lastHost', header: 'Last host' },
      { key: 'broadcast', header: 'Broadcast' },
      {
        key: 'usableHosts',
        header: 'Usable hosts',
        align: 'right',
        render: (row) => groupDigits(row.usableHosts),
      },
    ]
    // IPv6 subnets have no dotted mask and no broadcast address.
    return v6 ? all.filter((column) => !['netmask', 'broadcast'].includes(column.key)) : all
  }, [prefix])

  const summary =
    info === null
      ? ''
      : fieldsFor(info)
          .map((field) => `${field.label}: ${field.value}`)
          .join('\n')

  return (
    <ToolShell
      title="Subnet Calculator"
      description="Turn an IP and mask into network, broadcast, usable range and host count."
      help={
        <>
          <p>
            Type an address with a prefix (<code>192.168.1.10/24</code>) or with a mask (
            <code>192.168.1.10 255.255.255.0</code>). An address on its own is treated as a single
            host. IPv6 works too, with a prefix length.
          </p>
          <p className="mt-2">
            A /31 is a point-to-point link: both addresses are usable and there is no broadcast
            address (RFC 3021). A /32 is one host. Every other IPv4 network loses its network and
            broadcast addresses, which is why a /24 gives 254 usable hosts and not 256.
          </p>
        </>
      }
      actions={info !== null ? <CopyButton value={summary} label="Copy all" /> : undefined}
    >
      <div className="flex flex-col gap-4">
        <div className="max-w-xl">
          <TextInput
            label="IP address and mask"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            error={parsed.error ?? undefined}
            hint="Updates as you type."
            placeholder={EXAMPLE}
            spellCheck={false}
            autoComplete="off"
            autoFocus
            className="font-mono"
          />
        </div>

        {info !== null && (
          <>
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge>{`IPv${info.version}`}</Badge>
              <Badge>{info.scope}</Badge>
              {info.private && <Badge>Private</Badge>}
              {info.loopback && <Badge>Loopback</Badge>}
              {info.linkLocal && <Badge>Link-local</Badge>}
              {info.cgnat && <Badge>CGNAT</Badge>}
            </div>

            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {fieldsFor(info).map((field) => (
                <Field key={field.label} label={field.label} value={field.value} />
              ))}
            </div>

            {info.note !== '' && <p className="text-xs text-fg-muted">{info.note}</p>}

            <section className="flex flex-col gap-2 border-t border-border pt-4">
              <h2 className="text-sm font-semibold text-fg">Split this network</h2>
              <div className="flex flex-wrap items-end gap-2">
                <Select
                  label="Split"
                  value={mode}
                  onChange={(event) => setMode(event.target.value as 'prefix' | 'count')}
                  options={[
                    { value: 'prefix', label: 'Into subnets of size' },
                    { value: 'count', label: 'Into a number of subnets' },
                  ]}
                  className="w-56"
                />
                {mode === 'prefix' ? (
                  <Select
                    label="New prefix"
                    value={String(newBits)}
                    onChange={(event) => setPickedBits(Number(event.target.value))}
                    options={prefixOptions}
                    className="w-72"
                  />
                ) : (
                  <TextInput
                    label="How many"
                    type="number"
                    min={1}
                    value={count}
                    onChange={(event) => setCount(event.target.value)}
                    hint="Rounded up to a power of two."
                    className="w-32"
                  />
                )}
              </div>

              {splitResult.error !== null ? (
                <p className="text-xs text-danger">{splitResult.error}</p>
              ) : (
                <ResultsTable
                  columns={columns}
                  rows={splitResult.rows}
                  getRowId={(row) => row.cidr}
                  emptyMessage="Nothing to split."
                  csvName={`subnets-${info.cidr.replace('/', '-')}`}
                />
              )}
            </section>
          </>
        )}
      </div>
    </ToolShell>
  )
}
