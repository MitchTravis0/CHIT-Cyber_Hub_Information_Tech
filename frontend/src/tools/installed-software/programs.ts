import type { Program } from './api'

/** "pacman and flatpak". Mirrors swlist.SourceList so the two read the same. */
export function sourceList(sources: string[]): string {
  if (sources.length === 0) return ''
  if (sources.length === 1) return sources[0]
  if (sources.length === 2) return `${sources[0]} and ${sources[1]}`
  return `${sources.slice(0, -1).join(', ')} and ${sources[sources.length - 1]}`
}

export function countLine(programs: Program[], sources: string[]): string {
  if (programs.length === 0) return 'Nothing was found.'
  const word = programs.length === 1 ? 'program' : 'programs'
  const from = sourceList(sources)
  if (from === '') return `${programs.length} ${word}.`
  return `${programs.length} ${word} from ${from}.`
}

/** `source` is 'all' or one of Report.sources. */
export function filterPrograms(programs: Program[], source: string): Program[] {
  if (source === 'all') return programs
  return programs.filter((program) => program.source === source)
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/** By name, so 7-Zip and Adobe sort the way a person expects and case does not
 * split the list into two blocks. */
export function sortPrograms(programs: Program[]): Program[] {
  return programs.slice().sort((a, b) => collator.compare(a.name, b.name))
}

/**
 * Whether any row has a value for a field. A column nothing fills is dropped
 * from the table rather than rendered empty, which reads as a bug.
 */
export function hasAny(programs: Program[], field: 'installedOn' | 'sizeBytes'): boolean {
  return programs.some((program) =>
    field === 'installedOn' ? program.installedOn !== '' : program.sizeBytes > 0,
  )
}

export function programId(program: Program): string {
  return `${program.source}|${program.name}|${program.version}`
}
