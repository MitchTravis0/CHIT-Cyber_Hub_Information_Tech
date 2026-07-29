import { useMemo, useState } from 'react'
import { ResultsTable, Textarea, TextInput, ToolShell, type Column } from '../../components'
import { groupDigits } from '../subnet-calculator/subnet'
import { parseRequirements, planSubnets, type Allocation, type FreeBlock } from './plan'

const DEFAULT_PARENT = '192.168.10.0/24'

const DEFAULT_REQUIREMENTS = [
  'Office PCs: 40',
  'Printers: 12',
  'Wi-Fi: 100',
  'Link to branch: /30',
].join('\n')

export default function SubnetPlannerPage() {
  const [parentText, setParentText] = useState(DEFAULT_PARENT)
  const [requirementsText, setRequirementsText] = useState(DEFAULT_REQUIREMENTS)

  const parsed = useMemo(() => parseRequirements(requirementsText), [requirementsText])
  const plan = useMemo(
    () => planSubnets(parentText, parsed.requirements),
    [parentText, parsed.requirements],
  )

  const columns = useMemo<Column<Allocation>[]>(
    () => [
      { key: 'order', header: '#', align: 'right', width: '3rem' },
      { key: 'name', header: 'Name', width: '12rem' },
      { key: 'requested', header: 'Asked for', width: '7rem' },
      { key: 'cidr', header: 'Subnet', width: '11rem' },
      { key: 'netmask', header: 'Mask', width: '10rem' },
      {
        key: 'range',
        header: 'Usable range',
        value: (row) => `${row.firstHost} to ${row.lastHost}`,
      },
      { key: 'broadcast', header: 'Broadcast', width: '10rem' },
      { key: 'usableHosts', header: 'Usable', align: 'right', width: '6rem' },
      { key: 'spare', header: 'Spare', align: 'right', width: '5rem' },
    ],
    [],
  )

  const freeColumns = useMemo<Column<FreeBlock>[]>(
    () => [
      { key: 'cidr', header: 'Block' },
      { key: 'totalAddresses', header: 'Addresses', align: 'right', width: '6rem' },
      { key: 'usableHosts', header: 'Usable', align: 'right', width: '6rem' },
    ],
    [],
  )

  // The parent's own message belongs on the field; the fit failure is a page error.
  const parentFailed = plan.error !== null && plan.parentCidr === ''

  return (
    <ToolShell
      title="Subnet Planner"
      description="Carve one network into subnets that fit a list of host counts, and see what is left over."
      help={
        <>
          <p>
            Type the block you have been given at the top, then list what has to fit inside it, one
            per line, as a name and a number of hosts. CHIT allocates the biggest subnet first,
            which is what stops the block fragmenting, and shows you each subnet's mask, usable
            range and broadcast address alongside how many spare addresses it has. Anything left
            over is listed underneath as whole blocks you can hand out later.
          </p>
          <p className="mt-2">
            A host count is turned into the smallest subnet that holds it, remembering that every
            IPv4 subnet loses two addresses to the network and the broadcast: ask for 30 hosts and
            you get a /27, which holds 30. The smallest this tool hands out on its own is a /30 (2
            usable), because plenty of kit still refuses a /31 even though the standard allows it.
            If you know yours is happy with one, write /31 on the line instead of a host count and
            you will get exactly that.
          </p>
          <p className="mt-2">
            If you want the same block split into equal pieces rather than sized ones, use the
            Subnet Calculator: it splits by count or by new prefix. This tool is for the other job,
            where the pieces are different sizes. IPv6 is deliberately not accepted here, because
            IPv6 links get a /64 each regardless of how many devices are on them.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <TextInput
              label="Network to carve up"
              value={parentText}
              onChange={(event) => setParentText(event.target.value)}
              placeholder="10.42.0.0/22"
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
              hint="CIDR, or an address and a mask. IPv4 only."
              error={parentFailed ? (plan.error ?? undefined) : undefined}
            />
          </div>
        </div>

        <Textarea
          label="What has to fit"
          value={requirementsText}
          onChange={(event) => setRequirementsText(event.target.value)}
          rows={8}
          className="font-mono"
          spellCheck={false}
          hint="One per line: a name, then a colon, then how many hosts. Use /30 to ask for a size directly."
        />

        {plan.note !== '' && <p className="text-xs text-fg-muted">{plan.note}</p>}

        {parsed.errors.length > 0 && (
          <div role="alert" className="flex flex-col gap-1">
            {parsed.errors.map((message) => (
              <p key={message} className="text-xs text-danger">
                {message}
              </p>
            ))}
          </div>
        )}

        {plan.error !== null && !parentFailed && (
          <p
            role="alert"
            className="rounded border border-danger bg-danger/10 px-3 py-2 text-sm text-fg"
          >
            {plan.error}
          </p>
        )}

        {plan.error === null && (
          <p className="text-sm text-fg">
            {plan.allocations.length === 1
              ? '1 subnet uses'
              : `${plan.allocations.length} subnets use`}{' '}
            {groupDigits(String(plan.usedAddresses))} of{' '}
            {groupDigits(String(plan.parentAddresses))} addresses in {plan.parentCidr}.{' '}
            {groupDigits(String(plan.freeAddresses))}{' '}
            {plan.freeAddresses === 1 ? 'address is' : 'addresses are'} still free.
          </p>
        )}

        <ResultsTable
          columns={columns}
          rows={plan.allocations}
          getRowId={(row) => String(row.order)}
          csvName={`subnet-plan-${(plan.parentCidr || 'network').replace(/[./]/g, '-')}`}
          emptyMessage='Add a line above, such as "Office PCs: 40", to see the plan.'
          rowStatus={(row) => (row.spare === 0 && row.hosts > 0 ? 'warn' : undefined)}
        />

        {plan.free.length > 0 && (
          <>
            <h2 className="text-sm font-semibold text-fg">Still free</h2>
            <ResultsTable
              columns={freeColumns}
              rows={plan.free}
              getRowId={(row) => row.cidr}
              csvName="subnet-plan-free"
              emptyMessage="Nothing left over."
            />
          </>
        )}
      </div>
    </ToolShell>
  )
}
