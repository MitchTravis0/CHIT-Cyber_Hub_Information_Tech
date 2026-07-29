import type { Item } from './api'

export type Filter = 'all' | 'startup' | 'services' | 'automatic' | 'concern'

const START_LABELS: Record<string, string> = {
  automatic: 'Automatic',
  manual: 'When needed',
  disabled: 'Disabled',
  boot: 'At boot',
}

/** An empty start mode is a gap this OS left, and the page says so rather than
 *  showing a blank cell that reads as a bug. */
export function startModeLabel(mode: string): string {
  return START_LABELS[mode] ?? 'not reported'
}

export function stateLabel(state: string): string {
  if (state === 'running') return 'Running'
  if (state === 'stopped') return 'Stopped'
  return ''
}

export function concernCount(items: Item[]): number {
  return items.filter((item) => item.concern !== '').length
}

function plural(n: number, singular: string, many: string): string {
  return `${n.toLocaleString()} ${n === 1 ? singular : many}`
}

/** The line above the table. "0 worth a look" is left off entirely: a zero
 *  count is not news, and saying it invites the tech to read it as a verdict. */
export function countLine(items: Item[]): string {
  const startup = items.filter((item) => item.kind === 'startup').length
  const services = items.filter((item) => item.kind === 'service').length
  const concerns = concernCount(items)

  const head = `${plural(startup, 'startup entry', 'startup entries')} and ${plural(services, 'service', 'services')}.`
  return concerns === 0 ? head : `${head} ${concerns} worth a look.`
}

export function filterItems(items: Item[], filter: Filter): Item[] {
  switch (filter) {
    case 'startup':
      return items.filter((item) => item.kind === 'startup')
    case 'services':
      return items.filter((item) => item.kind === 'service')
    case 'automatic':
      return items.filter((item) => item.startMode === 'automatic' || item.startMode === 'boot')
    case 'concern':
      return items.filter((item) => item.concern !== '')
    default:
      return items
  }
}
