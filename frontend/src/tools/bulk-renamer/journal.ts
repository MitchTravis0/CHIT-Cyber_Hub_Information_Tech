export const RENAMER_NAMESPACE = 'bulk-renamer'
export const RENAMER_DOC_VERSION = 1

/** Mirrors renamer.Rename. */
export interface Rename {
  from: string
  to: string
}

/** Mirrors renamer.Batch. */
export interface Batch {
  folder: string
  appliedAt: string
  renames: Rename[]
}

export interface RenamerDoc {
  version: number
  batch: Batch | null
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

/**
 * Reads whatever came off disk. The file is user-editable, so nothing about its
 * shape is trusted: anything unreadable becomes null and the undo panel simply
 * does not appear, rather than the tool showing an error about its own scratch
 * file.
 */
export function readBatch(raw: unknown): Batch | null {
  if (!isObject(raw)) return null
  const batch = raw.batch
  if (!isObject(batch)) return null

  const folder = text(batch.folder)
  if (folder === '') return null
  if (!Array.isArray(batch.renames)) return null

  const renames: Rename[] = []
  for (const entry of batch.renames) {
    if (!isObject(entry)) continue
    const from = text(entry.from)
    const to = text(entry.to)
    if (from === '' || to === '') continue
    renames.push({ from, to })
  }
  if (renames.length === 0) return null

  return { folder, appliedAt: text(batch.appliedAt), renames }
}
