package main

import "chit/internal/tools/totp"

// TOTPStatus says whether a vault exists on this machine and whether it is
// currently unlocked. The page calls it on mount to decide what to show.
func (a *App) TOTPStatus() (totp.Status, error) {
	return totp.Shared(a.store).Status()
}

// CreateTOTPVault writes a new, empty, encrypted vault and leaves it unlocked.
func (a *App) CreateTOTPVault(passphrase string) (totp.Status, error) {
	return totp.Shared(a.store).Create(passphrase)
}

// UnlockTOTPVault decrypts the vault and keeps the key in memory until it is
// locked or 15 minutes pass with no use.
func (a *App) UnlockTOTPVault(passphrase string) (totp.Status, error) {
	return totp.Shared(a.store).Unlock(passphrase)
}

// LockTOTPVault forgets the key and every secret held in memory.
func (a *App) LockTOTPVault() (totp.Status, error) {
	return totp.Shared(a.store).Lock()
}

// ListTOTPAccounts returns the accounts without their secrets.
func (a *App) ListTOTPAccounts() (totp.AccountList, error) {
	return totp.Shared(a.store).List()
}

// TOTPCodes returns the code showing right now for every account.
func (a *App) TOTPCodes() (totp.CodeSet, error) {
	return totp.Shared(a.store).Codes()
}

// AddTOTPAccount takes an otpauth:// link, or a base32 secret plus an issuer
// and a label, and saves it to the vault.
func (a *App) AddTOTPAccount(in totp.NewAccount) (totp.Account, error) {
	return totp.Shared(a.store).Add(in)
}

// RemoveTOTPAccount deletes one account and re-encrypts the vault.
func (a *App) RemoveTOTPAccount(id string) (totp.Status, error) {
	return totp.Shared(a.store).Remove(id)
}

// ExportTOTPVault returns the vault file exactly as it is stored, still
// encrypted, for handing to a colleague.
func (a *App) ExportTOTPVault() (string, error) {
	return totp.Shared(a.store).Export()
}

// ImportTOTPVault decrypts another vault file with its own passphrase and adds
// the accounts it holds to the vault that is currently open.
func (a *App) ImportTOTPVault(fileJSON string, passphrase string) (totp.ImportReport, error) {
	return totp.Shared(a.store).Import(fileJSON, passphrase)
}
