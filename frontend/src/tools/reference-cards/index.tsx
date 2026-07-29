import { useMemo, useState } from 'react'
import { Button, ResultsTable, TextInput, ToolShell, type Column } from '../../components'
import { Icon } from '../../shell/Icon'
import { Rj45Diagram } from './Rj45Diagram'
import { CARDS, MAX_SEARCH_HITS, searchCards, type CardDef, type CardEntry, type CardId, type SearchHit } from './cards'

const NO_MATCHES = 'No matches. Try a port number, a status code, or a word like "crossover".'

const CAPPED = 'Only the first 100 matches are shown. Add another word to narrow it down.'

const RIGHT_ALIGNED = new Set(['Port', 'Code', 'Channel', 'Addresses', 'Usable hosts'])

function cardColumns(card: CardDef): Column<CardEntry>[] {
  return card.columns.map((header, index) => ({
    key: `col-${index}`,
    header,
    align: RIGHT_ALIGNED.has(header) ? ('right' as const) : ('left' as const),
    value: (row: CardEntry) =>
      index === 0 ? row.key : index === 1 ? row.label : (row.extra[index - 2] ?? ''),
  }))
}

export default function ReferenceCardsPage() {
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<CardId>('rj45')

  const card = CARDS.find((item) => item.id === selected) ?? CARDS[0]
  const hits = useMemo(() => searchCards(query), [query])
  const columns = useMemo(() => cardColumns(card), [card])

  const hitColumns = useMemo<Column<SearchHit>[]>(
    () => [
      { key: 'card', header: 'Card', width: '12rem', value: (row) => row.card.name },
      { key: 'key', header: 'Entry', width: '8rem', value: (row) => row.entry.key },
      { key: 'label', header: 'Means', width: '16rem', value: (row) => row.entry.label },
      { key: 'detail', header: 'Detail', value: (row) => row.entry.extra.join(' - ') },
    ],
    [],
  )

  const searching = query.trim() !== ''

  return (
    <ToolShell
      title="IT Reference Cards"
      description="The lookups every tech googles: cable pinouts, port numbers, status codes, Wi-Fi channels and more."
      help={
        <>
          <p>
            Seven cards of the things worth having on a phone with no signal. Pick one with the
            buttons, or type into the search box to look across all of them at once: a port number,
            a status code, a channel, or a word like "crossover". Every table sorts and exports to
            CSV like the rest of CHIT.
          </p>
          <p className="mt-2">
            The subnet table and the SLA table are worked out by the app rather than typed in, so
            they cannot be quietly wrong. The port list, the status codes and the Wi-Fi channels are
            fixed reference data that does not change. Beep codes are the one card to treat with
            care: every BIOS vendor uses its own, and boards from the same maker differ, so use the
            card to narrow it down and then check the manual for that model.
          </p>
          <p className="mt-2">
            Nothing here needs a network, a login or anything from this machine, which is the point:
            it works in a basement. If you want a lookup that is not here, the Shared Snippet
            Library is where your own notes go.
          </p>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <div className="max-w-xl">
          <TextInput
            label="Search every card"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="568b, 3389, 404, dfs"
            spellCheck={false}
            autoComplete="off"
            autoFocus
            hint="Searches the rows of all seven cards at once."
          />
        </div>

        {searching ? (
          <>
            <p className="text-sm text-fg">
              {hits.length === 0
                ? NO_MATCHES
                : `${hits.length === 1 ? '1 match' : `${hits.length} matches`} for "${query.trim()}"`}
            </p>
            {hits.length >= MAX_SEARCH_HITS && <p className="text-xs text-warn">{CAPPED}</p>}
            <ResultsTable
              columns={hitColumns}
              rows={hits}
              getRowId={(row) => row.entry.id}
              csvName="reference-search"
              emptyMessage={NO_MATCHES}
            />
          </>
        ) : (
          <>
            <div className="flex flex-wrap gap-2">
              {CARDS.map((item) => (
                <Button
                  key={item.id}
                  size="sm"
                  variant={item.id === card.id ? 'primary' : 'secondary'}
                  onClick={() => setSelected(item.id)}
                  icon={<Icon name={item.icon} size={14} aria-hidden />}
                >
                  {item.name}
                </Button>
              ))}
            </div>

            <div className="flex flex-col gap-2">
              <h2 className="text-sm font-semibold text-fg">{card.name}</h2>
              <p className="text-xs text-fg-muted">{card.blurb}</p>
              {card.id === 'rj45' && <Rj45Diagram />}
              <ResultsTable
                columns={columns}
                rows={card.entries}
                getRowId={(row) => row.id}
                csvName={`reference-${card.id}`}
                emptyMessage="Nothing on this card."
              />
              <ul className="list-disc space-y-1 pl-5 text-xs text-fg-muted">
                {card.notes.map((note) => (
                  <li key={note}>{note}</li>
                ))}
              </ul>
            </div>
          </>
        )}
      </div>
    </ToolShell>
  )
}
