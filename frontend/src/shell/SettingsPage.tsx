import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Button, SidebarToggle } from '../components'
import { cn, errorMessage } from '../lib/format'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { checkForUpdate, getAppInfo, type AppInfo, type UpdateResult } from './bindings'
import { Icon } from './Icon'
import { MOD_LABEL } from './nav'
import { ACCENTS, useTheme } from './ThemeProvider'

export function SettingsPage({ onShowTour }: { onShowTour: () => void }) {
  const { accent, setAccent } = useTheme()
  const [info, setInfo] = useState<AppInfo | null>(null)
  const [update, setUpdate] = useState<UpdateResult | null>(null)
  const [updateError, setUpdateError] = useState<string>()
  const [checking, setChecking] = useState(false)
  const accentGroup = useRef<HTMLDivElement>(null)

  const onCheck = async () => {
    setChecking(true)
    try {
      setUpdate(await checkForUpdate())
      setUpdateError(undefined)
    } catch (err) {
      setUpdate(null)
      setUpdateError(errorMessage(err))
    } finally {
      setChecking(false)
    }
  }

  // A radio group is one stop on the Tab key, and the arrows move inside it.
  // Without this all six swatches sit in the tab order and a keyboard user has
  // to walk past every colour to reach the rest of the page.
  const onAccentKeys = (event: KeyboardEvent<HTMLDivElement>) => {
    const last = ACCENTS.length - 1
    const current = ACCENTS.findIndex((option) => option.id === accent)
    let next: number
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        next = current === last ? 0 : current + 1
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        next = current === 0 ? last : current - 1
        break
      case 'Home':
        next = 0
        break
      case 'End':
        next = last
        break
      default:
        return
    }
    event.preventDefault()
    setAccent(ACCENTS[next].id)
    accentGroup.current?.querySelectorAll('button')[next]?.focus()
  }

  useEffect(() => {
    let cancelled = false
    getAppInfo()
      .then((loaded) => {
        if (!cancelled) setInfo(loaded)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="mx-auto max-w-3xl px-8 py-8">
      <header className="flex items-start gap-2">
        <SidebarToggle className="-ml-1.5 mt-1" />
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
          <p className="mt-1 text-sm text-fg-muted">
            How the app looks and where it keeps your data.
          </p>
        </div>
      </header>

      <Section title="Appearance" icon="Palette">
        <Row
          label="Accent colour"
          hint="Changes the highlight colour used across the whole app. It is saved, so it comes back the next time you open CHIT."
        >
          <div
            ref={accentGroup}
            role="radiogroup"
            aria-label="Accent colour"
            onKeyDown={onAccentKeys}
            className="flex flex-wrap gap-1.5"
          >
            {ACCENTS.map((option) => (
              <button
                key={option.id}
                type="button"
                role="radio"
                aria-checked={accent === option.id}
                tabIndex={accent === option.id ? 0 : -1}
                onClick={() => setAccent(option.id)}
                className={cn(
                  'focus-glow flex items-center gap-1.5 rounded border px-2 py-1.5 text-xs transition-colors',
                  accent === option.id
                    ? 'border-accent bg-accent-dim text-fg'
                    : 'border-border bg-surface-2 text-fg-muted hover:text-fg',
                )}
              >
                <span
                  aria-hidden
                  className="size-3 shrink-0 rounded-full"
                  style={{ backgroundColor: option.swatch }}
                />
                {option.label}
              </button>
            ))}
          </div>
        </Row>
      </Section>

      <Section title="Data" icon="Database">
        <Row
          label="Storage folder"
          hint="Settings, favourites and saved tool data live here. Copy this folder to back it up."
        >
          <code className="block break-all rounded border border-border bg-surface-2 px-2 py-1.5 text-xs">
            {info?.storeDir ?? 'Not available outside the app'}
          </code>
        </Row>
        <Row
          label="Portable mode"
          hint={
            info?.portable
              ? 'Data is stored next to the executable, so the app travels with a USB stick.'
              : `Data is stored in this machine's config folder. Put a file named portable.txt next to the executable to store it alongside the app instead.`
          }
        >
          <span
            className={cn(
              'inline-flex items-center gap-1.5 rounded border px-2 py-1 text-xs',
              info?.portable ? 'border-ok text-ok' : 'border-border text-fg-muted',
            )}
          >
            <Icon name={info?.portable ? 'Check' : 'Minus'} size={13} />
            {info?.portable ? 'On' : 'Off'}
          </span>
        </Row>
      </Section>

      <Section title="Keyboard" icon="Keyboard">
        <Row label="Search tools" hint="Works from any page.">
          <kbd className="rounded border border-border bg-surface-2 px-1.5 py-1 text-xs">
            {MOD_LABEL} K
          </kbd>
        </Row>
      </Section>

      <Section title="Help" icon="Compass">
        <Row
          label="Show the welcome tour"
          hint="Five short pages covering how to find a tool, favourites, the three tools that open a port on this machine, and where your data is kept."
        >
          <Button onClick={onShowTour} icon={<Icon name="Compass" size={14} aria-hidden />}>
            Show the tour
          </Button>
        </Row>
      </Section>

      <Section title="About" icon="Info">
        <Row label="Version">
          <span className="text-sm">{info?.version ?? '-'}</span>
        </Row>
        <Row label="Platform">
          <span className="text-sm">{info?.platform ?? '-'}</span>
        </Row>
        <Row
          label="Updates"
          hint="CHIT only asks GitHub when you press this button, and it never updates itself. If a newer version exists you download the file and replace this one."
        >
          <div className="flex flex-col items-start gap-2">
            <Button
              onClick={onCheck}
              disabled={checking}
              icon={<Icon name={checking ? 'LoaderCircle' : 'RefreshCw'} size={14} aria-hidden />}
            >
              {checking ? 'Checking...' : 'Check for updates'}
            </Button>
            {updateError && (
              <p role="alert" className="text-xs leading-relaxed text-danger">
                {updateError}
              </p>
            )}
            {update && (
              <div className="text-xs leading-relaxed text-fg-muted">
                <p className={cn(update.newer && 'text-fg')}>{update.note}</p>
                {update.newer && (
                  <button
                    type="button"
                    onClick={() => BrowserOpenURL(update.url)}
                    className="focus-glow mt-1 rounded text-accent underline underline-offset-2"
                  >
                    Open the download page
                    {update.published !== '' && ` (released ${update.published})`}
                  </button>
                )}
              </div>
            )}
          </div>
        </Row>
      </Section>
    </div>
  )
}

function Section({ title, icon, children }: { title: string; icon: string; children: ReactNode }) {
  return (
    <section className="mt-8">
      <h2 className="flex items-center gap-2 font-display text-xs font-medium tracking-wide uppercase">
        <Icon name={icon} size={15} className="text-fg-muted" />
        {title}
      </h2>
      <div className="mt-3 divide-y divide-border rounded border border-border">{children}</div>
    </section>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 p-4 sm:flex-row sm:items-start sm:justify-between sm:gap-8">
      <div className="min-w-0">
        <p className="text-sm">{label}</p>
        {hint && <p className="mt-0.5 text-xs leading-relaxed text-fg-muted">{hint}</p>}
      </div>
      <div className="min-w-0 shrink-0 sm:max-w-[60%]">{children}</div>
    </div>
  )
}
