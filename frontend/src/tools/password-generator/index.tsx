import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { Button, CopyButton, Select, StatusDot, TextInput, ToolShell } from '../../components'
import {
  charEntropyBits,
  generatePassphrase,
  generatePassword,
  optionsError,
  parseCount,
  parseLength,
  parseWordCount,
  phraseEntropyBits,
  strengthFor,
  type CharOptions,
  type PhraseOptions,
  type SeparatorValue,
} from './generate'

const MODES = [
  { value: 'chars', label: 'Random characters' },
  { value: 'words', label: 'Passphrase' },
]

const SEPARATORS = [
  { value: '-', label: 'Hyphen  -' },
  { value: '.', label: 'Full stop  .' },
  { value: '_', label: 'Underscore  _' },
  { value: ' ', label: 'Space' },
]

interface Generated {
  passwords: string[]
  bits: number
}

function Check({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (next: boolean) => void
}) {
  return (
    <label className="flex items-center gap-2 text-sm text-fg">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="size-4 accent-[var(--accent)]"
      />
      {label}
    </label>
  )
}

export default function PasswordGeneratorPage() {
  const [mode, setMode] = useState<'chars' | 'words'>('chars')
  const [length, setLength] = useState('20')
  const [lower, setLower] = useState(true)
  const [upper, setUpper] = useState(true)
  const [digits, setDigits] = useState(true)
  const [symbols, setSymbols] = useState(true)
  const [excludeLookAlikes, setExcludeLookAlikes] = useState(true)
  const [wordCount, setWordCount] = useState('5')
  const [separator, setSeparator] = useState<SeparatorValue>('-')
  const [capitalise, setCapitalise] = useState(false)
  const [addNumber, setAddNumber] = useState(false)
  const [count, setCount] = useState('1')
  // Bumped by the Generate button, so pressing it again with unchanged options
  // still produces a fresh set.
  const [nonce, setNonce] = useState(0)
  const [generated, setGenerated] = useState<Generated>({ passwords: [], bits: 0 })

  const parsedLength = parseLength(length)
  const parsedWords = parseWordCount(wordCount)
  const parsedCount = parseCount(count)

  const charOptions: CharOptions = {
    length: parsedLength.value,
    lower,
    upper,
    digits,
    symbols,
    excludeLookAlikes,
  }
  const classError = mode === 'chars' ? optionsError(charOptions) : null
  const canGenerate =
    parsedLength.ok && parsedWords.ok && parsedCount.ok && classError === null

  useEffect(() => {
    if (!canGenerate) return
    const chars: CharOptions = {
      length: parsedLength.value,
      lower,
      upper,
      digits,
      symbols,
      excludeLookAlikes,
    }
    const phrase: PhraseOptions = {
      words: parsedWords.value,
      separator,
      capitalise,
      addNumber,
    }
    const passwords: string[] = []
    for (let i = 0; i < parsedCount.value; i++) {
      passwords.push(mode === 'chars' ? generatePassword(chars) : generatePassphrase(phrase))
    }
    setGenerated({
      passwords,
      bits: mode === 'chars' ? charEntropyBits(chars) : phraseEntropyBits(phrase),
    })
  }, [
    canGenerate,
    mode,
    parsedLength.value,
    lower,
    upper,
    digits,
    symbols,
    excludeLookAlikes,
    parsedWords.value,
    separator,
    capitalise,
    addNumber,
    parsedCount.value,
    nonce,
  ])

  const strength = strengthFor(generated.bits)
  const many = generated.passwords.length > 1

  return (
    <ToolShell
      title="Password Generator"
      description="Generate strong passwords or passphrases, with the exact strength in bits."
      help={
        <>
          <p>
            Press Generate for a fresh set. The number underneath is how many bits of randomness the
            password actually has, which is the honest way to compare two passwords: 20 random
            characters is worth far more than "Summer2026!" no matter how many symbols the second one
            has. Anything at 75 bits or above is fine for an admin account.
          </p>
          <p className="mt-2">
            Random characters is what you want when a machine will store the password and nobody has
            to say it out loud. Passphrase is what you want when a user has to read it over the phone
            or type it on a tablet: five words are easier to get right than twelve symbols, and the
            separator keeps the words apart so nobody has to guess where one ends. Capitalising the
            words looks stronger but adds nothing, because it happens to every word every time.
          </p>
          <p className="mt-2">
            Look-alike characters (capital I, lowercase l, the digit 1, capital O, the digit 0,
            lowercase o) are left out by default, because those are the ones that get written on a
            job sheet and typed back wrong. Turn that off if the password is only ever going to be
            pasted. Nothing on this page is saved anywhere: copy what you need before you leave.
          </p>
        </>
      }
      actions={many ? <CopyButton value={generated.passwords.join('\n')} label="Copy all" /> : undefined}
    >
      <div className="flex flex-col gap-4">
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            setNonce((current) => current + 1)
          }}
        >
          <Select
            label="Mode"
            className="w-56"
            options={MODES}
            value={mode}
            onChange={(event) => setMode(event.target.value as 'chars' | 'words')}
          />

          {mode === 'chars' ? (
            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap items-end gap-3">
                <TextInput
                  label="Length"
                  type="number"
                  min={8}
                  max={128}
                  className="w-28"
                  value={length}
                  onChange={(event) => setLength(event.target.value)}
                  error={parsedLength.error ?? undefined}
                />
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                  <Check label="Lowercase a-z" checked={lower} onChange={setLower} />
                  <Check label="Uppercase A-Z" checked={upper} onChange={setUpper} />
                  <Check label="Digits 0-9" checked={digits} onChange={setDigits} />
                  <Check label="Symbols !#$%" checked={symbols} onChange={setSymbols} />
                </div>
              </div>
              <Check
                label="Exclude look-alike characters (I l 1 O 0 o)"
                checked={excludeLookAlikes}
                onChange={setExcludeLookAlikes}
              />
              {classError !== null && (
                <p role="alert" className="text-xs text-danger">
                  {classError}
                </p>
              )}
            </div>
          ) : (
            <div className="flex flex-wrap items-end gap-3">
              <TextInput
                label="Words"
                type="number"
                min={3}
                max={12}
                className="w-28"
                value={wordCount}
                onChange={(event) => setWordCount(event.target.value)}
                error={parsedWords.error ?? undefined}
              />
              <Select
                label="Separator"
                className="w-44"
                options={SEPARATORS}
                value={separator}
                onChange={(event) => setSeparator(event.target.value as SeparatorValue)}
              />
              <Check label="Capitalise each word" checked={capitalise} onChange={setCapitalise} />
              <Check label="Add a number" checked={addNumber} onChange={setAddNumber} />
            </div>
          )}

          <div className="flex flex-wrap items-end gap-3">
            <TextInput
              label="How many"
              type="number"
              min={1}
              max={20}
              className="w-28"
              value={count}
              onChange={(event) => setCount(event.target.value)}
              error={parsedCount.error ?? undefined}
            />
            <Button
              type="submit"
              variant="primary"
              disabled={!canGenerate}
              icon={<RefreshCw size={14} aria-hidden />}
            >
              Generate
            </Button>
          </div>
        </form>

        <div className="rounded border border-border bg-surface-2">
          {generated.passwords.map((password, index) => (
            <div
              key={index}
              className="flex items-center gap-2 border-b border-border px-3 py-2 last:border-b-0"
            >
              {many && (
                <span className="w-6 shrink-0 text-xs text-fg-muted tabular-nums">{index + 1}</span>
              )}
              <span className="min-w-0 flex-1 font-mono text-sm break-all text-fg select-all">
                {password}
              </span>
              <CopyButton value={password} />
            </div>
          ))}
        </div>

        {classError === null && (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-semibold tabular-nums text-fg">{strength.bits} bits</span>
            <StatusDot status={strength.tone} label={strength.label} />
            <span className="text-xs text-fg-muted">{strength.advice}</span>
          </div>
        )}

        <p className="text-xs text-fg-muted">
          Nothing here is saved. Close this page and these passwords are gone, so copy the one you
          want before you move on.
        </p>
      </div>
    </ToolShell>
  )
}
