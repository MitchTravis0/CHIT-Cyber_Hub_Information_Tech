import type { ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'
import { focusRing } from './Button'
import { SidebarToggle } from './SidebarToggle'
import { cn } from '../lib/format'

export interface ToolShellProps {
  title: string
  /** One line, the same sentence as the tool's registry description. */
  description?: string
  /** Contents of the collapsible "How to use this" panel. Omit to hide it. */
  help?: ReactNode
  /** Right-aligned controls in the header, usually buttons. */
  actions?: ReactNode
  children: ReactNode
}

/** Standard frame for every tool page: header, optional help, scrolling content. */
export function ToolShell({ title, description, help, actions, children }: ToolShellProps) {
  return (
    <section className="flex h-full min-h-0 flex-col">
      {/* corner-cut plus the accent top edge is the signature detail that marks
          a tool page. It sits on the header, not on the scrolling body, because
          a clip-path over a scroll container eats the scrollbar corner. */}
      <header className="corner-cut flex flex-wrap items-start justify-between gap-3 border-t-2 border-b border-t-accent border-b-border bg-surface-2 px-4 py-3">
        <div className="flex min-w-0 items-start gap-2">
          <SidebarToggle className="-ml-1 mt-0.5" />
          <div className="min-w-0">
            <h1 className="truncate text-base font-semibold text-fg">{title}</h1>
            {description !== undefined && (
              <p className="mt-0.5 text-xs text-fg-muted">{description}</p>
            )}
          </div>
        </div>
        {actions !== undefined && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </header>

      {help !== undefined && (
        <details className="group border-b border-border bg-surface-2">
          {/* WebKit ignores list-style on summary, hence the marker rule. */}
          <summary
            className={cn(
              'flex cursor-pointer list-none items-center gap-1.5 px-4 py-2 text-xs font-medium text-fg-muted select-none hover:text-fg [&::-webkit-details-marker]:hidden',
              focusRing,
            )}
          >
            <ChevronRight
              size={14}
              className="shrink-0 transition-transform group-open:rotate-90"
              aria-hidden
            />
            How to use this
          </summary>
          <div className="px-4 pt-1 pb-3 pl-9 text-xs leading-relaxed text-fg-muted">{help}</div>
        </details>
      )}

      <div className="min-h-0 flex-1 overflow-auto p-4">{children}</div>
    </section>
  )
}
