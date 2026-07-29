// The CSV / JSON viewer parses and converts entirely in the browser, so these
// pure modules are the whole of its behaviour. Every fixture is a string literal
// with explicit \r\n or \n so the suite does not depend on this machine.
import test from 'node:test'
import assert from 'node:assert/strict'
import { detectDelimiter, formatCsv, parseCsv } from '../src/tools/csv-json-viewer/csv.ts'
import { formatJson, parseJson } from '../src/tools/csv-json-viewer/json.ts'
import {
  columnKey,
  csvNameFor,
  detectFormat,
  hasDuplicateHeaders,
  parseInput,
} from '../src/tools/csv-json-viewer/table.ts'

/** The parsed table, failing the test with the error when the parse rejected. */
function csv(text, delimiter = ',', header = true) {
  const result = parseCsv(text, delimiter, header)
  assert.equal(result.ok, true, result.error)
  return result.table
}

function json(text) {
  const result = parseJson(text)
  assert.equal(result.ok, true, result.error)
  return result.table
}

const RAGGED_NOTE_ONE =
  '1 row did not have the same number of values as the header row. Short rows were padded with empty cells, and extra values were kept in extra columns named after their position. Nothing was thrown away.'
const UNCLOSED_NOTE =
  'The last value in the file starts with a quote that is never closed. Everything after it was read as one value.'
const NESTED_NOTE =
  'Some values were lists or nested records. They are shown as the JSON text they came from, and converting back to JSON will make them plain text rather than lists again.'
const NULL_NOTE =
  'Some values were empty (JSON null). They show as blank cells, and converting back to JSON will write them as empty text rather than null.'
const INVALID_JSON =
  'That is not valid JSON. Check for a missing comma, a bracket that was never closed, or a comma just before a closing bracket or brace.'
const NOT_A_LIST =
  'That JSON is a single value, not a list of records, so there is nothing to put in a table. This tool expects something like [{"name":"PC01"},{"name":"PC02"}].'

test('detectFormat picks the parser', () => {
  const cases: Array<[string, string]> = [
    ['', 'csv'],
    ['   ', 'csv'],
    ['[{"a":1}]', 'json'],
    ['  \n\t{"a":1}', 'json'],
    ['a,b', 'csv'],
    ['{not json', 'json'],
    ['"a",b', 'csv'],
  ]
  for (const [text, want] of cases) {
    assert.equal(detectFormat(text), want, JSON.stringify(text))
  }
})

test('detectDelimiter picks the separator', () => {
  const cases: Array<[string, string]> = [
    ['a,b,c', ','],
    ['a;b;c', ';'],
    ['a\tb\tc', '\t'],
    ['"a,b";c;d', ';'],
    ['a,b;c', ','],
    ['ab', ','],
    ['a,b\nc;d;e;f', ','],
    // Only the first 4096 characters of the first line count, so the commas
    // past that are never seen and the single semicolon wins.
    [`;${'x'.repeat(4200)},,,,`, ';'],
  ]
  for (const [text, want] of cases) {
    assert.equal(detectDelimiter(text), want, JSON.stringify(text.slice(0, 20)))
  }
})

test('parseCsv follows RFC 4180', () => {
  const cases: Array<[string, string[][]]> = [
    ['a,b,c', [['a', 'b', 'c']]],
    ['"a","b"', [['a', 'b']]],
    ['a"b,c', [['a"b', 'c']]],
    ['"a""b"', [['a"b']]],
    ['"a,b",c', [['a,b', 'c']]],
    ['"line1\nline2",c', [['line1\nline2', 'c']]],
    ['"line1\r\nline2"', [['line1\nline2']]],
    ['a,b\r\nc,d', [['a', 'b'], ['c', 'd']]],
    ['a,b\nc,d', [['a', 'b'], ['c', 'd']]],
    ['a,b\rc,d', [['a', 'b'], ['c', 'd']]],
    ['a,b\n', [['a', 'b']]],
    ['a,b\n\n\n', [['a', 'b']]],
    ['a,b', [['a', 'b']]],
    ['a,,c', [['a', '', 'c']]],
    [',a', [['', 'a']]],
    ['a,', [['a', '']]],
    ['"abc', [['abc']]],
    ['"a" x,b', [['a x', 'b']]],
  ]
  for (const [text, want] of cases) {
    assert.deepEqual(csv(text, ',', false).rows, want, JSON.stringify(text))
  }
  assert.deepEqual(csv('', ',', false), { headers: [], rows: [], notes: [] })
  assert.deepEqual(csv('   \n', ',', false), { headers: [], rows: [], notes: [] })
})

test('parseCsv handles line endings', () => {
  assert.deepEqual(csv('a,b\r\nc,d\r\ne,f', ',', false).rows, [
    ['a', 'b'],
    ['c', 'd'],
    ['e', 'f'],
  ])
  assert.deepEqual(csv('a,b\nc,d\ne,f', ',', false).rows, [
    ['a', 'b'],
    ['c', 'd'],
    ['e', 'f'],
  ])
  assert.deepEqual(csv('a,b\rc,d\re,f', ',', false).rows, [
    ['a', 'b'],
    ['c', 'd'],
    ['e', 'f'],
  ])
  assert.deepEqual(csv('a,b\nc,d\n', ',', false).rows, [
    ['a', 'b'],
    ['c', 'd'],
  ])
  assert.deepEqual(csv('a,b\r\nc,d\r\n', ',', false).rows, [
    ['a', 'b'],
    ['c', 'd'],
  ])
  assert.deepEqual(csv('a,b\r\nc,d', ',', false).rows, [
    ['a', 'b'],
    ['c', 'd'],
  ])
  assert.deepEqual(csv('"one\r\ntwo",b', ',', false).rows, [['one\ntwo', 'b']])
})

test('parseCsv skips blank lines but not whitespace lines', () => {
  assert.deepEqual(csv('a\n\n\nb', ',', false).rows, [['a'], ['b']])
  assert.deepEqual(csv('a\n \nb', ',', false).rows, [['a'], [' '], ['b']])
})

test('parseCsv names headers', () => {
  assert.deepEqual(csv('name,email\nPC01,a@b.c').headers, ['name', 'email'])
  assert.deepEqual(csv('name,,email\n1,2,3').headers, ['name', 'Column 2', 'email'])

  const duplicate = csv('Status,Status\nup,down')
  assert.deepEqual(duplicate.headers, ['Status', 'Status'])
  assert.deepEqual(duplicate.rows, [['up', 'down']])

  const noHeader = csv('a,b\nc,d', ',', false)
  assert.deepEqual(noHeader.headers, ['Column 1', 'Column 2'])
  assert.deepEqual(noHeader.rows, [
    ['a', 'b'],
    ['c', 'd'],
  ])
})

test('parseCsv pads and extends ragged rows', () => {
  const short = csv('a,b,c\n1,2')
  assert.deepEqual(short.rows, [['1', '2', '']])
  assert.deepEqual(short.notes, [RAGGED_NOTE_ONE])

  const long = csv('a,b,c\n1,2,3,4')
  assert.deepEqual(long.headers, ['a', 'b', 'c', 'Column 4'])
  assert.deepEqual(long.rows, [['1', '2', '3', '4']])

  const both = csv('a,b,c\n1,2\n1,2,3,4\n5,6,7')
  assert.deepEqual(both.headers, ['a', 'b', 'c', 'Column 4'])
  assert.deepEqual(both.rows, [
    ['1', '2', '', ''],
    ['1', '2', '3', '4'],
    ['5', '6', '7', ''],
  ])
  // Two rows differ from the header row: the last one has three values like the
  // header and is only padded because another row was wider.
  assert.equal(both.notes.length, 1)
  assert.match(both.notes[0], /^2 rows did not have the same number of values/)

  assert.deepEqual(csv('a,b\n1,2').notes, [])
})

test('parseCsv on empty input', () => {
  for (const text of ['', '   ']) {
    const result = parseCsv(text, ',', true)
    assert.equal(result.ok, true)
    assert.deepEqual(result.table, { headers: [], rows: [], notes: [] })
  }
})

test('parseCsv on an unclosed quote', () => {
  const table = csv('name,note\nPC01,"never closed, honest\nstill going')
  assert.deepEqual(table.rows, [['PC01', 'never closed, honest\nstill going']])
  assert.deepEqual(table.notes, [UNCLOSED_NOTE])
})

test('parseJson reads the four shapes', () => {
  const objects = json('[{"a":1},{"a":2}]')
  assert.deepEqual(objects.headers, ['a'])
  assert.deepEqual(objects.rows, [['1'], ['2']])

  const bare = json('{"a":1,"b":2}')
  assert.deepEqual(bare.headers, ['a', 'b'])
  assert.deepEqual(bare.rows, [['1', '2']])

  const scalars = json('[1,"a",true]')
  assert.deepEqual(scalars.headers, ['value'])
  assert.deepEqual(scalars.rows, [['1'], ['a'], ['true']])

  const mixed = json('[{"a":1},"x"]')
  assert.deepEqual(mixed.headers, ['value'])
  assert.deepEqual(mixed.rows, [['{"a":1}'], ['x']])
})

test('parseJson unions keys in first-seen order', () => {
  const table = json('[{"b":1},{"a":2,"c":3}]')
  assert.deepEqual(table.headers, ['b', 'a', 'c'])
  assert.deepEqual(table.rows, [
    ['1', '', ''],
    ['', '2', '3'],
  ])
})

test('parseJson stringifies nested values', () => {
  const table = json('[{"tags":["a","b"],"who":{"name":"PC01"}}]')
  assert.deepEqual(table.rows, [['["a","b"]', '{"name":"PC01"}']])
  assert.deepEqual(table.rows[0][0], JSON.stringify(['a', 'b']))
  assert.deepEqual(table.notes, [NESTED_NOTE])
})

test('parseJson renders scalars', () => {
  const table = json('[{"s":"text","a":1.50,"b":1e3,"c":true,"d":false,"e":null}]')
  assert.deepEqual(table.rows, [['text', '1.5', '1000', 'true', 'false', '']])
  assert.deepEqual(table.notes, [NULL_NOTE])
})

test('parseJson rejects what it cannot table', () => {
  for (const text of ['5', '"hi"', 'true', 'null']) {
    const result = parseJson(text)
    assert.equal(result.ok, false, text)
    assert.equal(result.error, NOT_A_LIST)
  }
  for (const text of ['{oops', '[1,]']) {
    const result = parseJson(text)
    assert.equal(result.ok, false, text)
    assert.equal(result.error, INVALID_JSON)
  }
})

test('parseJson on empty input', () => {
  for (const text of ['', '   ', '[]', '{}']) {
    const result = parseJson(text)
    assert.equal(result.ok, true, text)
    assert.deepEqual(result.table.headers, [])
    assert.deepEqual(result.table.rows, [])
  }
})

test('formatCsv quotes only what needs quoting', () => {
  const table = {
    headers: ['plain', 'comma', 'quote', 'lf', 'crlf', 'empty', 'formula'],
    rows: [['a', 'a,b', 'say "hi"', 'one\ntwo', 'one\r\ntwo', '', '=1+1']],
    notes: [],
  }
  const out = formatCsv(table)
  assert.equal(
    out,
    'plain,comma,quote,lf,crlf,empty,formula\r\na,"a,b","say ""hi""","one\ntwo","one\r\ntwo",,=1+1\r\n',
  )
  assert.ok(out.endsWith('\r\n'))
  // The Excel formula guard is ResultsTable's job, not this pane's.
  assert.ok(out.includes(',=1+1'))
})

test('formatCsv on an empty table', () => {
  assert.equal(formatCsv({ headers: [], rows: [], notes: [] }), '')
})

test('formatJson writes strings', () => {
  const out = formatJson({ headers: ['n', 'blank'], rows: [['42', '']], notes: [] })
  assert.equal(out, '[\n  {\n    "n": "42",\n    "blank": ""\n  }\n]')
  assert.equal(formatJson({ headers: [], rows: [], notes: [] }), '[]')
})

test('formatJson with duplicate headers', () => {
  assert.ok(hasDuplicateHeaders(['Status', 'Status']))
  assert.ok(!hasDuplicateHeaders(['Status', 'Other']))
  const out = formatJson({ headers: ['Status', 'Status'], rows: [['up', 'down']], notes: [] })
  assert.equal(out, '[\n  {\n    "Status": "down"\n  }\n]')
})

test('csv to json to csv round trips', () => {
  const original =
    'name,note\r\nPC01,"a,b"\r\nPC02,"say ""hi"""\r\nPC03,"line1\nline2"\r\n'
  const first = csv(original)
  const back = parseJson(formatJson(first))
  assert.equal(back.ok, true)
  assert.deepEqual(back.table.headers, first.headers)
  assert.deepEqual(back.table.rows, first.rows)
  assert.equal(formatCsv(back.table), original)
})

test('parseInput follows the format and delimiter choices', () => {
  const detected = parseInput('a;b\n1;2', 'auto', 'auto', true)
  assert.equal(detected.format, 'csv')
  assert.equal(detected.delimiter, ';')
  assert.deepEqual(detected.result.table.headers, ['a', 'b'])

  // Forcing the comma leaves the semicolons inside one column.
  const forced = parseInput('a;b\n1;2', 'csv', ',', true)
  assert.deepEqual(forced.result.table.headers, ['a;b'])

  const asJson = parseInput('[{"a":1}]', 'auto', 'auto', true)
  assert.equal(asJson.format, 'json')
  assert.deepEqual(asJson.result.table.headers, ['a'])

  // JSON read as CSV is not an error, it is one very odd table.
  const asCsv = parseInput('[{"a":1}]', 'csv', ',', true)
  assert.equal(asCsv.result.ok, true)
})

test('columnKey is unique per position', () => {
  assert.equal(columnKey(0), 'c0')
  assert.equal(columnKey(12), 'c12')
  const headers = ['Status', 'Status']
  assert.notEqual(columnKey(headers.indexOf('Status')), columnKey(1))
})

test('csvNameFor makes a safe download name', () => {
  const cases: Array<[string, string]> = [
    ['', 'converted-table'],
    ['users_export.csv', 'users_export'],
    ['Users Export (2026).json', 'users-export-2026'],
    ['....', 'converted-table'],
    ['no-extension', 'no-extension'],
  ]
  for (const [name, want] of cases) {
    assert.equal(csvNameFor(name), want, name)
  }
})
