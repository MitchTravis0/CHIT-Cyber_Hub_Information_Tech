/**
 * Saving a generated file to the user's Downloads folder.
 *
 * Six shipped tools each carry their own copy of this; new tools use this one.
 * The anchor has to be in the document before it is clicked, because WebKit
 * ignores a click on a detached element.
 */

/** Hands the browser a file to save. `name` includes the extension. */
export function downloadBlob(name: string, blob: Blob): void {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = name
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

/** Saves text as a file. `type` is the MIME type, plain text by default. */
export function downloadText(name: string, body: string, type = 'text/plain;charset=utf-8'): void {
  downloadBlob(name, new Blob([body], { type }))
}

/** Saves an object as pretty-printed JSON. */
export function downloadJson(name: string, value: unknown): void {
  downloadText(name, JSON.stringify(value, null, 2), 'application/json')
}
