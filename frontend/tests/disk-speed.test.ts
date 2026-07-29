import test from 'node:test'
import assert from 'node:assert/strict'
import {
  phaseLabel,
  rateText,
  runLine,
  sampleId,
  secondsText,
} from '../src/tools/disk-speed/format.ts'

test('rateText keeps one decimal place and never renders a negative', () => {
  assert.equal(rateText(512.44), '512.4 MB/s')
  assert.equal(rateText(1840.06), '1840.1 MB/s')
  assert.equal(rateText(0), '0 MB/s')
  assert.equal(rateText(-5), '0 MB/s')
  assert.equal(rateText(Number.NaN), '0 MB/s')
  assert.equal(rateText(Number.POSITIVE_INFINITY), '0 MB/s')
})

test('phaseLabel names both phases and passes anything else through', () => {
  assert.equal(phaseLabel('write'), 'Write')
  assert.equal(phaseLabel('read'), 'Read')
  assert.equal(phaseLabel('verify'), 'verify')
})

test('secondsText keeps one decimal place', () => {
  assert.equal(secondsText(2.34), '2.3 s')
  assert.equal(secondsText(0), '0 s')
  assert.equal(secondsText(-1), '0 s')
})

test('runLine names the size and the whole run', () => {
  assert.equal(runLine(256, 4100), '256 MB written and read back in 4.1 s')
  assert.equal(runLine(1024, 41000), '1024 MB written and read back in 41.0 s')
})

test('sampleId separates the write and read rows at the same byte count', () => {
  const write = { phase: 'write', bytes: 16777216, seconds: 1, mbps: 100 }
  const read = { phase: 'read', bytes: 16777216, seconds: 1, mbps: 100 }
  assert.notEqual(sampleId(write), sampleId(read))
  assert.equal(sampleId(write), sampleId({ ...write, seconds: 9, mbps: 9 }))
})
