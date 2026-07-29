// The two browser-side exports: printing through a hidden iframe, and a PNG
// through a canvas. Both are thin wrappers over the pure functions in
// labels.ts, which is where the geometry that can be tested lives.

import type { Preset } from './labels'

/** Writes the sheet into a hidden iframe and opens the OS print dialog.
 *  False when the iframe could not be created at all. */
export function printSheet(html: string): boolean {
  const frame = document.createElement('iframe')
  frame.style.position = 'fixed'
  frame.style.right = '0'
  frame.style.bottom = '0'
  frame.style.width = '0'
  frame.style.height = '0'
  frame.style.border = '0'
  document.body.appendChild(frame)

  const doc = frame.contentDocument
  const view = frame.contentWindow
  if (doc === null || view === null) {
    frame.remove()
    return false
  }

  doc.open()
  doc.write(html)
  doc.close()

  // The frame has to outlive the print dialog, which is modal in some webviews
  // and not in others, so it is removed on a timer rather than straight after.
  view.focus()
  view.print()
  setTimeout(() => frame.remove(), 60000)
  return true
}

/**
 * Draws one label onto a canvas and downloads it as a PNG.
 *
 * White background and black marks, for the same reason labelSvg gives: a label
 * is printed on paper and read by a camera, so it cannot follow the app theme.
 * The same technique the Wi-Fi QR tool already ships in its png.ts.
 */
export function downloadPng(
  svg: string,
  preset: Preset,
  scale: number,
  fileName: string,
): boolean {
  const canvas = document.createElement('canvas')
  canvas.width = Math.round(preset.w * scale)
  canvas.height = Math.round(preset.h * scale)
  const ctx = canvas.getContext('2d')
  if (ctx === null) return false

  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, canvas.width, canvas.height)

  const image = new Image()
  image.onload = () => {
    ctx.drawImage(image, 0, 0, canvas.width, canvas.height)
    const link = document.createElement('a')
    link.href = canvas.toDataURL('image/png')
    link.download = `${fileName}.png`
    document.body.appendChild(link)
    link.click()
    link.remove()
  }
  image.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
  return true
}
