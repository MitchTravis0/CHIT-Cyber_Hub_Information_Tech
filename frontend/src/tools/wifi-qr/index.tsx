import type { FormEvent } from 'react'
import { useMemo, useState } from 'react'
import { Download, Eye, EyeOff, QrCode } from 'lucide-react'
import { Button, CopyButton, Select, Textarea, TextInput, ToolShell } from '../../components'
import { errorMessage } from '../../lib/format'
import { generateQr, type QrCode as QrCodeData } from './api'
import { downloadPng } from './png'
import { downloadName, modulesToPath } from './render'

const KINDS = [
  { value: 'wifi', label: 'Wi-Fi network' },
  { value: 'text', label: 'Text or link' },
]

const SECURITY = [
  { value: 'WPA', label: 'WPA / WPA2 (nearly every network)' },
  { value: 'SAE', label: 'WPA3 only' },
  { value: 'WEP', label: 'WEP (very old equipment)' },
  { value: 'nopass', label: 'Open, no password' },
]

const LEVELS = [
  { value: 'L', label: 'L, smallest code' },
  { value: 'M', label: 'M, the usual choice' },
  { value: 'Q', label: 'Q, survives more damage' },
  { value: 'H', label: 'H, survives the most damage' },
]

/** Pixels per module in the downloaded PNG. 8 prints crisply on a label. */
const SCALE = 8

const HELP = (
  <>
    <p>
      A Wi-Fi QR code holds the network name, the password and the security type as plain text. A
      phone camera reads it and offers to join, so nobody has to read a 24 character password out
      loud. Print it, stick it on the reception desk, and the question stops being asked.
    </p>
    <p className="mt-2">
      Type the network name exactly as the router advertises it, capital letters included. Wi-Fi
      names are case sensitive and a phone will not find <code>Guest-Wifi</code> when the network is
      really <code>Guest-WiFi</code>. Leave the security on WPA / WPA2 unless the network refuses
      older devices: that setting works on WPA and WPA2 networks and on WPA3 networks running in the
      usual mixed mode. Pick WPA3 only when the network is WPA3 exclusively, because some older
      phones do not recognise that code.
    </p>
    <p className="mt-2">
      The code that appears on screen scans straight from the screen, so you can test it before you
      print anything. Download PNG gives you a file at 8 pixels per module for printing. Keep the
      white border: that empty margin is part of the code and a camera cannot find the pattern
      without it.
    </p>
    <p className="mt-2">
      Anyone who can see the code can join the network, exactly like anyone who can read the
      password written on a whiteboard. Put a guest network on the poster, not the one the servers
      are on. CHIT does not save the password anywhere: close the tool and it is gone.
    </p>
  </>
)

export default function WifiQrPage() {
  const [kind, setKind] = useState('wifi')
  const [ssid, setSsid] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [security, setSecurity] = useState('WPA')
  const [hidden, setHidden] = useState(false)
  const [text, setText] = useState('')
  const [ecLevel, setEcLevel] = useState('M')

  const [code, setCode] = useState<QrCodeData | null>(null)
  const [error, setError] = useState<string>()
  const [ssidError, setSsidError] = useState<string>()
  const [passwordError, setPasswordError] = useState<string>()
  const [textError, setTextError] = useState<string>()

  // The code on screen must always match the fields above it, so any change to
  // what is being encoded throws the old one away.
  const clear = () => {
    setCode(null)
    setError(undefined)
  }

  const path = useMemo(
    () => (code === null ? '' : modulesToPath(code.modules, code.size, code.quiet)),
    [code],
  )

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setSsidError(undefined)
    setPasswordError(undefined)
    setTextError(undefined)

    if (kind === 'wifi') {
      if (ssid.trim() === '') {
        setSsidError('Type the network name exactly as it appears on the router, capital letters and all.')
        return
      }
      if (security !== 'nopass' && password === '') {
        setPasswordError(
          'Type the Wi-Fi password, or change Security to "Open, no password" if the network does not have one.',
        )
        return
      }
    } else if (text.trim() === '') {
      setTextError('Type or paste the text or link to put in the code.')
      return
    }

    try {
      setCode(await generateQr({ mode: kind, text, ssid, password, security, hidden, ecLevel }))
      setError(undefined)
    } catch (err) {
      setCode(null)
      setError(errorMessage(err))
    }
  }

  const emptyMessage =
    kind === 'wifi'
      ? 'Fill in the network name and password, then press Generate. The code appears here.'
      : 'Type or paste a link, then press Generate. The code appears here.'

  return (
    <ToolShell
      title="Wi-Fi QR Code Generator"
      description="Turn a Wi-Fi network or a link into a QR code guests can scan."
      help={HELP}
    >
      <div className="flex flex-col gap-4">
        <div className="grid gap-4 md:grid-cols-[minmax(0,24rem)_minmax(0,1fr)]">
          <form className="flex flex-col gap-3" onSubmit={onSubmit}>
            <Select
              label="What is the code for?"
              options={KINDS}
              value={kind}
              onChange={(event) => {
                setKind(event.target.value)
                clear()
              }}
            />

            {kind === 'wifi' ? (
              <>
                <TextInput
                  label="Network name (SSID)"
                  placeholder="Guest-WiFi"
                  autoFocus
                  spellCheck={false}
                  autoComplete="off"
                  value={ssid}
                  onChange={(event) => setSsid(event.target.value)}
                  error={ssidError}
                  hint="Type it exactly as it appears on the router, capital letters and all."
                />

                {security !== 'nopass' && (
                  <div className="flex items-end gap-2">
                    <div className="min-w-0 flex-1">
                      <TextInput
                        label="Password"
                        placeholder="the Wi-Fi password"
                        spellCheck={false}
                        autoComplete="off"
                        className="font-mono"
                        type={showPassword ? 'text' : 'password'}
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        error={passwordError}
                      />
                    </div>
                    <Button
                      variant="ghost"
                      aria-label={showPassword ? 'Hide the password' : 'Show the password'}
                      onClick={() => setShowPassword(!showPassword)}
                      icon={showPassword ? <EyeOff size={14} aria-hidden /> : <Eye size={14} aria-hidden />}
                    />
                  </div>
                )}

                <Select
                  label="Security"
                  options={SECURITY}
                  value={security}
                  onChange={(event) => {
                    setSecurity(event.target.value)
                    clear()
                  }}
                  hint="Leave this on WPA / WPA2 unless the network refuses anything older. Some phones do not understand the WPA3 code."
                />

                <label className="flex items-center gap-2 text-sm text-fg">
                  <input
                    type="checkbox"
                    className="size-4 accent-[var(--accent)]"
                    checked={hidden}
                    onChange={(event) => setHidden(event.target.checked)}
                  />
                  Network is hidden (it does not broadcast its name)
                </label>
              </>
            ) : (
              <Textarea
                id="wifi-qr-text"
                label="Text or link"
                hint="Anything you type goes in exactly as typed, including spaces."
                error={textError}
                rows={4}
                spellCheck={false}
                placeholder="https://helpdesk.example.com"
                value={text}
                onChange={(event) => setText(event.target.value)}
              />
            )}

            <Select
              label="Error correction"
              options={LEVELS}
              value={ecLevel}
              onChange={(event) => {
                setEcLevel(event.target.value)
                clear()
              }}
              hint="Higher settings still scan when the sticker is scuffed, at the cost of a bigger code."
            />

            <div>
              <Button type="submit" variant="primary" icon={<QrCode size={14} aria-hidden />}>
                Generate
              </Button>
            </div>

            {error !== undefined && (
              <p role="alert" className="mt-2 text-xs text-danger">
                {error}
              </p>
            )}
          </form>

          {code === null ? (
            <div className="rounded border border-dashed border-border px-4 py-8 text-center text-sm text-fg-muted">
              {emptyMessage}
            </div>
          ) : (
            <div className="rounded border border-border bg-surface-2 p-4">
              <div className="mx-auto w-full max-w-64 rounded bg-white p-3">
                <svg
                  viewBox={`0 0 ${code.size + code.quiet * 2} ${code.size + code.quiet * 2}`}
                  className="h-auto w-full"
                  shapeRendering="crispEdges"
                  role="img"
                  aria-label={`QR code, ${code.size} by ${code.size} modules`}
                >
                  <rect
                    width={code.size + code.quiet * 2}
                    height={code.size + code.quiet * 2}
                    className="fill-white"
                  />
                  <path d={path} className="fill-black" />
                </svg>
              </div>

              <p className="mt-3 text-center text-xs text-fg-muted">
                Version {code.version}, {code.size} x {code.size} modules, error correction{' '}
                {code.ecLevel}. {code.payloadBytes} of {code.capacity} bytes used.
              </p>

              <div className="mt-3 flex flex-wrap justify-center gap-2">
                <Button
                  icon={<Download size={14} aria-hidden />}
                  onClick={() =>
                    downloadPng(
                      code,
                      `${downloadName(kind, kind === 'wifi' ? ssid : text)}.png`,
                      SCALE,
                    )
                  }
                >
                  Download PNG
                </Button>
                <CopyButton value={code.payload} label="Copy the text behind the code" />
              </div>

              <details className="mt-3 rounded border border-border bg-surface-3 px-3 py-2">
                <summary className="cursor-pointer text-xs font-medium text-fg-muted">
                  What is in the code
                </summary>
                <p className="mt-2 font-mono text-xs break-all text-fg">{code.payload}</p>
                {kind === 'wifi' && (
                  <p className="mt-2 text-xs text-warn">
                    This includes the Wi-Fi password in plain text. Anyone who scans the code, or
                    reads this line, has the password.
                  </p>
                )}
              </details>
            </div>
          )}
        </div>
      </div>
    </ToolShell>
  )
}
