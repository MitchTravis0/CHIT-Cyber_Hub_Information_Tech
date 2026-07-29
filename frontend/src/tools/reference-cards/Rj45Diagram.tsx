import { useState } from 'react'
import { Button } from '../../components'
import { CARDS } from './cards'

/**
 * The swatches are the literal colours of the wires, so they must not follow
 * the light and dark theme: an orange wire is orange in both. These are the
 * only fixed-palette colours in the tool, and every one is paired with the
 * colour written out in text so the card still works in greyscale.
 */
const SWATCH: Record<string, string> = {
  Orange: 'bg-orange-400',
  Green: 'bg-green-600',
  Blue: 'bg-blue-500',
  Brown: 'bg-amber-800',
  White: 'bg-white',
}

function Wire({ name }: { name: string }) {
  const parts = name.split('/')
  return (
    <div className="h-12 w-7 overflow-hidden rounded-sm border border-border" aria-hidden>
      {parts.length === 2 ? (
        <>
          <div className="h-1/2 bg-white" />
          <div className={`h-1/2 ${SWATCH[parts[1]] ?? 'bg-white'}`} />
        </>
      ) : (
        <div className={`h-full ${SWATCH[parts[0]] ?? 'bg-white'}`} />
      )}
    </div>
  )
}

export function Rj45Diagram() {
  const [standard, setStandard] = useState<'B' | 'A'>('B')
  const card = CARDS.find((item) => item.id === 'rj45')
  if (card === undefined) return null
  const column = standard === 'B' ? 0 : 1

  return (
    <div className="flex flex-col gap-2 rounded border border-border bg-surface-2 p-3">
      <div className="flex gap-2">
        <Button
          size="sm"
          variant={standard === 'B' ? 'primary' : 'secondary'}
          onClick={() => setStandard('B')}
        >
          T568B
        </Button>
        <Button
          size="sm"
          variant={standard === 'A' ? 'primary' : 'secondary'}
          onClick={() => setStandard('A')}
        >
          T568A
        </Button>
      </div>
      <div className="flex gap-1">
        {card.entries.map((entry) => (
          <div key={entry.id} className="flex w-14 flex-col items-center gap-1">
            <span className="text-xs text-fg-muted">{entry.key}</span>
            <Wire name={column === 0 ? entry.label : entry.extra[0]} />
            <span className="text-center text-[10px] leading-tight text-fg">
              {column === 0 ? entry.label : entry.extra[0]}
            </span>
          </div>
        ))}
      </div>
      <p className="text-xs text-fg-muted">
        Pins 1 to 8, clip away from you, contacts towards you.
      </p>
    </div>
  )
}
