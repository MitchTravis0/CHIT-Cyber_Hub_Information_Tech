import test from 'node:test'
import assert from 'node:assert/strict'
import {
  docWarning,
  elapsed,
  emptyNote,
  exportDoc,
  exportFileName,
  filterNotes,
  formatNote,
  mergeNotes,
  migrateDoc,
  noteLabel,
  parseStamp,
  readImport,
  sortNotes,
  stampOf,
  validateEntry,
  validateNote,
  ENTRY_LONG_MESSAGE,
  LONG_MESSAGE,
  NEWER_FILE_MESSAGE,
  NEWER_VERSION_MESSAGE,
  NOT_A_NOTE_FILE_MESSAGE,
  REF_LONG_MESSAGE,
  TITLE_LONG_MESSAGE,
  TOO_BIG_MESSAGE,
  type Note,
} from '../src/tools/ticket-notes/notes.ts'

const NOW = '2026-07-26T10:00:00.000Z'

function worked(): Note {
  return {
    id: 'note-1',
    ref: 'INC0012345',
    title: 'PC will not join the domain',
    issue: 'User says the new laptop will not join CH.local and shows a trust error.',
    resolution: 'The clock skew was over the Kerberos five minute limit. Set NTP to the DC.',
    entries: [
      {
        id: 'step-1',
        stamp: '2026-07-26 09:14',
        text: 'Confirmed the laptop can ping the DC and resolve CH.local.',
      },
      {
        id: 'step-2',
        stamp: '2026-07-26 09:31',
        text: 'Reset the computer account in AD and rejoined. Same error.',
      },
      {
        id: 'step-3',
        stamp: '2026-07-26 10:41',
        text: 'Corrected the laptop clock, which was 14 minutes fast. Join succeeded.',
      },
    ],
    createdAt: NOW,
    updatedAt: NOW,
  }
}

// stampOf is built from local components and read back with local components,
// so these assertions hold in any time zone.
test('stampOf writes the local date and time, zero padded', () => {
  assert.equal(stampOf(new Date(2026, 6, 26, 9, 4)), '2026-07-26 09:04')
  assert.equal(stampOf(new Date(2026, 0, 1, 0, 0)), '2026-01-01 00:00')
  assert.equal(stampOf(new Date(2026, 11, 31, 23, 59)), '2026-12-31 23:59')
})

test('stampOf counts months from one, not from zero', () => {
  // getMonth() is zero based; a stamp reading 2026-06-26 would be the bug.
  assert.equal(stampOf(new Date(2026, 6, 26, 12, 0)).slice(0, 7), '2026-07')
})

test('parseStamp reads back what stampOf wrote', () => {
  const date = new Date(2026, 6, 26, 9, 14)
  const parsed = parseStamp(stampOf(date))
  assert.notEqual(parsed, null)
  assert.equal(parsed, new Date(2026, 6, 26, 9, 14).getTime())
})

test('parseStamp refuses anything that is not the stamp format', () => {
  for (const bad of [
    '',
    'not a date',
    '2026-07-26',
    '26/07/2026 09:14',
    '2026-13-01 09:00',
    '2026-07-26 25:00',
    '2026-07-26 09:61',
    '2026-02-30 09:00',
    '2026-7-26 09:14',
  ]) {
    assert.equal(parseStamp(bad), null, `${bad} should not parse`)
  }
})

test('elapsed measures earliest to latest, not first to last', () => {
  const note = worked()
  const span = elapsed(note.entries)
  assert.equal(span, 87 * 60 * 1000)

  // Out of order entries must give the same answer.
  const shuffled = [note.entries[2], note.entries[0], note.entries[1]]
  assert.equal(elapsed(shuffled), 87 * 60 * 1000)
})

test('elapsed needs two readable stamps', () => {
  const note = worked()
  assert.equal(elapsed([]), null)
  assert.equal(elapsed([note.entries[0]]), null)
  assert.equal(
    elapsed([note.entries[0], { id: 'x', stamp: 'this morning', text: 'something' }]),
    null,
  )
})

test('formatNote writes the plain text write-up exactly', () => {
  const expected = [
    'INC0012345 - PC will not join the domain',
    '',
    'Issue',
    'User says the new laptop will not join CH.local and shows a trust error.',
    '',
    'Steps taken',
    '2026-07-26 09:14  Confirmed the laptop can ping the DC and resolve CH.local.',
    '2026-07-26 09:31  Reset the computer account in AD and rejoined. Same error.',
    '2026-07-26 10:41  Corrected the laptop clock, which was 14 minutes fast. Join succeeded.',
    '',
    'Resolution',
    'The clock skew was over the Kerberos five minute limit. Set NTP to the DC.',
  ].join('\n')

  assert.equal(formatNote(worked(), 'text'), expected)
})

test('formatNote writes the markdown write-up exactly', () => {
  const expected = [
    '**INC0012345 - PC will not join the domain**',
    '',
    '**Issue**',
    '',
    'User says the new laptop will not join CH.local and shows a trust error.',
    '',
    '**Steps taken**',
    '',
    '- `2026-07-26 09:14` Confirmed the laptop can ping the DC and resolve CH.local.',
    '- `2026-07-26 09:31` Reset the computer account in AD and rejoined. Same error.',
    '- `2026-07-26 10:41` Corrected the laptop clock, which was 14 minutes fast. Join succeeded.',
    '',
    '**Resolution**',
    '',
    'The clock skew was over the Kerberos five minute limit. Set NTP to the DC.',
  ].join('\n')

  assert.equal(formatNote(worked(), 'markdown'), expected)
})

test('formatNote leaves out a section that has nothing in it, heading and all', () => {
  const noIssue = formatNote({ ...worked(), issue: '   ' }, 'text')
  assert.equal(noIssue.includes('Issue'), false)
  assert.equal(noIssue.includes('Steps taken'), true)

  const noResolution = formatNote({ ...worked(), resolution: '' }, 'text')
  assert.equal(noResolution.includes('Resolution'), false)

  const noSteps = formatNote({ ...worked(), entries: [] }, 'text')
  assert.equal(noSteps.includes('Steps taken'), false)

  const onlySteps = formatNote({ ...worked(), issue: '', resolution: '' }, 'text')
  assert.equal(
    onlySteps,
    [
      'INC0012345 - PC will not join the domain',
      '',
      'Steps taken',
      '2026-07-26 09:14  Confirmed the laptop can ping the DC and resolve CH.local.',
      '2026-07-26 09:31  Reset the computer account in AD and rejoined. Same error.',
      '2026-07-26 10:41  Corrected the laptop clock, which was 14 minutes fast. Join succeeded.',
    ].join('\n'),
  )
})

test('formatNote builds the heading from whichever of ref and title exist', () => {
  const base = { ...worked(), issue: '', resolution: '', entries: [] }
  assert.equal(formatNote(base, 'text'), 'INC0012345 - PC will not join the domain')
  assert.equal(formatNote({ ...base, ref: '' }, 'text'), 'PC will not join the domain')
  assert.equal(formatNote({ ...base, title: '' }, 'text'), 'INC0012345')
  assert.equal(formatNote({ ...base, ref: '', title: '' }, 'text'), 'Ticket note')
  assert.equal(formatNote({ ...base, ref: '  ', title: ' ' }, 'text'), 'Ticket note')
})

test('formatNote folds a multi-line step onto one line', () => {
  const note = {
    ...worked(),
    issue: '',
    resolution: '',
    entries: [{ id: 'a', stamp: '2026-07-26 09:14', text: 'first line\n  second line' }],
  }
  assert.equal(
    formatNote(note, 'text'),
    'INC0012345 - PC will not join the domain\n\nSteps taken\n2026-07-26 09:14  first line second line',
  )
})

test('formatNote keeps the line breaks inside the issue and the resolution', () => {
  const note = { ...worked(), entries: [], resolution: '', issue: 'line one\nline two' }
  assert.equal(formatNote(note, 'text').includes('line one\nline two'), true)
})

test('formatNote never ends with a newline', () => {
  for (const style of ['text', 'markdown'] as const) {
    const written = formatNote(worked(), style)
    assert.equal(written.endsWith('\n'), false)
  }
})

test('formatNote drops a step whose text is only whitespace', () => {
  const note = {
    ...worked(),
    issue: '',
    resolution: '',
    entries: [
      { id: 'a', stamp: '2026-07-26 09:14', text: '   ' },
      { id: 'b', stamp: '2026-07-26 09:20', text: 'real step' },
    ],
  }
  const written = formatNote(note, 'text')
  assert.equal(written.includes('09:14'), false)
  assert.equal(written.includes('09:20  real step'), true)
})

test('validateNote checks each field at its boundary', () => {
  const ok = validateNote({
    ref: 'a'.repeat(40),
    title: 'b'.repeat(120),
    issue: 'c'.repeat(4000),
    resolution: 'd'.repeat(4000),
  })
  assert.deepEqual(ok, {})

  const bad = validateNote({
    ref: 'a'.repeat(41),
    title: 'b'.repeat(121),
    issue: 'c'.repeat(4001),
    resolution: 'd'.repeat(4001),
  })
  assert.equal(bad.ref, REF_LONG_MESSAGE)
  assert.equal(bad.title, TITLE_LONG_MESSAGE)
  assert.equal(bad.issue, LONG_MESSAGE)
  assert.equal(bad.resolution, LONG_MESSAGE)
})

test('validateEntry needs text and caps its length', () => {
  assert.notEqual(validateEntry(''), undefined)
  assert.notEqual(validateEntry('   '), undefined)
  assert.equal(validateEntry('a'.repeat(500)), undefined)
  assert.equal(validateEntry('a'.repeat(501)), ENTRY_LONG_MESSAGE)
})

test('migrateDoc copes with everything a file on disk can be', () => {
  assert.deepEqual(migrateDoc(null).notes, [])
  assert.deepEqual(migrateDoc('text').notes, [])
  assert.deepEqual(migrateDoc({ version: 1 }).notes, [])
  assert.deepEqual(migrateDoc({ version: 1, notes: 'no' }).notes, [])
  assert.deepEqual(migrateDoc({ version: 2, notes: [worked()] }).notes, [])
  assert.deepEqual(migrateDoc({ version: 1, notes: [null, 5] }).notes, [])
  assert.equal(migrateDoc({ version: 1, notes: [worked()] }).notes.length, 1)
})

test('migrateDoc tolerates a note whose entries are broken, and drops an empty note', () => {
  const doc = migrateDoc({
    version: 1,
    notes: [
      { ref: 'INC1', entries: 'not an array' },
      { ref: 'INC2', entries: [null, { text: '' }, { stamp: '2026-07-26 09:00', text: 'ok' }] },
      { ref: '', title: '', issue: '', resolution: '', entries: [] },
    ],
  })
  assert.equal(doc.notes.length, 2)
  assert.deepEqual(doc.notes[0].entries, [])
  assert.equal(doc.notes[1].entries.length, 1)
})

test('docWarning only fires for a document from the future', () => {
  assert.equal(docWarning({ version: 2 }), NEWER_VERSION_MESSAGE)
  assert.equal(docWarning({ version: 1 }), '')
  assert.equal(docWarning(null), '')
})

test('mergeNotes adds a new note and skips one whose id is already here', () => {
  const current = [worked()]
  const same = mergeNotes(current, { notes: [worked()] }, () => 'note-new', NOW)
  assert.equal(same.added, 0)
  assert.equal(same.skipped, 1)
  assert.equal(same.notes.length, 1)

  const other = mergeNotes(current, { notes: [{ ...worked(), id: 'note-2' }] }, () => 'note-new', NOW)
  assert.equal(other.added, 1)
  assert.equal(other.notes.length, 2)
})

test('mergeNotes gives a note with no id a fresh one', () => {
  const report = mergeNotes([], { notes: [{ ...worked(), id: '' }] }, () => 'note-fresh', NOW)
  assert.equal(report.added, 1)
  assert.equal(report.notes[0].id, 'note-fresh')
})

test('mergeNotes skips an entirely empty note and refuses a file that is not one', () => {
  const empty = mergeNotes(
    [],
    { notes: [{ ref: '', title: '', issue: '', resolution: '', entries: [] }] },
    () => 'note-x',
    NOW,
  )
  assert.equal(empty.added, 0)
  assert.equal(empty.skipped, 1)

  const wrong = mergeNotes([worked()], { devices: [] }, () => 'note-x', NOW)
  assert.equal(wrong.error, NOT_A_NOTE_FILE_MESSAGE)
  assert.equal(wrong.notes.length, 1)
})

test('mergeNotes is pure', () => {
  const current = [worked()]
  const incoming = { notes: [{ ...worked(), id: 'note-2' }] }
  const first = mergeNotes(current, incoming, () => 'note-fixed', NOW)
  const second = mergeNotes(current, incoming, () => 'note-fixed', NOW)
  assert.deepEqual(first.notes, second.notes)
  assert.equal(current.length, 1)
})

test('sortNotes puts the most recently touched first', () => {
  const list = [
    { ...worked(), id: 'a', updatedAt: '2026-07-26T09:00:00.000Z' },
    { ...worked(), id: 'b', updatedAt: '2026-07-26T11:00:00.000Z' },
    { ...worked(), id: 'c', updatedAt: '2026-07-26T10:00:00.000Z' },
  ]
  assert.deepEqual(
    sortNotes(list).map((n) => n.id),
    ['b', 'c', 'a'],
  )
})

test('filterNotes matches the reference and the title, case-insensitively', () => {
  const list = [worked(), { ...worked(), id: 'b', ref: 'REQ999', title: 'New starter setup' }]
  assert.equal(filterNotes(list, 'inc001').length, 1)
  assert.equal(filterNotes(list, 'DOMAIN').length, 1)
  assert.equal(filterNotes(list, 'starter').length, 1)
  assert.equal(filterNotes(list, '').length, 2)
  assert.equal(filterNotes(list, 'nothing').length, 0)
})

test('noteLabel prefers the title, then the reference, then a placeholder', () => {
  assert.equal(noteLabel(worked()), 'PC will not join the domain')
  assert.equal(noteLabel({ ...worked(), title: '' }), 'INC0012345')
  assert.equal(noteLabel({ ...worked(), title: '', ref: '' }), 'Untitled note')
})

test('emptyNote is empty and carries the clock it was given', () => {
  const note = emptyNote('note-1', NOW)
  assert.equal(note.ref, '')
  assert.deepEqual(note.entries, [])
  assert.equal(note.createdAt, NOW)
  assert.equal(note.updatedAt, NOW)
})

test('exportDoc names the format and exportFileName carries the date', () => {
  const doc = exportDoc([worked()], NOW)
  assert.equal(doc.kind, 'chit/ticket-notes')
  assert.equal(doc.version, 1)
  assert.equal(exportFileName(NOW), 'chit-ticket-notes-2026-07-26.json')
})

test('readImport refuses everything that is not a ticket note file', () => {
  assert.equal(readImport('not json').ok, false)
  assert.equal(readImport('null').ok, false)
  assert.equal(readImport('{"version":1}').ok, false)

  const newer = readImport('{"version":2,"notes":[]}')
  assert.equal(newer.ok, false)
  if (!newer.ok) assert.equal(newer.error, NEWER_FILE_MESSAGE)

  const tooBig = readImport('x'.repeat(5 * 1024 * 1024 + 1))
  assert.equal(tooBig.ok, false)
  if (!tooBig.ok) assert.equal(tooBig.error, TOO_BIG_MESSAGE)

  assert.equal(readImport(JSON.stringify(exportDoc([worked()], NOW))).ok, true)
})
