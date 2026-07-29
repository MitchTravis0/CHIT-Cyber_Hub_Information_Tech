import test from 'node:test'
import assert from 'node:assert/strict'
import {
  checklistKey,
  checklistsFileName,
  docWarning,
  exportChecklistsDoc,
  exportRunsDoc,
  formatRun,
  mergeChecklists,
  migrateDoc,
  moveItem,
  readImport,
  reportFileName,
  resultLine,
  runLabel,
  runTally,
  runsFileName,
  sortRuns,
  stampOf,
  startRun,
  starterChecklists,
  validateChecklist,
  validateNote,
  validateSite,
  ITEM_LONG_MESSAGE,
  NAME_LONG_MESSAGE,
  NAME_MESSAGE,
  NEWER_FILE_MESSAGE,
  NEWER_VERSION_MESSAGE,
  NOTE_LONG_MESSAGE,
  NOT_A_CHECKLIST_FILE_MESSAGE,
  NO_ITEMS_MESSAGE,
  SITE_LONG_MESSAGE,
  TOO_BIG_MESSAGE,
  TOO_MANY_ITEMS_MESSAGE,
  type Checklist,
  type Run,
} from '../src/tools/site-checklist/checklists.ts'

const NOW = '2026-07-26T09:00:00.000Z'

let counter = 0
const makeId = () => `id-${++counter}`

function checklist(over: Partial<Checklist> = {}): Checklist {
  return {
    id: 'list-1',
    name: 'New PC setup',
    description: 'Everything a new machine needs.',
    items: [
      { id: 'item-1', text: 'Windows updates installed and rebooted' },
      { id: 'item-2', text: 'Machine renamed and joined to the domain' },
    ],
    addedAt: NOW,
    updatedAt: NOW,
    ...over,
  }
}

/** The run from the worked example in the spec. */
function workedRun(): Run {
  return {
    id: 'run-1',
    checklistId: 'list-1',
    checklistName: 'New PC setup',
    site: 'Head Office',
    startedStamp: '2026-07-26 09:00',
    finishedStamp: '2026-07-26 10:12',
    items: [
      { id: 'a', text: 'Windows updates installed and rebooted', state: 'done', note: '' },
      {
        id: 'b',
        text: 'Machine renamed and joined to the domain',
        state: 'done',
        note: 'BitLocker key escrowed to AD, recorded in ticket INC0012345',
      },
      { id: 'c', text: 'Antivirus installed and reporting in', state: 'done', note: '' },
      {
        id: 'd',
        text: 'Power plan set so the machine does not sleep on mains',
        state: 'skipped',
        note: "Left on the client's own GPO setting at their request",
      },
      {
        id: 'e',
        text: 'Docking station firmware updated',
        state: 'na',
        note: 'No dock issued with this machine',
      },
      { id: 'f', text: 'Asset tag applied and recorded in the inventory', state: 'todo', note: '' },
    ],
    updatedAt: NOW,
  }
}

test('validateChecklist accepts a name and items, dropping empty rows', () => {
  const result = validateChecklist('  New PC setup  ', ['  first  ', '', '   ', 'second'])
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.name, 'New PC setup')
  assert.deepEqual(result.items, ['first', 'second'])
})

test('validateChecklist needs a name and at least one item', () => {
  const noName = validateChecklist('', ['a'])
  assert.equal(noName.ok, false)
  if (!noName.ok) assert.equal(noName.errors.name, NAME_MESSAGE)

  const noItems = validateChecklist('Name', ['', '   '])
  assert.equal(noItems.ok, false)
  if (!noItems.ok) assert.equal(noItems.errors.items, NO_ITEMS_MESSAGE)
})

test('validateChecklist checks the boundaries', () => {
  const okName = validateChecklist('a'.repeat(80), ['x'])
  assert.equal(okName.ok, true)

  const longName = validateChecklist('a'.repeat(81), ['x'])
  assert.equal(longName.ok, false)
  if (!longName.ok) assert.equal(longName.errors.name, NAME_LONG_MESSAGE)

  const okItem = validateChecklist('Name', ['i'.repeat(200)])
  assert.equal(okItem.ok, true)

  const longItem = validateChecklist('Name', ['i'.repeat(201)])
  assert.equal(longItem.ok, false)
  if (!longItem.ok) assert.equal(longItem.errors.itemAt?.[0], ITEM_LONG_MESSAGE)

  const ok200 = validateChecklist('Name', Array.from({ length: 200 }, (_, i) => `item ${i}`))
  assert.equal(ok200.ok, true)

  const too201 = validateChecklist('Name', Array.from({ length: 201 }, (_, i) => `item ${i}`))
  assert.equal(too201.ok, false)
  if (!too201.ok) assert.equal(too201.errors.items, TOO_MANY_ITEMS_MESSAGE)
})

test('validateNote and validateSite check their boundaries', () => {
  assert.equal(validateNote('n'.repeat(500)), undefined)
  assert.equal(validateNote('n'.repeat(501)), NOTE_LONG_MESSAGE)
  assert.equal(validateSite('s'.repeat(80)), undefined)
  assert.equal(validateSite('s'.repeat(81)), SITE_LONG_MESSAGE)
})

test('startRun copies the items so editing the checklist cannot rewrite a run', () => {
  const list = checklist()
  const run = startRun(list, '  Head Office  ', () => 'run-x', '2026-07-26 09:00', NOW)

  assert.equal(run.checklistId, 'list-1')
  assert.equal(run.checklistName, 'New PC setup')
  assert.equal(run.site, 'Head Office')
  assert.equal(run.finishedStamp, '')
  assert.equal(run.items.length, 2)
  assert.equal(
    run.items.every((item) => item.state === 'todo' && item.note === ''),
    true,
  )

  // Editing the checklist afterwards must not reach into the run.
  list.items[0].text = 'CHANGED'
  assert.equal(run.items[0].text, 'Windows updates installed and rebooted')
})

test('runTally counts every state and treats an unknown one as to do', () => {
  const tally = runTally(workedRun())
  assert.deepEqual(tally, { done: 3, skipped: 1, na: 1, todo: 1, dealtWith: 5, total: 6 })

  const doc = migrateDoc({
    version: 1,
    checklists: [],
    runs: [{ ...workedRun(), items: [{ id: 'a', text: 'x', state: 'nonsense', note: '' }] }],
  })
  assert.equal(doc.runs[0].items[0].state, 'todo')
})

test('resultLine lists only the non-zero counts, but always shows done', () => {
  assert.equal(
    resultLine(workedRun()),
    'Result: 3 done, 1 skipped, 1 not applicable, 1 still to do',
  )

  const allDone: Run = {
    ...workedRun(),
    items: workedRun().items.map((item) => ({ ...item, state: 'done' })),
  }
  assert.equal(resultLine(allDone), 'Result: 6 done')

  const noneDone: Run = {
    ...workedRun(),
    items: workedRun().items.map((item) => ({ ...item, state: 'todo' })),
  }
  assert.equal(resultLine(noneDone), 'Result: 0 done, 6 still to do')
})

test('resultLine says so when the run is still open', () => {
  assert.equal(
    resultLine({ ...workedRun(), finishedStamp: '' }),
    'Result: 3 done, 1 skipped, 1 not applicable, 1 still to do (still open)',
  )
})

test('formatRun writes the report exactly', () => {
  const expected = [
    'New PC setup',
    'Site: Head Office',
    'Started: 2026-07-26 09:00',
    'Finished: 2026-07-26 10:12',
    'Result: 3 done, 1 skipped, 1 not applicable, 1 still to do',
    '',
    '[x] Windows updates installed and rebooted',
    '[x] Machine renamed and joined to the domain',
    '    BitLocker key escrowed to AD, recorded in ticket INC0012345',
    '[x] Antivirus installed and reporting in',
    '[-] Power plan set so the machine does not sleep on mains',
    "    Left on the client's own GPO setting at their request",
    '[n/a] Docking station firmware updated',
    '    No dock issued with this machine',
    '[ ] Asset tag applied and recorded in the inventory',
  ].join('\n')

  assert.equal(formatRun(workedRun()), expected)
})

test('formatRun leaves out the site line and the finished line when empty', () => {
  const open = formatRun({ ...workedRun(), site: '', finishedStamp: '' })
  assert.equal(open.includes('Site:'), false)
  assert.equal(open.includes('Finished:'), false)
  assert.equal(open.includes('(still open)'), true)
})

test('formatRun indents every line of a multi-line note', () => {
  const run: Run = {
    ...workedRun(),
    items: [{ id: 'a', text: 'One item', state: 'done', note: 'first line\nsecond line' }],
  }
  assert.equal(
    formatRun(run),
    [
      'New PC setup',
      'Site: Head Office',
      'Started: 2026-07-26 09:00',
      'Finished: 2026-07-26 10:12',
      'Result: 1 done',
      '',
      '[x] One item',
      '    first line',
      '    second line',
    ].join('\n'),
  )
})

test('formatRun gives an item with no note no second line, and never ends with a newline', () => {
  const run: Run = {
    ...workedRun(),
    items: [{ id: 'a', text: 'One item', state: 'done', note: '   ' }],
  }
  const written = formatRun(run)
  assert.equal(written.endsWith('[x] One item'), true)
  assert.equal(written.endsWith('\n'), false)
})

test('reportFileName slugs the checklist and the site', () => {
  assert.equal(
    reportFileName(workedRun(), '2026-07-26T10:12:00.000Z'),
    'chit-run-new-pc-setup-head-office-2026-07-26.txt',
  )
  assert.equal(
    reportFileName({ ...workedRun(), site: '' }, '2026-07-26T10:12:00.000Z'),
    'chit-run-new-pc-setup-no-site-2026-07-26.txt',
  )
})

test('moveItem moves by one and does nothing at either end', () => {
  const items = ['a', 'b', 'c']
  assert.deepEqual(moveItem(items, 1, -1), ['b', 'a', 'c'])
  assert.deepEqual(moveItem(items, 1, 1), ['a', 'c', 'b'])
  assert.deepEqual(moveItem(items, 0, -1), ['a', 'b', 'c'])
  assert.deepEqual(moveItem(items, 2, 1), ['a', 'b', 'c'])
  assert.deepEqual(items, ['a', 'b', 'c'])
})

test('starterChecklists are the three the spec names, with the right item counts', () => {
  const seeds = starterChecklists(NOW)
  assert.equal(seeds.length, 3)
  assert.deepEqual(
    seeds.map((s) => s.id),
    ['list-starter-new-pc', 'list-starter-office-move', 'list-starter-decommission'],
  )
  assert.deepEqual(
    seeds.map((s) => s.items.length),
    [10, 11, 9],
  )
  for (const seed of seeds) {
    const checked = validateChecklist(
      seed.name,
      seed.items.map((item) => item.text),
    )
    assert.equal(checked.ok, true, `${seed.name} does not pass validation`)
    assert.equal(new Set(seed.items.map((i) => i.id)).size, seed.items.length)
  }
})

test('migrateDoc copes with everything a file on disk can be', () => {
  assert.deepEqual(migrateDoc(null).checklists, [])
  assert.deepEqual(migrateDoc({ version: 1 }).checklists, [])
  assert.deepEqual(migrateDoc({ version: 1, checklists: 'no', runs: 'no' }).runs, [])
  assert.deepEqual(migrateDoc({ version: 2, checklists: [checklist()] }).checklists, [])
  assert.equal(migrateDoc({ version: 1, checklists: [checklist()], runs: [] }).checklists.length, 1)
})

test('migrateDoc drops a checklist with no name or no items, and a run with no items', () => {
  const doc = migrateDoc({
    version: 1,
    checklists: [
      { name: '', items: [{ text: 'x' }] },
      { name: 'No items', items: [] },
      { name: 'No items array', items: 'nope' },
      checklist(),
    ],
    runs: [{ ...workedRun(), items: [] }, workedRun()],
  })
  assert.equal(doc.checklists.length, 1)
  assert.equal(doc.runs.length, 1)
})

test('docWarning only fires for a document from the future', () => {
  assert.equal(docWarning({ version: 2 }), NEWER_VERSION_MESSAGE)
  assert.equal(docWarning({ version: 1 }), '')
  assert.equal(docWarning(null), '')
})

test('checklistKey ignores case and surrounding space', () => {
  assert.equal(checklistKey('  New PC Setup '), 'new pc setup')
})

test('mergeChecklists adds a checklist the machine has not seen', () => {
  const report = mergeChecklists([], { checklists: [checklist()] }, makeId, makeId, NOW)
  assert.equal(report.added, 1)
  assert.equal(report.checklists.length, 1)
  assert.equal(report.checklists[0].addedAt, NOW)
})

test('mergeChecklists leaves an identical checklist alone', () => {
  const current = [checklist()]
  const report = mergeChecklists(current, { checklists: [checklist()] }, makeId, makeId, NOW)
  assert.equal(report.added, 0)
  assert.equal(report.unchanged, 1)
  assert.equal(report.checklists.length, 1)
})

test('mergeChecklists keeps both when the items differ, and never touches the original', () => {
  const current = [checklist()]
  const incoming = {
    checklists: [checklist({ items: [{ id: 'x', text: 'A completely different step' }] })],
  }
  const report = mergeChecklists(current, incoming, makeId, makeId, NOW)

  assert.equal(report.added, 1)
  assert.equal(report.checklists.length, 2)
  const original = report.checklists.find((l) => l.name === 'New PC setup')
  const imported = report.checklists.find((l) => l.name === 'New PC setup (imported)')
  assert.equal(original?.items.length, 2)
  assert.equal(imported?.items[0].text, 'A completely different step')
  assert.equal(current[0].items.length, 2)
})

test('mergeChecklists skips one with no name or no items, and refuses a wrong file', () => {
  const report = mergeChecklists(
    [],
    { checklists: [{ name: '', items: [{ text: 'x' }] }, { name: 'Empty', items: [] }] },
    makeId,
    makeId,
    NOW,
  )
  assert.equal(report.added, 0)
  assert.equal(report.skipped, 2)

  const wrong = mergeChecklists([checklist()], { devices: [] }, makeId, makeId, NOW)
  assert.equal(wrong.error, NOT_A_CHECKLIST_FILE_MESSAGE)
  assert.equal(wrong.checklists.length, 1)
})

test('mergeChecklists is pure and importing twice adds nothing the second time', () => {
  const current = [checklist()]
  const incoming = { checklists: [checklist({ name: 'Office move' })] }
  const first = mergeChecklists(current, incoming, () => 'fixed', () => 'item', NOW)
  const second = mergeChecklists(current, incoming, () => 'fixed', () => 'item', NOW)
  assert.deepEqual(first.checklists, second.checklists)

  const again = mergeChecklists(first.checklists, incoming, makeId, makeId, NOW)
  assert.equal(again.added, 0)
  assert.equal(again.unchanged, 1)
})

test('sortRuns puts the most recently touched first', () => {
  const list = [
    { ...workedRun(), id: 'a', updatedAt: '2026-07-26T09:00:00.000Z' },
    { ...workedRun(), id: 'b', updatedAt: '2026-07-26T11:00:00.000Z' },
  ]
  assert.deepEqual(
    sortRuns(list).map((r) => r.id),
    ['b', 'a'],
  )
})

test('runLabel names the checklist, the site and when it started', () => {
  assert.equal(runLabel(workedRun()), 'New PC setup - Head Office - 2026-07-26 09:00')
  assert.equal(runLabel({ ...workedRun(), site: '' }), 'New PC setup - No site - 2026-07-26 09:00')
})

test('stampOf writes the local date and time, zero padded', () => {
  assert.equal(stampOf(new Date(2026, 6, 26, 9, 4)), '2026-07-26 09:04')
  assert.equal(stampOf(new Date(2026, 11, 1, 0, 0)), '2026-12-01 00:00')
})

test('the two export documents carry their exact kind strings', () => {
  assert.equal(exportChecklistsDoc([], NOW).kind, 'chit/site-checklists')
  assert.equal(exportRunsDoc([], NOW).kind, 'chit/site-checklist-runs')
  assert.equal(checklistsFileName(NOW), 'chit-checklists-2026-07-26.json')
  assert.equal(runsFileName(NOW), 'chit-runs-2026-07-26.json')
})

test('readImport refuses everything that is not a checklist file', () => {
  assert.equal(readImport('not json').ok, false)
  assert.equal(readImport('null').ok, false)
  assert.equal(readImport('{"version":1}').ok, false)

  const newer = readImport('{"version":2,"checklists":[]}')
  assert.equal(newer.ok, false)
  if (!newer.ok) assert.equal(newer.error, NEWER_FILE_MESSAGE)

  const tooBig = readImport('x'.repeat(5 * 1024 * 1024 + 1))
  assert.equal(tooBig.ok, false)
  if (!tooBig.ok) assert.equal(tooBig.error, TOO_BIG_MESSAGE)

  assert.equal(readImport(JSON.stringify(exportChecklistsDoc([checklist()], NOW))).ok, true)
})
