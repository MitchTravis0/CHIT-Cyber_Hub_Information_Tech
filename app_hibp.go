package main

import "chit/internal/tools/hibp"

// CheckPasswordBreach asks haveibeenpwned whether a password appears in a public
// breach. Only the first five characters of the password's SHA-1 fingerprint are
// sent; the password itself never leaves this process and is never written down.
func (a *App) CheckPasswordBreach(password string) (hibp.Result, error) {
	return hibp.DefaultClient().Check(a.ctx, password)
}
