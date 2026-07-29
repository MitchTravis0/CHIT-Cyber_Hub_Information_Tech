import { useCallback, useEffect, useRef, useState } from 'react'
import { readDoc, writeDoc } from './bindings'

const PREFS_NAMESPACE = 'shell'
// Version 2 added tourSeen. A version 1 document has no such field, which reads
// as false, so an existing install sees the first-run tour once. That is the
// intent: the tour is new information, not a repeat.
const PREFS_VERSION = 2
const MAX_RECENTS = 6

interface PrefsDoc {
  version: number
  favorites: string[]
  recents: string[]
  sidebarCollapsed: boolean
  tourSeen: boolean
}

interface PrefsState {
  favorites: string[]
  recents: string[]
  sidebarCollapsed: boolean
  tourSeen: boolean
}

export interface Prefs extends PrefsState {
  /** False until the stored document has been read, so the tour cannot flash
   *  open for the moment before the saved answer arrives. */
  loaded: boolean
  toggleFavorite: (toolId: string) => void
  noteVisit: (toolId: string) => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setTourSeen: (seen: boolean) => void
}

const EMPTY: PrefsState = {
  favorites: [],
  recents: [],
  sidebarCollapsed: false,
  tourSeen: false,
}

/** Favourite and recently opened tools, persisted in the "shell" namespace. */
export function usePrefs(): Prefs {
  const [state, setState] = useState<PrefsState>(EMPTY)
  const [loaded, setLoaded] = useState(false)
  const stateRef = useRef<PrefsState>(EMPTY)

  useEffect(() => {
    let cancelled = false
    readDoc<PrefsDoc>(PREFS_NAMESPACE)
      .then((doc) => {
        if (cancelled) return
        if (doc) {
          const saved: PrefsState = {
            favorites: Array.isArray(doc.favorites) ? doc.favorites : [],
            recents: Array.isArray(doc.recents) ? doc.recents : [],
            sidebarCollapsed: doc.sidebarCollapsed === true,
            tourSeen: doc.tourSeen === true,
          }
          stateRef.current = saved
          setState(saved)
        }
        setLoaded(true)
      })
      // A store that cannot be read still has to let the app through, otherwise
      // the tour would never open and the shell would sit on its defaults.
      .catch(() => {
        if (!cancelled) setLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const apply = useCallback((next: PrefsState) => {
    stateRef.current = next
    setState(next)
    const doc: PrefsDoc = { version: PREFS_VERSION, ...next }
    writeDoc(PREFS_NAMESPACE, doc).catch(() => {})
  }, [])

  const toggleFavorite = useCallback(
    (toolId: string) => {
      const prev = stateRef.current
      apply({
        ...prev,
        favorites: prev.favorites.includes(toolId)
          ? prev.favorites.filter((id) => id !== toolId)
          : prev.favorites.concat(toolId),
      })
    },
    [apply],
  )

  const noteVisit = useCallback(
    (toolId: string) => {
      const prev = stateRef.current
      if (prev.recents[0] === toolId) return
      apply({
        ...prev,
        recents: [toolId, ...prev.recents.filter((id) => id !== toolId)].slice(0, MAX_RECENTS),
      })
    },
    [apply],
  )

  const setSidebarCollapsed = useCallback(
    (collapsed: boolean) => {
      apply({ ...stateRef.current, sidebarCollapsed: collapsed })
    },
    [apply],
  )

  const setTourSeen = useCallback(
    (seen: boolean) => {
      apply({ ...stateRef.current, tourSeen: seen })
    },
    [apply],
  )

  return {
    favorites: state.favorites,
    recents: state.recents,
    sidebarCollapsed: state.sidebarCollapsed,
    tourSeen: state.tourSeen,
    loaded,
    toggleFavorite,
    noteVisit,
    setSidebarCollapsed,
    setTourSeen,
  }
}
