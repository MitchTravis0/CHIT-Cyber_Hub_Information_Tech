import test from 'node:test'
import assert from 'node:assert/strict'
import { downloadName, modulesToPath } from '../src/tools/wifi-qr/render.ts'

/** Builds a size by size matrix with the listed [row, col] pairs dark. */
function matrix(size: number, dark: Array<[number, number]>): boolean[] {
  const modules = new Array<boolean>(size * size).fill(false)
  for (const [row, col] of dark) modules[row * size + col] = true
  return modules
}

test('modulesToPath builds one subpath per horizontal run', () => {
  const modules = matrix(3, [
    [0, 0],
    [0, 1],
    [2, 2],
  ])
  assert.equal(modulesToPath(modules, 3, 4), 'M4 4h2v1h-2zM6 6h1v1h-1z')
})

test('modulesToPath returns nothing for an all-light matrix', () => {
  assert.equal(modulesToPath(new Array<boolean>(25).fill(false), 5, 4), '')
})

test('modulesToPath merges a full row into one subpath', () => {
  const modules = new Array<boolean>(9).fill(true)
  assert.equal(modulesToPath(modules, 3, 0), 'M0 0h3v1h-3zM0 1h3v1h-3zM0 2h3v1h-3z')
})

test('modulesToPath splits a row at a light module', () => {
  const modules = matrix(4, [
    [1, 0],
    [1, 2],
    [1, 3],
  ])
  assert.equal(modulesToPath(modules, 4, 0), 'M0 1h1v1h-1zM2 1h2v1h-2z')
})

test('modulesToPath honours the quiet zone', () => {
  const modules = matrix(3, [
    [0, 0],
    [2, 2],
  ])
  assert.equal(modulesToPath(modules, 3, 0), 'M0 0h1v1h-1zM2 2h1v1h-1z')
  assert.equal(modulesToPath(modules, 3, 4), 'M4 4h1v1h-1zM6 6h1v1h-1z')
})

test('downloadName slugs a Wi-Fi SSID', () => {
  assert.equal(downloadName('wifi', 'Guest WiFi 2.4GHz'), 'wifi-guest-wifi-2-4ghz')
})

test('downloadName falls back when there is nothing to slug', () => {
  assert.equal(downloadName('wifi', ''), 'wifi-network')
  assert.equal(downloadName('text', ''), 'qr-code')
  assert.equal(downloadName('wifi', '!!!'), 'wifi-network')
})

test('downloadName prefixes anything that is not wifi with qr', () => {
  assert.equal(downloadName('text', 'https://helpdesk.example.com'), 'qr-https-helpdesk-example-com')
})

test('downloadName truncates a long name and never ends in a dash', () => {
  const name = downloadName('wifi', 'A'.repeat(30) + ' ' + 'B'.repeat(30))
  const slug = name.slice('wifi-'.length)
  assert.ok(slug.length <= 32, `slug is ${slug.length} characters`)
  assert.ok(!slug.endsWith('-'), `slug ends in a dash: ${slug}`)
  assert.equal(slug, 'a'.repeat(30) + '-b')
})

test('downloadName drops a dash left at the truncation point', () => {
  const name = downloadName('wifi', 'C'.repeat(32) + ' tail')
  assert.equal(name, 'wifi-' + 'c'.repeat(32))
})
