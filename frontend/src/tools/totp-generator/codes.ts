// Pure helpers for the TOTP page. Nothing here touches a secret: it only shapes
// what the backend already decided to show.

import type { Account, Code } from './api'

// The same collator ResultsTable sorts with, so "Site 2" comes before "Site 10".
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

/**
 * Splits a code into groups that are easy to read out and easy to type in.
 * Six digits split in half, seven and eight split after four, which is how
 * every authenticator app on a phone does it.
 */
export function groupCode(code: string): string {
  if (code.length === 6) return `${code.slice(0, 3)} ${code.slice(3)}`
  if (code.length === 7 || code.length === 8) return `${code.slice(0, 4)} ${code.slice(4)}`
  return code
}

/** Sorted for the page: issuer first, then label, both case-insensitive. */
export function sortAccounts<T extends { issuer: string; label: string }>(items: T[]): T[] {
  return items
    .slice()
    .sort((a, b) => collator.compare(a.issuer, b.issuer) || collator.compare(a.label, b.label))
}

/** The accounts matching the filter box. */
export function filterAccounts<T extends { issuer: string; label: string }>(
  items: T[],
  query: string,
): T[] {
  const needle = query.trim().toLowerCase()
  if (needle === '') return items
  return items.filter((item) => `${item.issuer} ${item.label}`.toLowerCase().includes(needle))
}

/** 'warn' once a code is nearly gone, so the tech waits for the next one. */
export function secondsTone(expiresIn: number): 'warn' | '' {
  return expiresIn <= 5 ? 'warn' : ''
}

/** A name for the card when an account only filled in one of the two fields. */
export function accountTitle(account: Pick<Account, 'issuer' | 'label'>): string {
  if (account.issuer !== '') return account.issuer
  if (account.label !== '') return account.label
  return 'Unnamed account'
}

/** The line under the title, empty when it would repeat the title. */
export function accountSubtitle(account: Pick<Account, 'issuer' | 'label'>): string {
  if (account.issuer === '') return ''
  return account.label
}

/** chit-totp-vault-2026-07-26.json */
export function vaultFileName(nowIso: string): string {
  return `chit-totp-vault-${nowIso.slice(0, 10)}.json`
}

/** What the import toast says. */
export function importSummary(added: number, skipped: number): string {
  if (added === 0 && skipped === 0) return 'That vault file has no accounts in it.'
  if (skipped === 0) return `Imported: ${added} added.`
  if (added === 0) return `Imported: nothing new, ${skipped} already in this vault.`
  return `Imported: ${added} added, ${skipped} already in this vault.`
}

/** Merges the account list with the codes so a card always has both. */
export function withCodes(accounts: Account[], codes: Code[]): Array<Account & { code: Code | null }> {
  const byId = new Map(codes.map((code) => [code.id, code]))
  return accounts.map((account) => ({ ...account, code: byId.get(account.id) ?? null }))
}
