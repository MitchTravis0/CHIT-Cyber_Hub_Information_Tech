import { useMemo } from 'react'
import { focusRing, SidebarToggle } from '../components'
import { cn } from '../lib/format'
import { TOOLS, type ToolDef } from '../tools/registry'
import { Icon } from './Icon'
import { RadialNav } from './RadialNav'
import { MOD_LABEL, resolveTools, toolGroups, type View } from './nav'

interface HomePageProps {
  onNavigate: (view: View) => void
  onOpenPalette: () => void
  favorites: string[]
  recents: string[]
  onToggleFavorite: (toolId: string) => void
}

export function HomePage({
  onNavigate,
  onOpenPalette,
  favorites,
  recents,
  onToggleFavorite,
}: HomePageProps) {
  const groups = useMemo(() => toolGroups(), [])
  const favoriteTools = useMemo(() => resolveTools(favorites), [favorites])
  const recentTools = useMemo(() => resolveTools(recents), [recents])

  return (
    <div className="home-grid min-h-full">
      <div className="mx-auto max-w-5xl px-8 py-8">
        <div className="flex items-center gap-2">
          <SidebarToggle className="-ml-1.5" />
          <button
            type="button"
            onClick={onOpenPalette}
            className={cn(
              'flex w-full items-center gap-2 rounded border border-border bg-surface-2 px-3 py-2 text-left text-sm text-fg-muted transition-colors hover:border-accent hover:text-fg',
              focusRing,
            )}
          >
            <Icon name="Search" size={16} aria-hidden />
            <span className="flex-1">Search tools</span>
            <kbd className="rounded border border-border px-1 py-px font-display text-[11px]">
              {MOD_LABEL} K
            </kbd>
          </button>
        </div>

        {TOOLS.length === 0 ? (
          <section className="corner-cut mt-8 border border-dashed border-border bg-surface-2 p-8 text-center">
            <span className="mx-auto flex h-10 w-10 items-center justify-center rounded bg-surface-3 text-fg-muted">
              <Icon name="PackageOpen" size={20} aria-hidden />
            </span>
            <h2 className="mt-3 text-sm font-medium">No tools in this build yet</h2>
            <p className="mx-auto mt-1 max-w-md text-sm text-fg-muted">
              The shell is ready and waiting. As tools are added they show up here and in the
              sidebar, grouped by what they are for.
            </p>
          </section>
        ) : (
          <>
            <ChipRow
              title="Favourites"
              icon="Star"
              tools={favoriteTools}
              favorites={favorites}
              onNavigate={onNavigate}
              onToggleFavorite={onToggleFavorite}
            />
            <ChipRow
              title="Recent"
              icon="Clock"
              tools={recentTools}
              favorites={favorites}
              onNavigate={onNavigate}
              onToggleFavorite={onToggleFavorite}
            />

            <div className="mt-6">
              <RadialNav onNavigate={onNavigate} />
            </div>

            <details className="group mt-8 border-t border-border pt-3">
              <summary
                className={cn(
                  'flex cursor-pointer list-none items-center gap-1.5 font-display text-[11px] font-medium tracking-wide text-fg-muted uppercase select-none hover:text-fg [&::-webkit-details-marker]:hidden',
                  focusRing,
                )}
              >
                <Icon
                  name="ChevronRight"
                  size={13}
                  aria-hidden
                  className="shrink-0 transition-transform group-open:rotate-90"
                />
                All tools ({TOOLS.length})
              </summary>
              {groups.map((group) => (
                <section key={group.category} className="mt-4">
                  <h2 className="font-display text-[11px] font-medium tracking-wide text-fg-muted uppercase">
                    {group.label}
                  </h2>
                  <ul className="mt-1.5 grid gap-1 sm:grid-cols-2">
                    {group.tools.map((tool) => (
                      <ToolRow
                        key={tool.id}
                        tool={tool}
                        favorite={favorites.includes(tool.id)}
                        onOpen={() => onNavigate({ kind: 'tool', id: tool.id })}
                        onToggleFavorite={() => onToggleFavorite(tool.id)}
                      />
                    ))}
                  </ul>
                </section>
              ))}
            </details>
          </>
        )}
      </div>
    </div>
  )
}

interface ChipRowProps {
  title: string
  icon: string
  tools: ToolDef[]
  favorites: string[]
  onNavigate: (view: View) => void
  onToggleFavorite: (toolId: string) => void
}

function ChipRow({ title, icon, tools, favorites, onNavigate, onToggleFavorite }: ChipRowProps) {
  if (tools.length === 0) return null

  return (
    <section className="mt-5 flex flex-wrap items-center gap-1.5">
      <h2 className="mr-1 flex items-center gap-1.5 font-display text-[11px] font-medium tracking-wide text-fg-muted uppercase">
        <Icon name={icon} size={13} aria-hidden />
        {title}
      </h2>
      {tools.map((tool) => (
        <span
          key={tool.id}
          className="inline-flex items-center rounded border border-border bg-surface-2"
        >
          <button
            type="button"
            onClick={() => onNavigate({ kind: 'tool', id: tool.id })}
            className={cn(
              'flex items-center gap-1.5 rounded py-1 pr-1.5 pl-2 text-xs text-fg-muted transition-colors hover:text-fg',
              focusRing,
            )}
          >
            <Icon name={tool.icon} size={13} aria-hidden className="text-accent" />
            {tool.name}
          </button>
          <FavouriteToggle
            tool={tool}
            favorite={favorites.includes(tool.id)}
            onToggle={() => onToggleFavorite(tool.id)}
            className="rounded py-1 pr-1.5 pl-0.5"
          />
        </span>
      ))}
    </section>
  )
}

interface ToolRowProps {
  tool: ToolDef
  favorite: boolean
  onOpen: () => void
  onToggleFavorite: () => void
}

function ToolRow({ tool, favorite, onOpen, onToggleFavorite }: ToolRowProps) {
  return (
    <li className="flex items-center gap-1">
      <button
        type="button"
        onClick={onOpen}
        className={cn(
          'flex min-w-0 flex-1 items-center gap-2 rounded border-l-2 border-l-transparent px-2 py-1 text-left text-[13px] transition-colors hover:border-l-accent hover:bg-surface-2',
          focusRing,
        )}
      >
        <Icon name={tool.icon} size={14} aria-hidden className="shrink-0 text-accent" />
        <span className="shrink-0">{tool.name}</span>
        <span className="truncate text-xs text-fg-muted">{tool.description}</span>
      </button>
      <FavouriteToggle
        tool={tool}
        favorite={favorite}
        onToggle={onToggleFavorite}
        className="rounded p-1"
      />
    </li>
  )
}

function FavouriteToggle({
  tool,
  favorite,
  onToggle,
  className,
}: {
  tool: ToolDef
  favorite: boolean
  onToggle: () => void
  className?: string
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={favorite}
      aria-label={favorite ? `Remove ${tool.name} from favourites` : `Add ${tool.name} to favourites`}
      title={favorite ? 'Remove from favourites' : 'Add to favourites'}
      className={cn(
        'shrink-0 transition-colors',
        favorite ? 'text-warn' : 'text-fg-muted hover:text-fg',
        focusRing,
        className,
      )}
    >
      <Icon name="Star" size={13} aria-hidden fill={favorite ? 'currentColor' : 'none'} />
    </button>
  )
}
