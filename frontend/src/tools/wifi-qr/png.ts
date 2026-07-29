import type { QrCode } from './api'

/** Draws the code onto an off-screen canvas and triggers a download. */
export function downloadPng(code: QrCode, fileName: string, scale: number): void {
  const side = (code.size + code.quiet * 2) * scale
  const canvas = document.createElement('canvas')
  canvas.width = side
  canvas.height = side
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  // A QR code has to be dark on light in both themes, so these two are the only
  // colours in this tool that are not semantic tokens.
  ctx.fillStyle = '#ffffff'
  ctx.fillRect(0, 0, side, side)
  ctx.fillStyle = '#000000'
  for (let row = 0; row < code.size; row++) {
    for (let col = 0; col < code.size; col++) {
      if (code.modules[row * code.size + col]) {
        ctx.fillRect((col + code.quiet) * scale, (row + code.quiet) * scale, scale, scale)
      }
    }
  }

  const link = document.createElement('a')
  link.href = canvas.toDataURL('image/png')
  link.download = fileName
  document.body.appendChild(link)
  link.click()
  link.remove()
}
