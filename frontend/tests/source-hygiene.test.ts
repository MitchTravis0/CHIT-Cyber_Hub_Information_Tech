import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

/**
 * A control byte in a TypeScript file is invisible three times over: the editor
 * draws it as nothing, tsc and Vite accept it inside a string literal, and every
 * search tool classifies the file as binary and silently skips it.
 *
 * ResultsTable.tsx shipped with a NUL where a space belonged, in the separator
 * that joins a row's cells into the string the filter box searches. Filtering
 * across a column boundary therefore never matched, and because the file read as
 * binary, no grep in this repository had been able to see inside it for nine
 * phases. Go is safe from this (a NUL in a .go file will not compile) so this
 * checks the frontend only.
 */
const ROOTS = ['../src', '../tests']

function sourceFiles(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name === 'dist') continue
    const path = join(dir, name)
    if (statSync(path).isDirectory()) {
      out.push(...sourceFiles(path))
    } else if (/\.(ts|tsx|css|json)$/.test(name)) {
      out.push(path)
    }
  }
  return out
}

const FILES = ROOTS.flatMap((root) => sourceFiles(new URL(root, import.meta.url).pathname))

test('the scan actually reaches the source tree', () => {
  // Without this, a wrong path would make every check below pass over nothing.
  assert.ok(FILES.length > 100, `only found ${FILES.length} source files`)
  assert.ok(FILES.some((path) => path.endsWith('/components/ResultsTable.tsx')))
  assert.ok(FILES.some((path) => path.endsWith('/tools/registry.ts')))
})

test('no source file contains a control byte', () => {
  const offenders: string[] = []
  for (const path of FILES) {
    const bytes = readFileSync(path)
    for (let i = 0; i < bytes.length; i++) {
      const byte = bytes[i]
      // Tab, newline and carriage return are the only control bytes allowed.
      if (byte < 9 || (byte > 13 && byte < 32) || byte === 127) {
        offenders.push(`${path} byte ${i} is 0x${byte.toString(16)}`)
        break
      }
    }
  }
  assert.deepEqual(offenders, [])
})

test('the results table joins cells with a real space before filtering', () => {
  // The specific line the NUL was on. Asserted directly as well as by the byte
  // scan above, because this one has a user-visible consequence: the filter box
  // searches the joined string, so the separator decides whether a query that
  // spans two columns can match.
  const source = readFileSync(
    new URL('../src/components/ResultsTable.tsx', import.meta.url),
    'utf8',
  )
  assert.ok(
    source.includes("values.map(asText).join(' ').toLowerCase()"),
    'the filter haystack is no longer joined with a plain space',
  )
})
