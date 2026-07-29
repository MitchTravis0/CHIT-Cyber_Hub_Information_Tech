import test from 'node:test'
import assert from 'node:assert/strict'
import {
  CARDS,
  downtimeSeconds,
  formatSpan,
  MAX_SEARCH_HITS,
  searchCards,
  type CardDef,
  type CardEntry,
  type CardId,
} from '../src/tools/reference-cards/cards.ts'

const IDS: CardId[] = [
  'rj45',
  'ports',
  'http-status',
  'wifi-channels',
  'subnet-table',
  'beep-codes',
  'sla',
]

function card(id: CardId): CardDef {
  const found = CARDS.find((item) => item.id === id)
  assert.ok(found !== undefined, `no card ${id}`)
  return found
}

function column(id: CardId, header: string): number {
  const index = card(id).columns.indexOf(header)
  assert.notEqual(index, -1, `${id} has no column ${header}`)
  return index
}

/** Column 0 is the key, column 1 is the label, the rest are extra. */
function valueAt(row: CardEntry, index: number): string {
  return index === 0 ? row.key : index === 1 ? row.label : row.extra[index - 2]
}

function cell(id: CardId, key: string, header: string): string {
  const found = card(id).entries.find((row) => row.key === key)
  assert.ok(found !== undefined, `${id} has no row ${key}`)
  return valueAt(found, column(id, header))
}

test('every card is well formed', () => {
  assert.equal(CARDS.length, 7)
  assert.deepEqual(CARDS.map((item) => item.id), IDS)
  assert.equal(new Set(CARDS.map((item) => item.id)).size, 7)
  for (const item of CARDS) {
    assert.ok(item.name.length > 0, `${item.id} name`)
    assert.ok(item.blurb.length > 0, `${item.id} blurb`)
    assert.ok(item.icon.length > 0, `${item.id} icon`)
    assert.ok(item.columns.length >= 2, `${item.id} columns`)
    assert.ok(item.entries.length >= 4, `${item.id} entries`)
    assert.ok(item.keywords.length > 0, `${item.id} keywords`)
  }
})

test('every entry lines up with its columns', () => {
  for (const item of CARDS) {
    for (const row of item.entries) {
      assert.equal(
        row.extra.length,
        item.columns.length - 2,
        `${item.id} row ${row.key} has ${row.extra.length} extra cells for ${item.columns.length} columns`,
      )
    }
  }
})

test('entry ids are unique across every card', () => {
  const ids = CARDS.flatMap((item) => item.entries.map((row) => row.id))
  assert.equal(new Set(ids).size, ids.length)
})

test('the haystack is derived, not typed', () => {
  for (const item of CARDS) {
    for (const row of item.entries) {
      assert.equal(row.haystack, [row.key, row.label, ...row.extra].join(' ').toLowerCase())
    }
  }
})

test('the RJ45 card is the standard', () => {
  const rj45 = card('rj45')
  assert.deepEqual(rj45.entries.map((row) => row.key), ['1', '2', '3', '4', '5', '6', '7', '8'])
  assert.equal(cell('rj45', '1', 'T568B'), 'White/Orange')
  assert.equal(cell('rj45', '1', 'T568A'), 'White/Green')
  assert.equal(cell('rj45', '2', 'T568B'), 'Orange')
  assert.equal(cell('rj45', '3', 'T568B'), 'White/Green')
  assert.equal(cell('rj45', '3', 'T568A'), 'White/Orange')
  assert.equal(cell('rj45', '6', 'T568B'), 'Green')
  assert.equal(cell('rj45', '6', 'T568A'), 'Orange')
  for (const pin of ['4', '5', '7', '8']) {
    assert.equal(cell('rj45', pin, 'T568B'), cell('rj45', pin, 'T568A'), `pin ${pin}`)
  }
  assert.equal(cell('rj45', '1', '10/100 signal'), 'Transmit +')
  assert.equal(cell('rj45', '3', '10/100 signal'), 'Receive +')
})

test('swapping B for A swaps exactly two pairs', () => {
  const differ = card('rj45').entries.filter((row) => row.label !== row.extra[0])
  assert.deepEqual(differ.map((row) => row.key), ['1', '2', '3', '6'])
  // Orange and green change places, nothing else moves.
  const swap = (name: string) =>
    name.replace('Orange', 'X').replace('Green', 'Orange').replace('X', 'Green')
  for (const row of card('rj45').entries) {
    assert.equal(swap(row.label), row.extra[0], `pin ${row.key}`)
  }
})

// Every port below was confirmed present in /etc/services (IANA sourced) while
// this card was written.
test('the port list is sorted and named', () => {
  const ports = card('ports').entries
  assert.ok(ports.length >= 45, `only ${ports.length} ports`)
  const numbers = ports.map((row) => Number(row.key))
  assert.ok(numbers.every((n) => Number.isInteger(n) && n >= 1 && n <= 65535))
  for (let i = 1; i < numbers.length; i++) {
    assert.ok(numbers[i] > numbers[i - 1], `${numbers[i]} is out of order`)
  }
  assert.equal(cell('ports', '3389', 'Service'), 'Remote Desktop')
  assert.equal(cell('ports', '3268', 'Service'), 'Global Catalog')
  assert.equal(cell('ports', '9100', 'Service'), 'Raw printing (JetDirect)')
  assert.equal(cell('ports', '445', 'Service'), 'SMB file sharing')
  assert.equal(cell('ports', '22', 'Service'), 'SSH and SFTP')
})

// Every meaning below is python's http.HTTPStatus phrase for that code.
test('the status code list covers the classes', () => {
  assert.equal(cell('http-status', '200', 'Meaning'), 'OK')
  assert.equal(cell('http-status', '301', 'Meaning'), 'Moved Permanently')
  assert.equal(cell('http-status', '404', 'Meaning'), 'Not Found')
  assert.equal(cell('http-status', '502', 'Meaning'), 'Bad Gateway')
  assert.equal(cell('http-status', '200', 'Class'), 'Success')
  assert.equal(cell('http-status', '301', 'Class'), 'Redirect')
  assert.equal(cell('http-status', '404', 'Class'), 'Client error')
  assert.equal(cell('http-status', '502', 'Class'), 'Server error')

  const expected: Record<string, string> = {
    '1': 'Informational',
    '2': 'Success',
    '3': 'Redirect',
    '4': 'Client error',
    '5': 'Server error',
  }
  const classIndex = column('http-status', 'Class')
  for (const row of card('http-status').entries) {
    assert.match(row.key, /^[1-5]\d\d$/)
    assert.equal(valueAt(row, classIndex), expected[row.key[0]], `code ${row.key}`)
  }
})

test('2.4 GHz frequencies are generated', () => {
  const bandIndex = column('wifi-channels', 'Band')
  const freqIndex = column('wifi-channels', 'Centre frequency')
  const rows = card('wifi-channels').entries.filter((row) => valueAt(row, bandIndex) === '2.4 GHz')
  assert.equal(rows.length, 14)
  for (const row of rows) {
    const channel = Number(row.key)
    const expected = channel === 14 ? 2484 : 2407 + 5 * channel
    assert.equal(valueAt(row, freqIndex), `${expected} MHz`, `channel ${channel}`)
  }
  assert.equal(cell('wifi-channels', '1', 'Centre frequency'), '2412 MHz')
  assert.equal(cell('wifi-channels', '13', 'Centre frequency'), '2472 MHz')
  assert.equal(cell('wifi-channels', '14', 'Centre frequency'), '2484 MHz')
})

test('5 GHz frequencies are generated', () => {
  const bandIndex = column('wifi-channels', 'Band')
  const freqIndex = column('wifi-channels', 'Centre frequency')
  const rows = card('wifi-channels').entries.filter((row) => valueAt(row, bandIndex) === '5 GHz')
  assert.equal(rows.length, 25)
  for (const row of rows) {
    assert.equal(valueAt(row, freqIndex), `${5000 + 5 * Number(row.key)} MHz`, `channel ${row.key}`)
  }
  assert.equal(cell('wifi-channels', '36', 'Centre frequency'), '5180 MHz')
  assert.equal(cell('wifi-channels', '165', 'Centre frequency'), '5825 MHz')
})

test('the DFS range is marked', () => {
  const bandIndex = column('wifi-channels', 'Band')
  const noteIndex = column('wifi-channels', 'Notes')
  for (const row of card('wifi-channels').entries) {
    if (valueAt(row, bandIndex) !== '5 GHz') continue
    const channel = Number(row.key)
    const dfs = channel >= 52 && channel <= 144
    assert.equal(
      valueAt(row, noteIndex).includes('DFS'),
      dfs,
      `channel ${channel} DFS marking is wrong`,
    )
  }
})

test('the subnet table is computed', () => {
  const rows = card('subnet-table').entries
  assert.equal(rows.length, 25)
  const wildIndex = column('subnet-table', 'Wildcard')
  const addrIndex = column('subnet-table', 'Addresses')
  const hostIndex = column('subnet-table', 'Usable hosts')

  const dotted = (value: number) => [24, 16, 8, 0].map((s) => (value >>> s) & 255).join('.')
  const grouped = (value: number) => String(value).replace(/\B(?=(\d{3})+(?!\d))/g, ',')

  for (let i = 0; i < rows.length; i++) {
    const bits = i + 8
    const row = rows[i]
    assert.equal(row.key, `/${bits}`)
    const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0
    assert.equal(row.label, dotted(mask), `mask for /${bits}`)
    assert.equal(valueAt(row, wildIndex), dotted(~mask >>> 0), `wildcard for /${bits}`)
    const addresses = 2 ** (32 - bits)
    assert.equal(valueAt(row, addrIndex), grouped(addresses), `addresses for /${bits}`)
    const usable = bits <= 30 ? addresses - 2 : bits === 31 ? 2 : 1
    assert.equal(valueAt(row, hostIndex), grouped(usable), `usable for /${bits}`)
  }
  assert.equal(cell('subnet-table', '/24', 'Mask'), '255.255.255.0')
  assert.equal(cell('subnet-table', '/24', 'Wildcard'), '0.0.0.255')
  assert.equal(cell('subnet-table', '/24', 'Addresses'), '256')
  assert.equal(cell('subnet-table', '/24', 'Usable hosts'), '254')
  assert.equal(cell('subnet-table', '/8', 'Usable hosts'), '16,777,214')
})

test('downtimeSeconds is the plain arithmetic', () => {
  // Percentages like 99.9 are not exact in binary, so these compare to a
  // hundredth of a second rather than to the bit.
  const near = (actual: number, expected: number) =>
    assert.ok(Math.abs(actual - expected) < 0.01, `${actual} is not about ${expected}`)
  near(downtimeSeconds(99.9, 86400 * 365), 31536)
  assert.equal(downtimeSeconds(100, 86400 * 365), 0)
  near(downtimeSeconds(90, 86400), 8640)
  near(downtimeSeconds(99.999, 86400 * 365), 315.36)
  near(downtimeSeconds(99.99, 86400 * 30), 259.2)
})

test('formatSpan reads in plain units', () => {
  assert.equal(formatSpan(0), 'none')
  assert.equal(formatSpan(0.4), 'under 1s')
  assert.equal(formatSpan(1), '1s')
  assert.equal(formatSpan(59), '59s')
  assert.equal(formatSpan(60), '1m')
  assert.equal(formatSpan(61), '1m 1s')
  assert.equal(formatSpan(3600), '1h')
  assert.equal(formatSpan(3661), '1h 1m 1s')
  assert.equal(formatSpan(86400), '1d')
  assert.equal(formatSpan(31536), '8h 45m 36s')
  assert.equal(formatSpan(315.36), '5m 15s')
})

test('the SLA card is computed from the percentages', () => {
  const rows = card('sla').entries
  assert.equal(rows.length, 9)
  const percents = [90, 95, 98, 99, 99.5, 99.9, 99.95, 99.99, 99.999]
  const windows = [86400, 86400 * 7, 86400 * 30, 86400 * 365]
  for (let i = 0; i < rows.length; i++) {
    assert.equal(rows[i].key, `${percents[i]}%`)
    const cells = [rows[i].label, ...rows[i].extra]
    for (let w = 0; w < windows.length; w++) {
      assert.equal(
        cells[w],
        formatSpan(downtimeSeconds(percents[i], windows[w])),
        `${percents[i]}% over window ${w}`,
      )
    }
  }
  assert.equal(cell('sla', '99.9%', 'Per year'), '8h 45m 36s')
  assert.equal(cell('sla', '99.9%', 'Per month'), '43m 12s')
})

test('searchCards finds a row by its key', () => {
  const rdp = searchCards('3389')
  assert.equal(rdp.length, 1)
  assert.equal(rdp[0].card.id, 'ports')
  assert.equal(rdp[0].entry.label, 'Remote Desktop')

  const notFound = searchCards('404')
  assert.ok(notFound.some((hit) => hit.card.id === 'http-status' && hit.entry.key === '404'))
  assert.ok(!notFound.some((hit) => hit.card.id === 'ports'))
})

test('searchCards finds a card by its keywords', () => {
  assert.equal(searchCards('568b').length, 8)
  assert.ok(searchCards('568b').every((hit) => hit.card.id === 'rj45'))
  assert.equal(searchCards('nines').length, 9)
  assert.ok(searchCards('nines').every((hit) => hit.card.id === 'sla'))
})

test('a card keyword never buries the row that answers the query', () => {
  // "404" is in the status card's keyword list and is also a row key. Without
  // the fallback rule it matches the card and returns all 37 rows, burying the
  // one the tech asked for. Same for "dfs" and "dell".
  const notFound = searchCards('404')
  assert.equal(notFound.length, 1)
  assert.equal(notFound[0].entry.key, '404')

  const dell = searchCards('dell')
  assert.ok(dell.length > 0 && dell.length < card('beep-codes').entries.length)
  assert.ok(dell.every((hit) => hit.entry.haystack.includes('dell')))

  const dfs = searchCards('dfs')
  assert.ok(dfs.length > 0 && dfs.length < card('wifi-channels').entries.length)
  assert.ok(dfs.every((hit) => hit.entry.haystack.includes('dfs')))

  // The invariant behind all three: when any row of a card answers a term, the
  // card's own keywords must not widen the result to the rest of the card.
  for (const item of CARDS) {
    for (const keyword of item.keywords) {
      const rows = item.entries.filter((row) => row.haystack.includes(keyword))
      if (rows.length === 0) continue
      const hits = searchCards(keyword).filter((hit) => hit.card.id === item.id)
      assert.equal(
        hits.length,
        Math.min(rows.length, MAX_SEARCH_HITS),
        `"${keyword}" on ${item.id} returned ${hits.length} rows but only ${rows.length} contain it`,
      )
    }
  }
})

test('searchCards requires every term', () => {
  const dfs = searchCards('dfs 100')
  assert.ok(dfs.length > 0)
  assert.ok(dfs.every((hit) => hit.entry.haystack.includes('dfs')))
  assert.ok(dfs.every((hit) => hit.entry.haystack.includes('100')))
  assert.deepEqual(searchCards('crossover 3389'), [])
})

test('searchCards ignores case and surrounding space', () => {
  const a = searchCards('  RDP ').map((hit) => hit.entry.id)
  const b = searchCards('rdp').map((hit) => hit.entry.id)
  const c = searchCards('RdP').map((hit) => hit.entry.id)
  assert.deepEqual(a, b)
  assert.deepEqual(b, c)
})

test('searchCards treats the query as text, not a pattern', () => {
  // As a regular expression every one of these would match nearly everything.
  // As plain text none of them appears anywhere, so all three come back empty.
  assert.deepEqual(searchCards('.*'), [])
  assert.deepEqual(searchCards('[0-9]'), [])
  assert.deepEqual(searchCards('^1'), [])
})

test('searchCards returns nothing for an empty query', () => {
  assert.deepEqual(searchCards(''), [])
  assert.deepEqual(searchCards('   '), [])
})

test('searchCards caps its results', () => {
  assert.equal(MAX_SEARCH_HITS, 100)
  const many = searchCards('e')
  assert.equal(many.length, 100)
})
