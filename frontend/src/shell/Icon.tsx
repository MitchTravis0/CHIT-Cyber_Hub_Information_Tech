import * as lucide from 'lucide-react'
import type { LucideProps } from 'lucide-react'
import type { ComponentType } from 'react'

const ICONS = lucide as unknown as Record<string, ComponentType<LucideProps> | undefined>

export interface IconProps extends LucideProps {
  /** lucide-react export name, e.g. 'Radar'. Unknown names fall back to a wrench. */
  name: string
}

/**
 * Renders a lucide icon chosen by name, because the tool registry stores icons
 * as plain strings so a new tool never has to touch shell code.
 */
export function Icon({ name, ...props }: IconProps) {
  const Component = ICONS[name] ?? lucide.Wrench
  return <Component {...props} />
}
