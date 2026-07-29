import test from 'node:test'
import assert from 'node:assert/strict'
import {
  accountSubtitle,
  accountTitle,
  filterAccounts,
  groupCode,
  importSummary,
  secondsTone,
  sortAccounts,
  vaultFileName,
  withCodes,
} from '../src/tools/totp-generator/codes.ts'

test('groupCode splits six digits in half', () => {
  assert.equal(groupCode('483921'), '483 921')
  assert.equal(groupCode('000000'), '000 000')
})

test('groupCode splits seven and eight digits after four', () => {
  assert.equal(groupCode('4839213'), '4839 213')
  assert.equal(groupCode('48392137'), '4839 2137')
})

test('groupCode leaves an unexpected length alone rather than mangling it', () => {
  assert.equal(groupCode(''), '')
  assert.equal(groupCode('12345'), '12345')
  assert.equal(groupCode('123456789'), '123456789')
})

test('sortAccounts orders by issuer then label, counting numbers properly', () => {
  const sorted = sortAccounts([
    { issuer: 'Site 10', label: 'admin' },
    { issuer: 'site 2', label: 'b' },
    { issuer: 'Site 2', label: 'a' },
    { issuer: 'Alpha', label: 'z' },
  ])
  assert.deepEqual(
    sorted.map((a) => `${a.issuer}/${a.label}`),
    ['Alpha/z', 'Site 2/a', 'site 2/b', 'Site 10/admin'],
  )
})

test('filterAccounts matches issuer and label, case-insensitively', () => {
  const items = [
    { issuer: 'Firewall', label: 'admin@head-office' },
    { issuer: 'Registrar', label: 'billing@example.com' },
  ]
  assert.equal(filterAccounts(items, 'FIRE').length, 1)
  assert.equal(filterAccounts(items, 'billing').length, 1)
  assert.equal(filterAccounts(items, 'head-office').length, 1)
  assert.equal(filterAccounts(items, '').length, 2)
  assert.equal(filterAccounts(items, '   ').length, 2)
  assert.equal(filterAccounts(items, 'nothing').length, 0)
})

test('secondsTone warns only in the last five seconds', () => {
  assert.equal(secondsTone(30), '')
  assert.equal(secondsTone(6), '')
  assert.equal(secondsTone(5), 'warn')
  assert.equal(secondsTone(1), 'warn')
  assert.equal(secondsTone(0), 'warn')
})

test('accountTitle falls back through issuer, label, then a placeholder', () => {
  assert.equal(accountTitle({ issuer: 'Firewall', label: 'admin' }), 'Firewall')
  assert.equal(accountTitle({ issuer: '', label: 'admin' }), 'admin')
  assert.equal(accountTitle({ issuer: '', label: '' }), 'Unnamed account')
})

test('accountSubtitle never repeats what the title already says', () => {
  assert.equal(accountSubtitle({ issuer: 'Firewall', label: 'admin' }), 'admin')
  assert.equal(accountSubtitle({ issuer: '', label: 'admin' }), '')
  assert.equal(accountSubtitle({ issuer: 'Firewall', label: '' }), '')
})

test('vaultFileName carries the date and nothing else', () => {
  assert.equal(vaultFileName('2026-07-26T10:41:00.000Z'), 'chit-totp-vault-2026-07-26.json')
})

test('importSummary reads correctly for every combination', () => {
  assert.equal(importSummary(0, 0), 'That vault file has no accounts in it.')
  assert.equal(importSummary(3, 0), 'Imported: 3 added.')
  assert.equal(importSummary(0, 2), 'Imported: nothing new, 2 already in this vault.')
  assert.equal(importSummary(3, 2), 'Imported: 3 added, 2 already in this vault.')
})

test('withCodes pairs each account with its own code and tolerates a missing one', () => {
  const accounts = [
    { id: 'a', issuer: 'A', label: '1', digits: 6, period: 30, algorithm: 'SHA1', addedAt: '' },
    { id: 'b', issuer: 'B', label: '2', digits: 6, period: 30, algorithm: 'SHA1', addedAt: '' },
  ]
  const codes = [
    { id: 'b', issuer: 'B', label: '2', code: '111222', digits: 6, period: 30, expiresIn: 12 },
  ]
  const merged = withCodes(accounts, codes)
  assert.equal(merged.length, 2)
  assert.equal(merged[0].code, null)
  assert.equal(merged[1].code?.code, '111222')
  assert.equal(merged[1].code?.expiresIn, 12)
})
