import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

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

/**
 * fileURLToPath, not URL.pathname: on Windows the latter yields "/D:/a/..." with
 * a leading slash, which readdirSync cannot open, and the whole file throws
 * before a single assertion runs. Paths are then normalised to forward slashes
 * so the string comparisons below mean the same thing on all three platforms;
 * join() produces backslashes on Windows.
 */
function scan(root: string): string[] {
  return sourceFiles(fileURLToPath(new URL(root, import.meta.url))).map((path) =>
    path.split(sep).join('/'),
  )
}

const FILES = ROOTS.flatMap(scan)

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

/**
 * Two modules in one folder whose names differ only by case work perfectly on
 * Linux and break on macOS and Windows, where the filesystem is case-insensitive
 * by default. `disk-visualizer` shipped with `treemap.ts` beside `TreeMap.tsx`
 * from Phase 5 until the first CI run found it: resolving `'./TreeMap'` there
 * finds `treemap.ts` first, so the import landed on the layout module and tsc
 * reported "has no exported member 'TreeMap'". Nothing on a Linux developer
 * machine can reproduce that, which is why it is asserted here instead.
 */
test('no two modules in a folder differ only by case', () => {
  const byFolder = new Map<string, string[]>()
  for (const path of FILES) {
    if (!/\.tsx?$/.test(path)) continue
    const slash = path.lastIndexOf('/')
    const dir = path.slice(0, slash)
    const stem = path.slice(slash + 1).replace(/\.tsx?$/, '')
    const key = `${dir} ${stem.toLowerCase()}`
    byFolder.set(key, [...(byFolder.get(key) ?? []), path.slice(slash + 1)])
  }
  const clashes = [...byFolder.values()].filter((names) => names.length > 1)
  assert.deepEqual(clashes, [])
})

test('the case-collision scan looks at real .ts and .tsx files', () => {
  // A control: without this, a wrong filter would make the check above pass
  // over an empty list.
  const modules = FILES.filter((path) => /\.tsx?$/.test(path))
  assert.ok(modules.length > 100, `only ${modules.length} modules found`)
  assert.ok(modules.some((path) => path.endsWith('/disk-visualizer/layout.ts')))
  assert.ok(modules.some((path) => path.endsWith('/disk-visualizer/TreeMap.tsx')))
})
