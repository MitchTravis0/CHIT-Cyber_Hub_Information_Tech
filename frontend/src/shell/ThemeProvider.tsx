import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { readDoc, writeDoc } from './bindings'

export type AccentId = 'red' | 'cyan' | 'violet' | 'amber' | 'green' | 'magenta'

export interface AccentOption {
  id: AccentId
  label: string
  /** Same colour as the matching rule in style.css, used only for the swatch. */
  swatch: string
}

export const ACCENTS: readonly AccentOption[] = [
  { id: 'red', label: 'Red', swatch: '#ff3b4b' },
  { id: 'cyan', label: 'Cyan', swatch: '#22d3ee' },
  { id: 'violet', label: 'Violet', swatch: '#a78bfa' },
  { id: 'amber', label: 'Amber', swatch: '#fbbf24' },
  { id: 'green', label: 'Green', swatch: '#34d399' },
  { id: 'magenta', label: 'Magenta', swatch: '#f472b6' },
]

const DEFAULT_ACCENT: AccentId = 'red'

function isAccentId(value: unknown): value is AccentId {
  return ACCENTS.some((accent) => accent.id === value)
}

const SETTINGS_NAMESPACE = 'settings'
const SETTINGS_VERSION = 2

interface SettingsDoc {
  version: number
  portableMode: boolean
  accent: AccentId
}

interface ThemeValue {
  accent: AccentId
  setAccent: (accent: AccentId) => void
}

const ThemeContext = createContext<ThemeValue>({
  accent: DEFAULT_ACCENT,
  setAccent: () => {},
})

export function useTheme(): ThemeValue {
  return useContext(ThemeContext)
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [accent, setAccentState] = useState<AccentId>(DEFAULT_ACCENT)
  // portableMode belongs to the settings document (C4) but is decided by the
  // portable.txt marker, so it is carried through writes rather than set here.
  const portableRef = useRef(false)

  useEffect(() => {
    let cancelled = false
    readDoc<SettingsDoc>(SETTINGS_NAMESPACE)
      .then((doc) => {
        if (cancelled || !doc) return
        portableRef.current = doc.portableMode === true
        // A version 1 document also held a light/dark/system choice. The app is
        // dark only now, so that field is ignored and dropped on the next write.
        if (isAccentId(doc.accent)) setAccentState(doc.accent)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  // Layout effect so the attribute lands before the first paint, otherwise the
  // window flashes the default red before the saved accent arrives.
  useLayoutEffect(() => {
    document.documentElement.dataset.accent = accent
  }, [accent])

  const setAccent = useCallback((next: AccentId) => {
    setAccentState(next)
    const doc: SettingsDoc = {
      version: SETTINGS_VERSION,
      portableMode: portableRef.current,
      accent: next,
    }
    writeDoc(SETTINGS_NAMESPACE, doc).catch(() => {})
  }, [])

  return <ThemeContext.Provider value={{ accent, setAccent }}>{children}</ThemeContext.Provider>
}
