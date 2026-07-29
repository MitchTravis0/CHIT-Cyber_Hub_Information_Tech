import { useMemo } from 'react'
import { focusRing } from '../components'
import { cn } from '../lib/format'
import { TOOLS } from '../tools/registry'
import { Icon } from './Icon'
import { MOD_LABEL, toolGroups, type View } from './nav'

interface SidebarProps {
  view: View
  onNavigate: (view: View) => void
  onOpenPalette: () => void
}

/** The side menu. Hiding it is App's job: it simply stops rendering this, and
 *  the toggle that brings it back lives in every page header. */
export function Sidebar({ view, onNavigate, onOpenPalette }: SidebarProps) {
  const groups = useMemo(() => toolGroups(), [])

  return (
    <aside className="flex h-full w-60 shrink-0 flex-col border-r border-border bg-surface-2">
      <div className="flex items-center gap-2.5 border-t-2 border-t-accent px-4 pt-4 pb-3">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded bg-accent text-accent-fg">
          <Icon name="Wrench" size={17} />
        </span>
        <span className="min-w-0 flex-1 leading-tight">
          <span className="block font-display text-sm font-semibold tracking-wide">CHIT</span>
          <span className="block text-xs text-fg-muted">IT toolbox</span>
        </span>
      </div>

      <div className="px-3 pb-3">
        <button
          type="button"
          onClick={onOpenPalette}
          className={cn(
            'flex w-full items-center gap-2 rounded border border-border bg-surface px-2.5 py-1.5 text-left text-xs text-fg-muted transition-colors hover:border-accent hover:text-fg',
            focusRing,
          )}
        >
          <Icon name="Search" size={14} />
          <span className="flex-1">Search tools</span>
          <kbd className="rounded border border-border px-1 py-px font-display text-[10px] text-fg-muted">
            {MOD_LABEL} K
          </kbd>
        </button>
      </div>

      <nav className="min-h-0 flex-1 overflow-y-auto px-2 pb-4">
        <NavItem
          icon="House"
          label="Home"
          active={view.kind === 'home'}
          onClick={() => onNavigate({ kind: 'home' })}
        />

        {groups.map((group) => (
          <div key={group.category} className="mt-4">
            <p className="px-2 pb-1 font-display text-[11px] font-medium tracking-wide text-fg-muted uppercase">
              {group.label}
            </p>
            {group.tools.map((tool) => (
              <NavItem
                key={tool.id}
                icon={tool.icon}
                label={tool.name}
                active={view.kind === 'tool' && view.id === tool.id}
                onClick={() => onNavigate({ kind: 'tool', id: tool.id })}
              />
            ))}
          </div>
        ))}

        {TOOLS.length === 0 && (
          <p className="px-2 pt-6 text-xs leading-relaxed text-fg-muted">
            No tools are installed in this build yet. They will appear here, grouped by category.
          </p>
        )}
      </nav>

      <div className="border-t border-border p-2">
        <NavItem
          icon="Settings"
          label="Settings"
          active={view.kind === 'settings'}
          onClick={() => onNavigate({ kind: 'settings' })}
        />
      </div>
    </aside>
  )
}

interface NavItemProps {
  icon: string
  label: string
  active: boolean
  onClick: () => void
}

function NavItem({ icon, label, active, onClick }: NavItemProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex w-full items-center gap-2.5 rounded border-l-2 px-2 py-1.5 text-left text-[13px] transition-colors',
        active
          ? 'border-l-accent bg-surface-3 font-medium text-fg'
          : 'border-l-transparent text-fg-muted hover:bg-surface-3 hover:text-fg',
        focusRing,
      )}
    >
      <Icon name={icon} size={15} className={cn('shrink-0', active && 'text-accent')} />
      <span className="truncate">{label}</span>
    </button>
  )
}
