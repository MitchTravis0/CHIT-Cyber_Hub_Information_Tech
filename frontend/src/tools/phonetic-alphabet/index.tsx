import { useMemo, useState } from 'react'
import { TriangleAlert } from 'lucide-react'
import { CopyButton, TextInput, ToolShell } from '../../components'
import { convert, LETTERS } from './phonetic'

const EMPTY = 'Type or paste a serial, licence key or password above and the words to say appear here.'

const TRUNCATED =
  'Only the first 500 characters were converted. That is already a long way to read down a phone.'

export default function PhoneticAlphabetPage() {
  const [input, setInput] = useState('')
  const [showCapitals, setShowCapitals] = useState(true)

  const result = useMemo(() => convert(input, { showCapitals }), [input, showCapitals])

  return (
    <ToolShell
      title="Phonetic Alphabet Converter"
      description="Turn a serial, licence key or password into a NATO readout you can say down the phone."
      help={
        <>
          <p>
            Paste the serial, licence key or password you need to read out and say the words on the
            right, one line at a time. Each line shows the block exactly as you typed it, then the
            words for it. Where there is more than one block, the lines are numbered, so you can say
            "second block" and both of you know where you are.
          </p>
          <p className="mt-2">
            "Show capitals" is on because it usually matters. A capital letter reads as "Capital
            Bravo" and a lowercase one reads as "bravo", so the person typing knows which key to
            hold. Turn it off when the thing you are reading is not case sensitive, such as a MAC
            address or a hex serial, and every letter will just be "Bravo".
          </p>
          <p className="mt-2">
            Spaces are read out as the word "Space", because a space in a Wi-Fi password is a
            character the user has to type and silence does not tell them that. Anything not on a
            normal keyboard, such as an emoji or an accented letter, is shown as "Unknown character"
            with the character next to it rather than being quietly dropped: if you see one, check
            what you pasted, because it is often a smart quote that came from a Word document and
            will not work anyway.
          </p>
        </>
      }
      actions={
        result.text !== '' ? <CopyButton value={result.text} label="Copy readout" /> : undefined
      }
    >
      <div className="flex flex-col gap-4">
        <div className="max-w-2xl">
          <TextInput
            label="Serial, key or password"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            hint="Converts as you type."
            placeholder="4KD7-9F2X"
            spellCheck={false}
            autoComplete="off"
            autoCapitalize="off"
            autoFocus
            className="font-mono"
          />
        </div>

        <label className="flex items-center gap-2 text-sm text-fg">
          <input
            type="checkbox"
            checked={showCapitals}
            onChange={(event) => setShowCapitals(event.target.checked)}
            className="size-4 accent-[var(--accent)]"
          />
          Show capitals (Capital Alfa versus alfa)
        </label>

        <div className="rounded border border-border bg-surface-2 px-3 py-2">
          {result.text === '' ? (
            <p className="text-sm text-fg-muted">{EMPTY}</p>
          ) : (
            <p className="font-mono text-sm leading-relaxed whitespace-pre-wrap break-words text-fg">
              {result.text}
            </p>
          )}
        </div>

        {result.unknown.length > 0 && (
          <p className="text-xs text-warn">
            <TriangleAlert size={14} aria-hidden className="mr-1 inline align-text-bottom" />
            {`These characters are not in the phonetic table and are shown as "Unknown character": ${result.unknown.join(' ')}. Check what you pasted.`}
          </p>
        )}

        {result.truncated && <p className="text-xs text-warn">{TRUNCATED}</p>}

        <details className="rounded border border-border bg-surface-2">
          <summary className="cursor-pointer list-none px-3 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden">
            The full alphabet
          </summary>
          <div className="grid grid-cols-2 gap-x-6 gap-y-1 px-3 pb-3 sm:grid-cols-4">
            {Object.entries(LETTERS).map(([letter, word]) => (
              <div key={letter} className="text-xs text-fg">
                <span className="font-mono text-fg-muted">{letter}</span> {word}
              </div>
            ))}
          </div>
        </details>
      </div>
    </ToolShell>
  )
}
