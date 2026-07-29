import test from 'node:test'
import assert from 'node:assert/strict'
import {
  docWarning,
  exportDoc,
  exportFileName,
  filterSnippets,
  groupNames,
  mergeSnippets,
  migrateDoc,
  normalizeTags,
  readImport,
  snippetKey,
  sortSnippets,
  starterSnippets,
  validateSnippet,
  BODY_LONG_MESSAGE,
  BODY_MESSAGE,
  EMPTY_DRAFT,
  GROUP_LONG_MESSAGE,
  NEWER_FILE_MESSAGE,
  NEWER_VERSION_MESSAGE,
  NOT_A_LIBRARY_MESSAGE,
  TITLE_LONG_MESSAGE,
  TITLE_MESSAGE,
  TOO_BIG_MESSAGE,
  type Snippet,
} from '../src/tools/snippet-library/snippets.ts'

const NOW = '2026-07-26T10:00:00.000Z'

let counter = 0
const makeId = () => `snip-test-${++counter}`

function snippet(over: Partial<Snippet> = {}): Snippet {
  return {
    id: 'snip-1',
    title: 'Flush the DNS cache',
    group: 'Windows',
    tags: ['dns'],
    body: 'ipconfig /flushdns',
    addedAt: NOW,
    updatedAt: NOW,
    ...over,
  }
}

test('validateSnippet accepts a full snippet and trims it', () => {
  const result = validateSnippet({
    title: '  Flush the DNS cache  ',
    group: ' Windows ',
    tags: 'DNS, cache, dns, ,',
    body: '  ipconfig /flushdns  ',
  })
  assert.equal(result.ok, true)
  if (!result.ok) return
  assert.equal(result.fields.title, 'Flush the DNS cache')
  assert.equal(result.fields.group, 'Windows')
  assert.deepEqual(result.fields.tags, ['dns', 'cache'])
  assert.equal(result.fields.body, 'ipconfig /flushdns')
})

test('validateSnippet needs a title and a body', () => {
  const empty = validateSnippet({ ...EMPTY_DRAFT })
  assert.equal(empty.ok, false)
  if (empty.ok) return
  assert.equal(empty.errors.title, TITLE_MESSAGE)
  assert.equal(empty.errors.body, BODY_MESSAGE)

  const whitespace = validateSnippet({ title: '   ', group: '', tags: '', body: '  \n  ' })
  assert.equal(whitespace.ok, false)
  if (whitespace.ok) return
  assert.equal(whitespace.errors.title, TITLE_MESSAGE)
  assert.equal(whitespace.errors.body, BODY_MESSAGE)
})

test('validateSnippet checks every length at the boundary', () => {
  const ok = validateSnippet({
    title: 'a'.repeat(80),
    group: 'g'.repeat(40),
    tags: '',
    body: 'b'.repeat(4000),
  })
  assert.equal(ok.ok, true)

  const tooLong = validateSnippet({
    title: 'a'.repeat(81),
    group: 'g'.repeat(41),
    tags: '',
    body: 'b'.repeat(4001),
  })
  assert.equal(tooLong.ok, false)
  if (tooLong.ok) return
  assert.equal(tooLong.errors.title, TITLE_LONG_MESSAGE)
  assert.equal(tooLong.errors.group, GROUP_LONG_MESSAGE)
  assert.equal(tooLong.errors.body, BODY_LONG_MESSAGE)
})

test('normalizeTags cleans, lower-cases and de-duplicates', () => {
  assert.deepEqual(normalizeTags('dns, Cache , DNS'), ['dns', 'cache'])
  assert.deepEqual(normalizeTags(''), [])
  assert.deepEqual(normalizeTags('  ,  ,'), [])
  assert.deepEqual(normalizeTags('one'), ['one'])
  assert.deepEqual(normalizeTags('canned reply, restart'), ['canned reply', 'restart'])
})

test('migrateDoc copes with everything a file on disk can be', () => {
  assert.deepEqual(migrateDoc(null).snippets, [])
  assert.deepEqual(migrateDoc('text').snippets, [])
  assert.deepEqual(migrateDoc({}).snippets, [])
  assert.deepEqual(migrateDoc({ version: 1 }).snippets, [])
  assert.deepEqual(migrateDoc({ version: 1, snippets: 'no' }).snippets, [])
  assert.deepEqual(migrateDoc({ version: 2, snippets: [snippet()] }).snippets, [])
  assert.deepEqual(migrateDoc({ version: 1, snippets: [null, 4, 'x'] }).snippets, [])
  assert.equal(migrateDoc({ version: 1, snippets: [snippet()] }).snippets.length, 1)
})

test('migrateDoc drops an entry with no title or no body, and tolerates missing tags', () => {
  const doc = migrateDoc({
    version: 1,
    snippets: [
      { title: 'No body' },
      { body: 'no title' },
      { title: 'Fine', body: 'arp -a' },
    ],
  })
  assert.equal(doc.snippets.length, 1)
  assert.equal(doc.snippets[0].title, 'Fine')
  assert.deepEqual(doc.snippets[0].tags, [])
})

test('docWarning only fires for a document from the future', () => {
  assert.equal(docWarning({ version: 2 }), NEWER_VERSION_MESSAGE)
  assert.equal(docWarning({ version: 1 }), '')
  assert.equal(docWarning(null), '')
})

test('starterSnippets are ten real, valid, uniquely identified snippets', () => {
  const seeds = starterSnippets(NOW)
  assert.equal(seeds.length, 10)

  const ids = new Set(seeds.map((s) => s.id))
  assert.equal(ids.size, 10)
  for (const seed of seeds) {
    assert.match(seed.id, /^snip-starter-\d+$/)
    assert.notEqual(seed.title, '')
    assert.notEqual(seed.body, '')
    assert.equal(seed.addedAt, NOW)
    const checked = validateSnippet({
      title: seed.title,
      group: seed.group,
      tags: seed.tags.join(','),
      body: seed.body,
    })
    assert.equal(checked.ok, true, `${seed.title} does not pass validation`)
  }
})

test('starterSnippets carry the exact commands the spec names', () => {
  const byTitle = new Map(starterSnippets(NOW).map((s) => [s.title, s]))
  assert.equal(byTitle.get('Flush the DNS cache')?.body, 'ipconfig /flushdns')
  assert.equal(byTitle.get('Force a Group Policy update')?.body, 'gpupdate /force')
  assert.equal(byTitle.get('Show the printer server properties')?.body, 'printui.exe /s /t2')
  assert.equal(byTitle.get('Restart the print spooler')?.body, 'net stop spooler\nnet start spooler')
  assert.equal(byTitle.get('Show the ARP table')?.group, 'Networking')
  assert.equal(byTitle.get('Ask a user to restart properly')?.group, 'Helpdesk')
})

test('snippetKey ignores case and surrounding space', () => {
  assert.equal(snippetKey({ group: ' Windows ', title: ' Flush ' }), 'windows|flush')
  assert.equal(snippetKey({ group: 'WINDOWS', title: 'FLUSH' }), 'windows|flush')
  assert.notEqual(snippetKey({ group: 'Linux', title: 'Flush' }), 'windows|flush')
})

test('mergeSnippets adds a snippet the library has not seen', () => {
  const report = mergeSnippets([], { snippets: [snippet({ id: '' })] }, makeId, NOW)
  assert.equal(report.added, 1)
  assert.equal(report.snippets.length, 1)
  assert.equal(report.snippets[0].addedAt, NOW)
  assert.notEqual(report.snippets[0].id, '')
})

test('mergeSnippets counts an identical snippet as unchanged', () => {
  const current = [snippet()]
  const report = mergeSnippets(current, { snippets: [snippet({ id: '' })] }, makeId, NOW)
  assert.equal(report.added, 0)
  assert.equal(report.unchanged, 1)
  assert.equal(report.snippets.length, 1)
})

test('mergeSnippets keeps both when the body differs, and never touches the original', () => {
  const current = [snippet({ body: 'ipconfig /flushdns' })]
  const incoming = { snippets: [snippet({ id: '', body: 'ipconfig /flushdns && ipconfig /registerdns' })] }
  const report = mergeSnippets(current, incoming, makeId, NOW)

  assert.equal(report.added, 1)
  assert.equal(report.snippets.length, 2)
  const original = report.snippets.find((s) => s.title === 'Flush the DNS cache')
  const imported = report.snippets.find((s) => s.title === 'Flush the DNS cache (imported)')
  assert.equal(original?.body, 'ipconfig /flushdns')
  assert.equal(imported?.body, 'ipconfig /flushdns && ipconfig /registerdns')
})

test('mergeSnippets unions the tags when the bodies are identical', () => {
  const current = [snippet({ tags: ['dns'] })]
  const report = mergeSnippets(
    current,
    { snippets: [snippet({ id: '', tags: ['dns', 'cache'] })] },
    makeId,
    NOW,
  )
  assert.equal(report.updated, 1)
  assert.deepEqual(report.snippets[0].tags, ['dns', 'cache'])
  // The caller's list must not have been mutated.
  assert.deepEqual(current[0].tags, ['dns'])
})

test('mergeSnippets skips an entry with no title or no body', () => {
  const report = mergeSnippets(
    [],
    { snippets: [{ title: 'No body' }, { body: 'no title' }, snippet({ id: '' })] },
    makeId,
    NOW,
  )
  assert.equal(report.added, 1)
  assert.equal(report.skipped, 2)
})

test('mergeSnippets refuses something that is not a library', () => {
  const report = mergeSnippets([snippet()], { devices: [] }, makeId, NOW)
  assert.equal(report.error, NOT_A_LIBRARY_MESSAGE)
  assert.equal(report.snippets.length, 1)
})

test('mergeSnippets is pure and importing twice adds nothing the second time', () => {
  const current = [snippet()]
  const incoming = { snippets: [snippet({ id: '', title: 'Show the ARP table', body: 'arp -a' })] }
  const first = mergeSnippets(current, incoming, () => 'snip-fixed', NOW)
  const second = mergeSnippets(current, incoming, () => 'snip-fixed', NOW)
  assert.deepEqual(first.snippets, second.snippets)

  const again = mergeSnippets(first.snippets, incoming, makeId, NOW)
  assert.equal(again.added, 0)
  assert.equal(again.snippets.length, 2)
})

test('filterSnippets searches the title, body, group and tags', () => {
  const list = [
    snippet({ id: '1' }),
    snippet({ id: '2', title: 'Show the ARP table', group: 'Networking', tags: ['arp'], body: 'arp -a' }),
  ]
  assert.equal(filterSnippets(list, 'flush', '').length, 1)
  assert.equal(filterSnippets(list, 'FLUSHDNS', '').length, 1)
  assert.equal(filterSnippets(list, 'arp', '').length, 1)
  assert.equal(filterSnippets(list, 'networking', '').length, 1)
  assert.equal(filterSnippets(list, '', '').length, 2)
  assert.equal(filterSnippets(list, '', 'Windows').length, 1)
  assert.equal(filterSnippets(list, 'arp', 'Windows').length, 0)
})

test('groupNames lists each group once, counting numbers properly', () => {
  const list = [
    snippet({ id: '1', group: 'Server 10' }),
    snippet({ id: '2', group: 'Server 2' }),
    snippet({ id: '3', group: 'Server 2' }),
    snippet({ id: '4', group: '' }),
  ]
  assert.deepEqual(groupNames(list), ['Server 2', 'Server 10'])
})

test('sortSnippets orders by group then title, numerically', () => {
  const list = [
    snippet({ id: '1', group: 'Windows', title: 'Step 10' }),
    snippet({ id: '2', group: 'Networking', title: 'Step 2' }),
    snippet({ id: '3', group: 'Windows', title: 'Step 2' }),
  ]
  assert.deepEqual(
    sortSnippets(list).map((s) => `${s.group}/${s.title}`),
    ['Networking/Step 2', 'Windows/Step 2', 'Windows/Step 10'],
  )
})

test('exportDoc names the format and exportFileName slugs the group', () => {
  const doc = exportDoc([snippet()], NOW)
  assert.equal(doc.kind, 'chit/snippet-library')
  assert.equal(doc.version, 1)
  assert.equal(exportFileName('Windows', NOW), 'chit-snippets-windows-2026-07-26.json')
  assert.equal(exportFileName('', NOW), 'chit-snippets-all-2026-07-26.json')
})

test('readImport refuses everything that is not a snippet library', () => {
  assert.equal(readImport('not json').ok, false)
  assert.equal(readImport('[]').ok, false)
  assert.equal(readImport('{"version":1}').ok, false)
  assert.equal(readImport('null').ok, false)

  const newer = readImport('{"version":2,"snippets":[]}')
  assert.equal(newer.ok, false)
  if (!newer.ok) assert.equal(newer.error, NEWER_FILE_MESSAGE)

  const tooBig = readImport('x'.repeat(5 * 1024 * 1024 + 1))
  assert.equal(tooBig.ok, false)
  if (!tooBig.ok) assert.equal(tooBig.error, TOO_BIG_MESSAGE)

  const good = readImport(JSON.stringify(exportDoc([snippet()], NOW)))
  assert.equal(good.ok, true)
})
