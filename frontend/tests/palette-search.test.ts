import test from 'node:test'
import assert from 'node:assert/strict'
import { paletteEntries, rankEntries, squash } from '../src/shell/search.ts'
import { TOOLS } from '../src/tools/registry.ts'

// Everything here runs against the real registry rather than a fixture, because
// the defect this module fixes only appears at 57 tools: cmdk matched a
// scattered subsequence, so "netcat" kept 58 of the 59 entries.
const ALL = paletteEntries()

function ids(query: string): string[] {
  return rankEntries(ALL, query).map((entry) => entry.id)
}

test('every entry in the registry reaches the palette, plus Home and Settings', () => {
  assert.equal(ALL.length, TOOLS.length + 2)
  assert.equal(ALL.length, 59)
  assert.deepEqual(ALL.slice(0, 2).map((entry) => entry.id), ['home', 'settings'])
})

test('an empty query returns everything in browse order', () => {
  assert.equal(rankEntries(ALL, '').length, 59)
  assert.equal(rankEntries(ALL, '   ').length, 59)
  // Browse order is the destinations, then CATEGORY_ORDER with names sorted.
  const network = ALL.filter((entry) => entry.group === 'Network').map((entry) => entry.name)
  assert.deepEqual(network, [...network].sort((a, b) => a.localeCompare(b)))
  assert.equal(ALL[2].group, 'Network')
})

// The invariant. Not "the right tool is in the results" (which passed happily in
// Phase 7 while 36 wrong ones were there too) but "the right tool is first".
test('typing a tool name with the spaces removed ranks that tool first, for all 57', () => {
  const wrong: string[] = []
  for (const entry of ALL) {
    const first = ids(squash(entry.name))[0]
    if (first !== entry.id) wrong.push(`${squash(entry.name)} -> ${first}, wanted ${entry.id}`)
  }
  assert.deepEqual(wrong, [])
})

test('typing a tool id ranks that tool first, for all 57', () => {
  const wrong: string[] = []
  for (const entry of ALL) {
    const first = ids(entry.id)[0]
    if (first !== entry.id) wrong.push(`${entry.id} -> ${first}`)
  }
  assert.deepEqual(wrong, [])
})

test('typing a tool name with its spaces ranks that tool first, for all 57', () => {
  const wrong: string[] = []
  for (const entry of ALL) {
    const first = ids(entry.name)[0]
    if (first !== entry.id) wrong.push(`${entry.name} -> ${first}`)
  }
  assert.deepEqual(wrong, [])
})

// The queries Phases 7, 8 and 9 measured as broken, with the counts written in.
test('netcat returns only the tool that has it as a keyword', () => {
  assert.deepEqual(ids('netcat'), ['port-listener'])
})

test('netcat does not match Subnet Calculator', () => {
  // "subNET CAlculaTor" contains netcat as a scattered subsequence. That single
  // match is why the old palette kept 58 of 59 entries for this query.
  assert.ok(!ids('netcat').includes('subnet-calculator'))
})

test('ntp ranks NTP Time Check first, where it used to rank seventh', () => {
  assert.deepEqual(ids('ntp'), ['ntp-check'])
})

test('pinout returns exactly one tool', () => {
  assert.deepEqual(ids('pinout'), ['reference-cards'])
})

test('568b returns exactly one tool', () => {
  assert.deepEqual(ids('568b'), ['reference-cards'])
})

test('the full name Raw Printer Test ranks it first, where it used to rank fourth', () => {
  assert.equal(ids('Raw Printer Test')[0], 'printer-test')
})

test('every token has to match, so "port listener" drops Port Scanner', () => {
  const hits = ids('port listener')
  assert.deepEqual(hits, ['port-listener'])
  assert.ok(!hits.includes('port-scanner'))
})

test('a broad query still returns the whole family, best first', () => {
  const hits = ids('dns')
  assert.equal(hits[0], 'dns-lookup')
  assert.equal(hits.length, 6)
  for (const id of ['dns-compare', 'email-dns', 'internet-triage', 'adapter-info', 'device-discovery']) {
    assert.ok(hits.includes(id), `${id} missing from a search for dns`)
  }
})

test('a query that matches nothing returns nothing', () => {
  assert.deepEqual(ids('zzzznotatool'), [])
})

test('a name prefix outranks an exact keyword on another tool', () => {
  // ip-range-scanner lists "subnet" as a keyword, so an exact-keyword-wins order
  // put it above both tools actually called Subnet something.
  const hits = ids('subnet')
  assert.equal(hits[0], 'subnet-calculator')
  assert.equal(hits[1], 'subnet-planner')
  assert.ok(hits.includes('ip-range-scanner'))
  assert.ok(hits.indexOf('ip-range-scanner') > 1)
})

test('two tools tied on score come out in name order', () => {
  const hits = rankEntries(ALL, 'subnet')
  assert.deepEqual(hits.slice(0, 2).map((entry) => entry.name), [
    'Subnet Calculator',
    'Subnet Planner',
  ])
})

test('an exact keyword still wins when no name starts with the query', () => {
  assert.equal(ids('nslookup')[0], 'dns-lookup')
  assert.equal(ids('ipconfig')[0], 'adapter-info')
})

test('a multi-word tool name beats keyword hits on another tool', () => {
  // "Listening Ports" scored 80 + 60 as two tokens while Port Scanner scored
  // 90 + 90 on two exact keywords, so the tool being named came second.
  assert.equal(ids('Listening Ports')[0], 'listening-ports')
  assert.equal(ids('Disk Speed Test')[0], 'disk-speed')
  assert.equal(ids('LAN File Drop')[0], 'lan-file-drop')
})

test('squash makes hyphens, spaces and case the same query', () => {
  assert.equal(squash('Port Listener'), 'portlistener')
  assert.equal(squash('port-listener'), 'portlistener')
  assert.equal(squash('LAN File Drop'), 'lanfiledrop')
  assert.deepEqual(ids('lan-file-drop'), ids('LAN File Drop'))
})

test('ranking is case insensitive', () => {
  assert.deepEqual(ids('NETCAT'), ids('netcat'))
})

test('no query returns more entries than exist', () => {
  for (const query of ['a', 'e', 'i', 'o', 's', 'the', 'network']) {
    assert.ok(rankEntries(ALL, query).length <= 59, `${query} returned more than 59`)
  }
})

// The tier tests below exist because mutation found each of these tiers could be
// changed to almost any value with the rest of the suite still green. Every one
// asserts a full ordering, not just presence.

test('a word of the name beats a keyword or description match elsewhere', () => {
  // "listener" is not a prefix of "portlistener", it is a prefix of the second
  // word, which is the NAME_WORD_PREFIX tier and nothing else exercised it.
  assert.deepEqual(ids('listener'), ['port-listener'])
  assert.equal(ids('planner')[0], 'subnet-planner')
  assert.equal(ids('explainer')[0], 'cron-explainer')
  assert.deepEqual(ids('scanner'), ['ip-range-scanner', 'port-scanner'])
})

test('a substring inside a name beats a description match', () => {
  // "oughput" is inside "lanthroughputtest" and starts no word of it.
  assert.equal(ids('oughput')[0], 'lan-throughput')
  const hits = ids('throughput')
  assert.equal(hits[0], 'lan-throughput')
  // disk-speed and speed-test only mention throughput in their description.
  assert.ok(hits.indexOf('disk-speed') > 0)
})

test('a description match ranks below every name and keyword match', () => {
  assert.deepEqual(ids('sweep'), ['ip-range-scanner'])
  assert.deepEqual(ids('treemap'), ['disk-visualizer'])
  // "cheat" is in two descriptions and no name, so both rank purely on it.
  assert.deepEqual(ids('cheat'), ['reference-cards', 'snippet-library'])
})

test('a category label match ranks below everything else', () => {
  const hits = ids('diagnostics')
  // Every diagnostics tool matches on its group, so the ones that also match on
  // a name or keyword have to come first.
  assert.equal(hits[0], 'dns-lookup')
  assert.ok(hits.length > 8, 'a category query should return the whole category')
  assert.ok(hits.includes('ntp-check'))
})

test('a partial id prefix matches even when the name does not contain it', () => {
  // The tool is called "Website / Service Up Checker": nothing in the name
  // starts with "uptime", only the id does.
  assert.equal(ids('uptime')[0], 'uptime-ssl-checker')
  assert.deepEqual(ids('csv'), ['csv-json-viewer'])
  assert.deepEqual(ids('totp'), ['totp-generator'])
  assert.equal(ids('wol')[0], 'wake-on-lan')
})

// Two-word queries a tech actually types. These pin the tiers against each other
// across tokens, and they are what showed the whole-phrase bonus an earlier draft
// carried was making four of these five answers worse, not better.
test('a two-word query lands on the tool the words name', () => {
  assert.equal(ids('usb stick')[0], 'usb-history')
  assert.equal(ids('disk full')[0], 'disk-visualizer')
  assert.equal(ids('disk space')[0], 'disk-visualizer')
  assert.equal(ids('wifi speed')[0], 'wifi-info')
  assert.equal(ids('network share')[0], 'lan-file-drop')
  assert.equal(ids('listening ports')[0], 'listening-ports')
})

test('a description match never outranks a name or keyword match', () => {
  // "network" is a name word of nothing, a keyword of several tools, a
  // description word of more, and the label of a whole category. Raising the
  // description tier reorders this list; the assertion is the full top three.
  assert.deepEqual(ids('network').slice(0, 3), ['adapter-info', 'disk-speed', 'lan-file-drop'])
  assert.deepEqual(ids('device').slice(0, 3), ['device-discovery', 'device-inventory', 'usb-history'])
})

test('a category label match never outranks anything else', () => {
  // Every tool in the category matches on its label, so if GROUP_CONTAINS were
  // raised, the ones that also match by keyword would be pushed down.
  assert.deepEqual(ids('data').slice(0, 3), ['settings', 'bulk-renamer', 'csv-json-viewer'])
  assert.deepEqual(ids('at').slice(0, 3), ['battery-health', 'cert-generator', 'cert-decoder'])
})

test('a word of the description beats a bare description substring', () => {
  // These three are the only queries in a 1,709 query corpus where this rule
  // decides the top result, which is why they look arbitrary and are written in
  // with their reason. "fit" starts a word of Subnet Planner's description
  // ("fit"), and only appears mid-word in Battery Health's ("benefit" style).
  assert.equal(ids('fit')[0], 'subnet-planner')
  assert.equal(ids('rate')[0], 'breach-checker')
  assert.equal(ids('team')[0], 'snippet-library')
  // And the ordering below the top, which is where most of the rule's effect is.
  assert.deepEqual(ids('for').slice(0, 3), ['ticket-notes', 'port-listener', 'text-diff'])
  assert.deepEqual(ids('on').slice(0, 3), ['wake-on-lan', 'site-checklist', 'cron-explainer'])
})
