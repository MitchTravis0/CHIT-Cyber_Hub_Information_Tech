import { Command } from 'cmdk'
import { useMemo, useState } from 'react'
import { TOOLS } from '../tools/registry'
import { Icon } from './Icon'
import { paletteEntries, rankEntries, type Entry } from './search'
import { type View } from './nav'

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelect: (view: View) => void
}

export function CommandPalette({ open, onOpenChange, onSelect }: CommandPaletteProps) {
  const entries = useMemo(() => paletteEntries(), [])
  const [query, setQuery] = useState('')
  const ranked = useMemo(() => rankEntries(entries, query), [entries, query])

  const go = (view: View) => {
    onOpenChange(false)
    onSelect(view)
  }

  const change = (next: boolean) => {
    if (!next) setQuery('')
    onOpenChange(next)
  }

  return (
    <Command.Dialog
      open={open}
      onOpenChange={change}
      label="Search tools"
      loop
      // cmdk's own matcher accepts a scattered subsequence, so "netcat" matched
      // 58 of these 59 entries by way of "subNET CAlculaTor". search.ts does the
      // filtering and the ordering instead, and the list is rendered flat: the
      // category groups this used to render were what buried the best match.
      shouldFilter={false}
      overlayClassName="fixed inset-0 z-40 bg-black/60"
      contentClassName="corner-cut fixed left-1/2 top-[12vh] z-50 w-[min(38rem,calc(100vw-2rem))] -translate-x-1/2 overflow-hidden rounded border border-t-2 border-border border-t-accent bg-surface shadow-2xl"
    >
      <div className="flex items-center gap-2 border-b border-border px-3">
        <Icon name="Search" size={16} className="shrink-0 text-fg-muted" />
        <Command.Input
          value={query}
          onValueChange={setQuery}
          placeholder="Search tools by name, job or keyword"
          className="h-11 w-full bg-transparent text-sm text-fg outline-none placeholder:text-fg-muted"
        />
      </div>

      <Command.List className="max-h-[22rem] overflow-y-auto p-2">
        <Command.Empty className="px-3 py-8 text-center text-sm text-fg-muted">
          {TOOLS.length === 0
            ? 'No tools are installed in this build yet.'
            : 'Nothing matched. Try a plainer word, such as ping, dns or password.'}
        </Command.Empty>

        {ranked.map((entry) => (
          <PaletteItem key={entry.id} entry={entry} onSelect={() => go(entry.view)} />
        ))}
      </Command.List>

      <div className="flex items-center gap-4 border-t border-border px-3 py-2 text-[11px] text-fg-muted">
        <span>Up and down to move</span>
        <span>Enter to open</span>
        <span>Esc to close</span>
      </div>
    </Command.Dialog>
  )
}

function PaletteItem({ entry, onSelect }: { entry: Entry; onSelect: () => void }) {
  return (
    <Command.Item
      value={entry.id}
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-2.5 rounded border-l-2 border-l-transparent px-2 py-2 text-sm text-fg data-[selected=true]:border-l-accent data-[selected=true]:bg-accent-dim"
    >
      <Icon name={entry.icon} size={16} className="shrink-0 text-fg-muted" />
      <span className="shrink-0 font-medium">{entry.name}</span>
      <span className="min-w-0 flex-1 truncate text-xs text-fg-muted">{entry.description}</span>
      <span className="shrink-0 font-display text-[10px] tracking-wide text-fg-muted uppercase">
        {entry.group}
      </span>
    </Command.Item>
  )
}
