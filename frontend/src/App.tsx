import {
  Component as ReactComponent,
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ComponentType,
  type LazyExoticComponent,
  type ReactNode,
} from 'react'
import { focusRing } from './components'
import { cn } from './lib/format'
import { SidebarProvider } from './lib/sidebar'
import { CommandPalette } from './shell/CommandPalette'
import { FirstRunTour } from './shell/FirstRunTour'
import { HomePage } from './shell/HomePage'
import { Icon } from './shell/Icon'
import { SettingsPage } from './shell/SettingsPage'
import { Sidebar } from './shell/Sidebar'
import { findTool, type View } from './shell/nav'
import { usePrefs } from './shell/prefs'
import type { ToolDef } from './tools/registry'

// Lazy components are cached so returning to a tool does not re-import it.
const loaded = new Map<string, LazyExoticComponent<ComponentType>>()

function toolComponent(tool: ToolDef): LazyExoticComponent<ComponentType> {
  const cached = loaded.get(tool.id)
  if (cached) return cached
  const component = lazy(tool.element)
  loaded.set(tool.id, component)
  return component
}

function App() {
  const [view, setView] = useState<View>({ kind: 'home' })
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [tourOpen, setTourOpen] = useState(false)
  const {
    favorites,
    recents,
    sidebarCollapsed,
    tourSeen,
    loaded,
    toggleFavorite,
    noteVisit,
    setSidebarCollapsed,
    setTourSeen,
  } = usePrefs()

  // Only once the stored answer has arrived, so the tour cannot flash open on a
  // machine that has already dismissed it.
  useEffect(() => {
    if (loaded && !tourSeen) setTourOpen(true)
  }, [loaded, tourSeen])

  const closeTour = useCallback(() => {
    setTourOpen(false)
    setTourSeen(true)
  }, [setTourSeen])

  const navigate = useCallback(
    (next: View) => {
      setView(next)
      if (next.kind === 'tool') noteVisit(next.id)
    },
    [noteVisit],
  )

  const sidebar = useMemo(
    () => ({ hidden: sidebarCollapsed, toggle: () => setSidebarCollapsed(!sidebarCollapsed) }),
    [sidebarCollapsed, setSidebarCollapsed],
  )

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() === 'k' && (event.ctrlKey || event.metaKey)) {
        event.preventDefault()
        setPaletteOpen((open) => !open)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  return (
    <SidebarProvider value={sidebar}>
      <div className="flex h-full min-h-0 bg-surface text-fg">
        {!sidebarCollapsed && (
          <Sidebar view={view} onNavigate={navigate} onOpenPalette={() => setPaletteOpen(true)} />
        )}
        <main className="min-w-0 flex-1 overflow-y-auto">
          {view.kind === 'home' && (
            <HomePage
              onNavigate={navigate}
              onOpenPalette={() => setPaletteOpen(true)}
              favorites={favorites}
              recents={recents}
              onToggleFavorite={toggleFavorite}
            />
          )}
          {view.kind === 'settings' && <SettingsPage onShowTour={() => setTourOpen(true)} />}
          {view.kind === 'tool' && <ToolView id={view.id} onNavigate={navigate} />}
        </main>
        <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} onSelect={navigate} />
        <FirstRunTour open={tourOpen} onClose={closeTour} />
      </div>
    </SidebarProvider>
  )
}

function ToolView({ id, onNavigate }: { id: string; onNavigate: (view: View) => void }) {
  const tool = findTool(id)

  if (!tool) {
    return (
      <div className="mx-auto max-w-3xl px-8 py-16 text-center">
        <span className="mx-auto flex h-10 w-10 items-center justify-center rounded bg-surface-2 text-fg-muted">
          <Icon name="CircleHelp" size={20} />
        </span>
        <h1 className="mt-3 text-sm font-medium">That tool is not in this build</h1>
        <p className="mt-1 text-sm text-fg-muted">It may have been renamed or removed.</p>
        <button
          type="button"
          onClick={() => onNavigate({ kind: 'home' })}
          className={cn(
            'mt-4 rounded border border-border bg-surface-2 px-3 py-1.5 text-sm hover:border-accent',
            focusRing,
          )}
        >
          Back to home
        </button>
      </div>
    )
  }

  const Component = toolComponent(tool)
  return (
    <ToolBoundary key={tool.id} name={tool.name}>
      <Suspense
        fallback={
          <div className="px-8 py-16 text-center text-sm text-fg-muted">Loading {tool.name}...</div>
        }
      >
        <Component />
      </Suspense>
    </ToolBoundary>
  )
}

// Without this, a tool that throws while loading or rendering takes the whole
// window with it and the tech sees a blank app with no way back.
class ToolBoundary extends ReactComponent<
  { name: string; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div className="mx-auto max-w-3xl px-8 py-16 text-center">
        <span className="mx-auto flex h-10 w-10 items-center justify-center rounded bg-surface-2 text-danger">
          <Icon name="TriangleAlert" size={20} />
        </span>
        <h1 className="mt-3 text-sm font-medium">{this.props.name} stopped working</h1>
        <p className="mt-1 text-sm text-fg-muted">
          Nothing else in CHIT is affected. Pick another tool from the sidebar, or reopen this one
          to try again.
        </p>
      </div>
    )
  }
}

export default App
