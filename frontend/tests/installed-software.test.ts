import test from 'node:test'
import assert from 'node:assert/strict'
import {
  countLine,
  filterPrograms,
  hasAny,
  programId,
  sortPrograms,
  sourceList,
} from '../src/tools/installed-software/programs.ts'
import type { Program } from '../src/tools/installed-software/api.ts'

function program(over: Partial<Program> = {}): Program {
  return {
    name: 'thing',
    version: '1.0',
    publisher: '',
    installedOn: '',
    sizeBytes: 0,
    source: 'pacman',
    ...over,
  }
}

// Five rows: 3 from pacman, 2 from flatpak.
const FIXTURE: Program[] = [
  program({ name: 'bash', source: 'pacman', installedOn: '2026-07-24', sizeBytes: 1024 }),
  program({ name: 'curl', source: 'pacman' }),
  program({ name: 'zlib', source: 'pacman' }),
  program({ name: 'org.mozilla.firefox', source: 'flatpak' }),
  program({ name: 'org.gimp.GIMP', source: 'flatpak' }),
]

test('sourceList joins one, two and three sources', () => {
  assert.equal(sourceList([]), '')
  assert.equal(sourceList(['pacman']), 'pacman')
  assert.equal(sourceList(['pacman', 'flatpak']), 'pacman and flatpak')
  assert.equal(sourceList(['pacman', 'flatpak', 'dpkg']), 'pacman, flatpak and dpkg')
  assert.equal(sourceList(['a', 'b', 'c', 'd']), 'a, b, c and d')
})

test('countLine names the count and the sources', () => {
  assert.equal(countLine(FIXTURE, ['pacman', 'flatpak']), '5 programs from pacman and flatpak.')
  assert.equal(countLine([program()], ['pacman']), '1 program from pacman.')
  assert.equal(countLine([], []), 'Nothing was found.')
  assert.equal(countLine([program()], []), '1 program.')
})

test('filterPrograms returns exactly the right rows, not merely the right ones among others', () => {
  assert.equal(filterPrograms(FIXTURE, 'all').length, 5)
  assert.equal(filterPrograms(FIXTURE, 'pacman').length, 3)
  assert.equal(filterPrograms(FIXTURE, 'flatpak').length, 2)
  assert.equal(filterPrograms(FIXTURE, 'dpkg').length, 0)
  for (const row of filterPrograms(FIXTURE, 'flatpak')) {
    assert.equal(row.source, 'flatpak')
  }
})

test('sortPrograms puts numbers and mixed case where a person expects them', () => {
  const messy = [
    program({ name: 'Adobe Reader' }),
    program({ name: '7-Zip' }),
    program({ name: 'adobe air' }),
    program({ name: 'App 10' }),
    program({ name: 'App 2' }),
  ]
  const names = sortPrograms(messy).map((p) => p.name)
  assert.deepEqual(names, ['7-Zip', 'adobe air', 'Adobe Reader', 'App 2', 'App 10'])
})

test('sortPrograms does not mutate the array it was given', () => {
  const first = FIXTURE[0].name
  sortPrograms(FIXTURE)
  assert.equal(FIXTURE[0].name, first)
})

test('hasAny is true when one row fills the field and false for an all-empty list', () => {
  assert.equal(hasAny(FIXTURE, 'installedOn'), true)
  assert.equal(hasAny(FIXTURE, 'sizeBytes'), true)
  assert.equal(hasAny([program(), program()], 'installedOn'), false)
  assert.equal(hasAny([program(), program()], 'sizeBytes'), false)
  assert.equal(hasAny([], 'installedOn'), false)
  assert.equal(hasAny([], 'sizeBytes'), false)
})

test('programId separates the same name at two versions and from two sources', () => {
  assert.notEqual(
    programId(program({ name: '7-Zip', version: '23.01' })),
    programId(program({ name: '7-Zip', version: '22.00' })),
  )
  assert.notEqual(
    programId(program({ source: 'pacman' })),
    programId(program({ source: 'flatpak' })),
  )
})
