// Explicit .ts extension so Node's test runner, which has no extension
// inference, can reach this module and search.ts through it.
import { TOOLS, type ToolCategory, type ToolDef } from '../tools/registry.ts'

export type View = { kind: 'home' } | { kind: 'settings' } | { kind: 'tool'; id: string }

/** Sidebar and home page order: the network work a tech does most comes first. */
export const CATEGORY_ORDER: ToolCategory[] = [
  'network',
  'diagnostics',
  'remote',
  'system',
  'files',
  'security',
  'utilities',
  'helpdesk',
]

export const CATEGORY_LABELS: Record<ToolCategory, string> = {
  network: 'Network',
  diagnostics: 'Diagnostics',
  remote: 'Remote and management',
  system: 'System and bench',
  files: 'Files and data',
  security: 'Security',
  utilities: 'Utilities',
  helpdesk: 'Helpdesk',
}

export const CATEGORY_ICONS: Record<ToolCategory, string> = {
  network: 'Network',
  diagnostics: 'Activity',
  remote: 'MonitorSmartphone',
  system: 'Cpu',
  files: 'FolderOpen',
  security: 'ShieldCheck',
  utilities: 'Wrench',
  helpdesk: 'LifeBuoy',
}

export interface ToolGroup {
  category: ToolCategory
  label: string
  icon: string
  tools: ToolDef[]
}

/** Groups tools by category in CATEGORY_ORDER, names sorted, empty categories dropped. */
export function toolGroups(tools: ToolDef[] = TOOLS): ToolGroup[] {
  return CATEGORY_ORDER.map((category) => ({
    category,
    label: CATEGORY_LABELS[category],
    icon: CATEGORY_ICONS[category],
    tools: tools
      .filter((tool) => tool.category === category)
      .sort((a, b) => a.name.localeCompare(b.name)),
  })).filter((group) => group.tools.length > 0)
}

export function findTool(id: string): ToolDef | undefined {
  return TOOLS.find((tool) => tool.id === id)
}

/** Keeps only ids that still exist, in the order given. */
export function resolveTools(ids: string[]): ToolDef[] {
  return ids.map(findTool).filter((tool): tool is ToolDef => tool !== undefined)
}

export const MOD_LABEL = /Mac|iPhone|iPad/.test(navigator.userAgent) ? 'Cmd' : 'Ctrl'
