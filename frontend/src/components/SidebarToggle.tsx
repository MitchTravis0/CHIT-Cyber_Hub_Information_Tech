import { PanelLeft } from 'lucide-react'
import { cn } from '../lib/format'
import { useSidebar } from '../lib/sidebar'
import { focusRing } from './Button'

/** Shows and hides the side menu. Sits in every page header, because when the
 *  menu is hidden the header is the only place left to bring it back from. */
export function SidebarToggle({ className }: { className?: string }) {
  const { hidden, toggle } = useSidebar()
  const label = hidden ? 'Show the menu' : 'Hide the menu'

  return (
    <button
      type="button"
      onClick={toggle}
      aria-expanded={!hidden}
      aria-label={label}
      title={label}
      className={cn(
        'shrink-0 rounded p-1.5 text-fg-muted transition-colors hover:bg-surface-3 hover:text-fg',
        focusRing,
        className,
      )}
    >
      <PanelLeft size={16} aria-hidden />
    </button>
  )
}
