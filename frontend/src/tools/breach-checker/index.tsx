import type { FormEvent } from 'react'
import { useMemo, useState } from 'react'
import {
  CircleCheck,
  CircleHelp,
  Eye,
  EyeOff,
  ShieldAlert,
  ShieldCheck,
  TriangleAlert,
} from 'lucide-react'
import { Button, ProgressBar, Spinner, TextInput, ToolShell } from '../../components'
import { cn, errorMessage } from '../../lib/format'
import { checkPasswordBreach, type BreachResult } from './api'
import { score, type StrengthTone } from './strength'

// There is deliberately no CopyButton on this page. The only things worth
// copying are the password and its fingerprint, and putting either on the
// system clipboard is the opposite of what this tool is for.

const HELP = (
  <>
    <p>
      Two different questions get answered here. The bar tells you how hard the password would be to
      guess. The button tells you whether this exact password already appears in a public data
      breach, which is a much more urgent problem: a password on those lists is tried first, no
      matter how clever it looks.
    </p>
    <p className="mt-2">
      The strength rating is an estimate, not a promise. It counts how many kinds of character you
      used and how long the password is, then takes marks off for the patterns attackers try first:
      common passwords, runs like abcd or 1234, repeated characters, keyboard runs like qwerty, and
      years. Length beats cleverness. Four ordinary words in a row score far higher than eight
      characters of punctuation.
    </p>
    <p className="mt-2">
      The breach check never sends your password. CHIT works out the password's fingerprint on this
      machine and sends only the first 5 characters of it. Hundreds of thousands of different
      passwords share those 5 characters, and the comparison that finds yours happens here, not on
      their server.
    </p>
    <p className="mt-2">
      If the check says it could not reach the list, that is not a clean bill of health. It means the
      question was never asked, usually because this machine has no internet connection or something
      on the network is blocking api.pwnedpasswords.com. Try again before telling anyone the password
      is fine.
    </p>
  </>
)

function bandTone(tone: StrengthTone): { text: string; Icon: typeof ShieldAlert } {
  if (tone === 'danger') return { text: 'text-danger', Icon: ShieldAlert }
  if (tone === 'warn') return { text: 'text-warn', Icon: TriangleAlert }
  return { text: 'text-ok', Icon: ShieldCheck }
}

function verdictTone(level: string): { box: string; icon: string; Icon: typeof CircleCheck } {
  if (level === 'ok') return { box: 'border-ok bg-ok/10', icon: 'text-ok', Icon: CircleCheck }
  if (level === 'danger')
    return { box: 'border-danger bg-danger/10', icon: 'text-danger', Icon: ShieldAlert }
  return { box: 'border-warn bg-warn/10', icon: 'text-warn', Icon: CircleHelp }
}

function PrivacyPanel() {
  return (
    <div className="rounded border border-border bg-surface-2 px-3 py-2 text-xs text-fg-muted">
      <p className="font-medium text-fg">How this stays private</p>
      <p className="mt-1.5">
        The password you type stays on this computer. It is never sent anywhere, never saved to disk
        and never written to a log. Close the tool and it is gone.
      </p>
      <p className="mt-1.5">
        CHIT works out the password's SHA-1 fingerprint here, then sends only the first 5 characters
        of that fingerprint to haveibeenpwned.com. Those 5 characters are shared by hundreds of
        thousands of different passwords, so on their own they say nothing about yours.
      </p>
      <p className="mt-1.5">
        The service replies with every leaked fingerprint that starts with those 5 characters,
        usually several hundred of them, plus some padding entries so that even the size of the reply
        gives nothing away. CHIT compares the rest of your fingerprint against that list here.
        haveibeenpwned never learns which one was yours, or whether there was a match at all.
      </p>
    </div>
  )
}

function Verdict({ result }: { result: BreachResult }) {
  const { box, icon, Icon } = verdictTone(result.level)

  return (
    <div>
      <div className={cn('flex items-start gap-2 rounded border px-3 py-2 text-sm text-fg', box)}>
        <Icon size={16} className={cn('mt-0.5 shrink-0', icon)} aria-hidden />
        <p>{result.verdict}</p>
      </div>
      <p className="mt-1.5 text-xs text-fg-muted">
        {result.checked
          ? `Sent "${result.prefix}" and nothing else. ${result.compared.toLocaleString()} fingerprints came back and were compared here on this machine.`
          : 'Nothing was compared, because there was no answer to compare against.'}
      </p>
    </div>
  )
}

export default function BreachCheckerPage() {
  // Nothing is prefilled and nothing is remembered: the only state this tool has
  // is a credential, and storing one in plain text is forbidden outright.
  const [password, setPassword] = useState('')
  const [reveal, setReveal] = useState(false)
  const [result, setResult] = useState<BreachResult | null>(null)
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(false)

  const strength = useMemo(() => score(password), [password])
  const { text: bandText, Icon: BandIcon } = bandTone(strength.tone)

  const onChange = (next: string) => {
    setPassword(next)
    // A verdict from the previous password must never sit under a different one.
    setResult(null)
    setError(undefined)
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setLoading(true)
    try {
      setResult(await checkPasswordBreach(password))
      setError(undefined)
    } catch (err) {
      setResult(null)
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <ToolShell
      title="Password Strength / Breach Checker"
      description="Rate a password and check it against known breaches without ever sending the password."
      help={HELP}
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <TextInput
              label="Password to check"
              type={reveal ? 'text' : 'password'}
              value={password}
              onChange={(event) => onChange(event.target.value)}
              placeholder="Type or paste the password"
              autoComplete="new-password"
              spellCheck={false}
              autoFocus
              className="font-mono"
              hint="The rating updates as you type. Nothing is sent until you press the button."
            />
          </div>
          <Button
            variant="ghost"
            onClick={() => setReveal(!reveal)}
            icon={reveal ? <EyeOff size={14} aria-hidden /> : <Eye size={14} aria-hidden />}
          >
            {reveal ? 'Hide' : 'Show'}
          </Button>
        </div>

        {password !== '' && (
          <div>
            <ProgressBar
              value={Math.min(strength.bits, 100)}
              max={100}
              label={`${strength.band}, about ${strength.bits} bits`}
            />
            <p className="mt-1.5 flex items-start gap-1.5">
              <BandIcon size={16} className={cn('mt-px shrink-0', bandText)} aria-hidden />
              <span className={cn('text-sm font-semibold', bandText)}>{strength.band}</span>
              <span className="text-sm text-fg">{strength.advice}</span>
            </p>
            {strength.reasons.length > 0 && (
              <ul className="mt-1.5 ml-4 list-disc space-y-0.5 text-xs text-warn">
                {/* Two runs can produce the same sentence ("abcd!abcd"), so the
                    position is the only stable key. */}
                {strength.reasons.map((reason, index) => (
                  <li key={index}>{reason}</li>
                ))}
              </ul>
            )}
          </div>
        )}

        <PrivacyPanel />

        <div className="flex items-center gap-2">
          <Button
            type="submit"
            variant="primary"
            icon={<ShieldAlert size={14} aria-hidden />}
            disabled={loading || password === ''}
          >
            Check against breach lists
          </Button>
          {loading && <Spinner label="Asking the breach list" />}
        </div>

        {error !== undefined ? (
          <p role="alert" className="mt-2 text-xs text-danger">
            {error}
          </p>
        ) : (
          result !== null && <Verdict result={result} />
        )}
      </form>
    </ToolShell>
  )
}
