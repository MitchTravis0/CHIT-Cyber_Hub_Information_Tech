// Explicit .ts extensions, unlike the rest of src/. Node's test runner has no
// extension inference, and this module has to be importable by a test that
// ranks the real registry rather than a fixture of it. Vite and tsc (moduleResolution
// "Bundler") both accept the extension, so nothing else changes.
import { TOOLS } from '../tools/registry.ts'
import { CATEGORY_LABELS, CATEGORY_ORDER, type View } from './nav.ts'

/** One row of the command palette: a tool, or one of the fixed destinations. */
export interface Entry {
  id: string
  name: string
  description: string
  keywords: string[]
  /** Category label, shown on the row and searched at the lowest weight. */
  group: string
  icon: string
  view: View
}

/**
 * Scores are tiers, not additions: only the best rule a token matches counts.
 * The gaps are wide enough that a name match can never be outranked by a
 * description match, whatever the tie-breaks do.
 */
const EXACT_ID = 100
const NAME_PREFIX = 85
const NAME_WORD_PREFIX = 80
const EXACT_KEYWORD = 75
const KEYWORD_PREFIX = 70
const NAME_CONTAINS = 60
const KEYWORD_CONTAINS = 50
const DESCRIPTION_WORD_PREFIX = 35
const DESCRIPTION_CONTAINS = 20
const GROUP_CONTAINS = 15

/** Lowercased with the separators dropped, so "Port Listener", "port-listener"
 *  and "portlistener" are all the same query. */
export function squash(text: string): string {
  return text.toLowerCase().replace(/[\s\-/_.]/g, '')
}

function words(text: string): string[] {
  return text.toLowerCase().split(/[^a-z0-9]+/i).filter((word) => word !== '')
}

function anyWordStartsWith(text: string, token: string): boolean {
  return words(text).some((word) => word.startsWith(token))
}

/** 0 means no match, and a token that scores 0 drops the whole entry. */
function scoreToken(entry: Entry, token: string): number {
  const name = squash(entry.name)
  const id = squash(entry.id)

  if (id === token || name === token) return EXACT_ID
  // Both name rules beat an exact keyword on purpose: typing "subnet" means the
  // tool called Subnet something rather than the IP Range Scanner that lists it
  // as a keyword, and typing "throughput" means LAN Throughput Test rather than
  // Disk Speed Test, which only has it as a keyword.
  if (name.startsWith(token) || id.startsWith(token)) return NAME_PREFIX
  if (anyWordStartsWith(entry.name, token)) return NAME_WORD_PREFIX
  if (entry.keywords.some((keyword) => squash(keyword) === token)) return EXACT_KEYWORD
  if (entry.keywords.some((keyword) => squash(keyword).startsWith(token))) return KEYWORD_PREFIX
  if (name.includes(token) || id.includes(token)) return NAME_CONTAINS
  if (entry.keywords.some((keyword) => squash(keyword).includes(token))) return KEYWORD_CONTAINS
  if (anyWordStartsWith(entry.description, token)) return DESCRIPTION_WORD_PREFIX
  if (squash(entry.description).includes(token)) return DESCRIPTION_CONTAINS
  if (squash(entry.group).includes(token)) return GROUP_CONTAINS
  return 0
}

/**
 * Ranks palette entries best first. Matching is substring-only on purpose: cmdk's
 * own matcher accepts a scattered subsequence, so "netcat" matched 58 of the 59
 * entries by way of "subNET CAlculaTor" and buried the one tool that names it.
 */
export function rankEntries(entries: Entry[], query: string): Entry[] {
  const tokens = query.toLowerCase().split(/\s+/).map(squash).filter((token) => token !== '')
  if (tokens.length === 0) return entries.slice()

  const scored: { entry: Entry; score: number }[] = []
  for (const entry of entries) {
    let total = 0
    // Every token has to land somewhere, or "port listener" would keep Port Scanner.
    const matchedAll = tokens.every((token) => {
      const score = scoreToken(entry, token)
      total += score
      return score > 0
    })
    if (matchedAll) scored.push({ entry, score: total })
  }

  return scored
    .sort((a, b) => b.score - a.score || a.entry.name.localeCompare(b.entry.name))
    .map((hit) => hit.entry)
}

const DESTINATIONS: Entry[] = [
  {
    id: 'home',
    name: 'Home',
    description: 'Favourites, recent tools and everything by category',
    keywords: ['home', 'start', 'dashboard', 'favourites', 'favorites'],
    group: 'Go to',
    icon: 'House',
    view: { kind: 'home' },
  },
  {
    id: 'settings',
    name: 'Settings',
    description: 'Accent colour, data location, updates and app version',
    keywords: ['settings', 'accent', 'colour', 'color', 'appearance', 'portable', 'update', 'version'],
    group: 'Go to',
    icon: 'Settings',
    view: { kind: 'settings' },
  },
]

/** Every palette entry in browse order: the destinations, then the categories in
 *  CATEGORY_ORDER with names sorted, which is what an empty query shows. */
export function paletteEntries(): Entry[] {
  const tools = CATEGORY_ORDER.flatMap((category) =>
    TOOLS.filter((tool) => tool.category === category)
      .sort((a, b) => a.name.localeCompare(b.name))
      .map(
        (tool): Entry => ({
          id: tool.id,
          name: tool.name,
          description: tool.description,
          keywords: tool.keywords,
          group: CATEGORY_LABELS[category],
          icon: tool.icon,
          view: { kind: 'tool', id: tool.id },
        }),
      ),
  )
  return DESTINATIONS.concat(tools)
}
