/** The standard raw printing port, matching rawprint.DefaultPort. */
export const DEFAULT_PORT = 9100

export interface PortResult {
  ok: boolean
  port: number
  error: string
}

/** Reads the port field. An empty field means the standard port, not an error. */
export function parsePort(text: string): PortResult {
  const trimmed = text.trim()
  if (trimmed === '') return { ok: true, port: DEFAULT_PORT, error: '' }
  if (!/^\d+$/.test(trimmed)) {
    return { ok: false, port: 0, error: 'The port must be a number, usually 9100.' }
  }
  const port = Number(trimmed)
  if (port < 1 || port > 65535) {
    return { ok: false, port: 0, error: 'The port must be a number, usually 9100.' }
  }
  return { ok: true, port, error: '' }
}

/** The Online row's value, or null when the printer did not say and the row is omitted. */
export function onlineLabel(online: string): string | null {
  if (online === 'true') return 'Yes'
  if (online === 'false') return 'No'
  return null
}
