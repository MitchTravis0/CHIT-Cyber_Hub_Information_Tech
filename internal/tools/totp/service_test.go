package totp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"chit/internal/core"
	"chit/internal/store"
)

const testPassphrase = "a passphrase for the vault"

// testClock is a clock the tests move by hand, so the idle timeout can be
// crossed without waiting fifteen minutes.
type testClock struct {
	at time.Time
}

func (c *testClock) now() time.Time { return c.at }

func newTestService(t *testing.T) (*Service, *testClock) {
	t.Helper()
	st, err := store.NewAt(t.TempDir(), false)
	if err != nil {
		t.Fatalf("store.NewAt: %v", err)
	}
	// 2,000 iterations rather than 600,000: the shipping figure is exercised by
	// TestVaultRoundTripAtShippingCost, and paying it in every test here would
	// cost the suite a minute for no extra coverage.
	clock := &testClock{at: time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)}
	return newService(st, 2000, clock.now), clock
}

func mustCreate(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.Create(testPassphrase); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func mustAdd(t *testing.T, s *Service, issuer, label, secret string) Account {
	t.Helper()
	account, err := s.Add(NewAccount{Secret: secret, Issuer: issuer, Label: label})
	if err != nil {
		t.Fatalf("Add(%s): %v", issuer, err)
	}
	return account
}

// TestNewUsesTheShippingIterationCount guards the one difference between the
// service the tests build and the one the app builds.
func TestNewUsesTheShippingIterationCount(t *testing.T) {
	st, err := store.NewAt(t.TempDir(), false)
	if err != nil {
		t.Fatalf("store.NewAt: %v", err)
	}
	if got := New(st).iterations; got != kdfIterations {
		t.Fatalf("New writes vaults at %d iterations, want %d", got, kdfIterations)
	}
}

func TestStatusWithNoVault(t *testing.T) {
	s, _ := newTestService(t)

	status, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.HasVault || status.Unlocked || status.Accounts != 0 {
		t.Fatalf("Status = %+v, want an empty machine", status)
	}
	if status.Note != msgNoVault {
		t.Errorf("Note = %q, want %q", status.Note, msgNoVault)
	}
}

func TestCreateUnlockLockCycle(t *testing.T) {
	s, _ := newTestService(t)
	mustCreate(t, s)

	status, err := s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.HasVault || !status.Unlocked {
		t.Fatalf("after Create, Status = %+v", status)
	}

	if _, err := s.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	status, err = s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.HasVault || status.Unlocked {
		t.Fatalf("after Lock, Status = %+v", status)
	}

	if _, err := s.Unlock(testPassphrase); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	status, err = s.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Unlocked {
		t.Fatal("the vault did not reopen")
	}
}

func TestCreateRules(t *testing.T) {
	t.Run("a passphrase of eleven characters", func(t *testing.T) {
		s, _ := newTestService(t)
		_, err := s.Create(strings.Repeat("a", 11))
		if err == nil || err.Error() != msgPassphraseShort {
			t.Fatalf("err = %v, want %q", err, msgPassphraseShort)
		}
	})

	t.Run("a passphrase of exactly twelve characters", func(t *testing.T) {
		s, _ := newTestService(t)
		if _, err := s.Create(strings.Repeat("a", 12)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	})

	t.Run("a passphrase past the maximum", func(t *testing.T) {
		s, _ := newTestService(t)
		_, err := s.Create(strings.Repeat("a", maxPassphrase+1))
		if err == nil || err.Error() != msgPassphraseLong {
			t.Fatalf("err = %v, want %q", err, msgPassphraseLong)
		}
	})

	t.Run("a second vault is refused", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		_, err := s.Create(testPassphrase)
		if err == nil || err.Error() != msgVaultExists {
			t.Fatalf("err = %v, want %q", err, msgVaultExists)
		}
	})
}

func TestUnlockRules(t *testing.T) {
	t.Run("with no vault", func(t *testing.T) {
		s, _ := newTestService(t)
		_, err := s.Unlock(testPassphrase)
		if err == nil || err.Error() != msgNoVault {
			t.Fatalf("err = %v, want %q", err, msgNoVault)
		}
	})

	t.Run("with the wrong passphrase", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		if _, err := s.Lock(); err != nil {
			t.Fatal(err)
		}
		_, err := s.Unlock(testPassphrase + "!")
		if err == nil || err.Error() != msgWrongPassphrase {
			t.Fatalf("err = %v, want %q", err, msgWrongPassphrase)
		}
	})

	t.Run("the vault survives a lock and a reopen", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		mustAdd(t, s, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")
		if _, err := s.Lock(); err != nil {
			t.Fatal(err)
		}
		status, err := s.Unlock(testPassphrase)
		if err != nil {
			t.Fatalf("Unlock: %v", err)
		}
		if status.Accounts != 1 {
			t.Fatalf("Accounts = %d, want 1", status.Accounts)
		}
	})
}

// TestEveryContentCallNeedsUnlocking is the property the whole tool rests on.
func TestEveryContentCallNeedsUnlocking(t *testing.T) {
	s, _ := newTestService(t)
	mustCreate(t, s)
	mustAdd(t, s, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")
	id := s.accounts[0].ID
	if _, err := s.Lock(); err != nil {
		t.Fatal(err)
	}

	calls := map[string]func() error{
		"List":   func() error { _, err := s.List(); return err },
		"Codes":  func() error { _, err := s.Codes(); return err },
		"Add":    func() error { _, err := s.Add(NewAccount{Secret: "MFRGGZDFMZTWQ2LK", Issuer: "X"}); return err },
		"Remove": func() error { _, err := s.Remove(id); return err },
		"Import": func() error { _, err := s.Import(`{"ciphertext":"AA=="}`, testPassphrase); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("the call worked while the vault was locked")
			}
			want := msgLocked
			if name == "Import" {
				want = msgImportLocked
			}
			if err.Error() != want {
				t.Errorf("message = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestAutoLock(t *testing.T) {
	// The literal is deliberate: measuring against idleTimeout itself would pass
	// for any timeout, including one long enough to be no protection at all.
	const window = 15 * time.Minute

	t.Run("the timeout really is fifteen minutes", func(t *testing.T) {
		if idleTimeout != window {
			t.Fatalf("idleTimeout is %v, and the help text and the message both say 15 minutes", idleTimeout)
		}
	})

	t.Run("just inside the timeout keeps working and pushes it out", func(t *testing.T) {
		s, clock := newTestService(t)
		mustCreate(t, s)

		clock.at = clock.at.Add(window - time.Second)
		if _, err := s.Codes(); err != nil {
			t.Fatalf("Codes at 14m59s: %v", err)
		}
		// The successful call reset the clock, so another 14m59s is also fine.
		clock.at = clock.at.Add(window - time.Second)
		if _, err := s.Codes(); err != nil {
			t.Fatalf("Codes after a second 14m59s: %v", err)
		}
	})

	t.Run("past the timeout locks and forgets the key", func(t *testing.T) {
		s, clock := newTestService(t)
		mustCreate(t, s)
		mustAdd(t, s, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")

		clock.at = clock.at.Add(window + time.Second)
		_, err := s.Codes()
		if err == nil || err.Error() != msgAutoLocked {
			t.Fatalf("err = %v, want %q", err, msgAutoLocked)
		}
		if s.key != nil || s.accounts != nil {
			t.Fatal("the key or the accounts survived the auto-lock")
		}

		status, err := s.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.Unlocked {
			t.Fatal("Status still reports the vault as unlocked")
		}
	})

	t.Run("Status alone locks an idle vault", func(t *testing.T) {
		s, clock := newTestService(t)
		mustCreate(t, s)
		clock.at = clock.at.Add(window + time.Second)

		status, err := s.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if status.Unlocked || s.key != nil {
			t.Fatal("Status left an idle vault open")
		}
	})
}

func TestAddRules(t *testing.T) {
	t.Run("a duplicate is refused", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		mustAdd(t, s, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")

		_, err := s.Add(NewAccount{Secret: "GEZDGNBVGY3TQOJQ", Issuer: "Firewall", Label: "admin"})
		if err == nil || err.Error() != "Firewall admin is already in this vault." {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the same name with a different secret is allowed", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		mustAdd(t, s, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")
		mustAdd(t, s, "Firewall", "admin", "MFRGGZDFMZTWQ2LK")
	})

	t.Run("a secret that is not base32 is refused", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		_, err := s.Add(NewAccount{Secret: "not a secret 111", Issuer: "Firewall"})
		if err == nil || err.Error() != msgSecretFormat {
			t.Fatalf("err = %v, want %q", err, msgSecretFormat)
		}
	})

	t.Run("the two hundredth is accepted and the next is not", func(t *testing.T) {
		// The literal is deliberate: looping to maxAccounts would pass whatever
		// the cap were, including one the refusal message no longer matches.
		const cap = 200
		if maxAccounts != cap {
			t.Fatalf("maxAccounts is %d, and msgTooManyAccounts says 200", maxAccounts)
		}
		s, _ := newTestService(t)
		mustCreate(t, s)
		for i := 0; i < cap; i++ {
			if _, err := s.Add(NewAccount{Secret: "GEZDGNBVGY3TQOJQ", Issuer: "Site", Label: string(rune('a'+i%26)) + strings.Repeat("x", i/26)}); err != nil {
				t.Fatalf("Add %d: %v", i, err)
			}
		}
		_, err := s.Add(NewAccount{Secret: "GEZDGNBVGY3TQOJQ", Issuer: "One", Label: "too many"})
		if err == nil || err.Error() != msgTooManyAccounts {
			t.Fatalf("err = %v, want %q", err, msgTooManyAccounts)
		}
	})

	t.Run("an added account survives a lock and an unlock", func(t *testing.T) {
		s, _ := newTestService(t)
		mustCreate(t, s)
		added := mustAdd(t, s, "Registrar", "billing@example.com", "MFRGGZDFMZTWQ2LK")
		if _, err := s.Lock(); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Unlock(testPassphrase); err != nil {
			t.Fatal(err)
		}
		list, err := s.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list.Accounts) != 1 || list.Accounts[0].ID != added.ID {
			t.Fatalf("List = %+v", list.Accounts)
		}
		if list.Accounts[0].Issuer != "Registrar" || list.Accounts[0].Label != "billing@example.com" {
			t.Errorf("the account came back changed: %+v", list.Accounts[0])
		}
	})
}

func TestRemove(t *testing.T) {
	s, _ := newTestService(t)
	mustCreate(t, s)
	first := mustAdd(t, s, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")
	second := mustAdd(t, s, "Registrar", "billing", "MFRGGZDFMZTWQ2LK")

	status, err := s.Remove(first.ID)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if status.Accounts != 1 {
		t.Fatalf("Accounts = %d, want 1", status.Accounts)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Accounts) != 1 || list.Accounts[0].ID != second.ID {
		t.Fatalf("the wrong account was removed: %+v", list.Accounts)
	}

	_, err = s.Remove(first.ID)
	if err == nil || err.Error() != msgNotFound {
		t.Fatalf("removing it twice: err = %v, want %q", err, msgNotFound)
	}
}

// TestCodesMatchTheRFCVector checks the whole path from a stored account to a
// code, against the same published vector the unit test uses.
func TestCodesMatchTheRFCVector(t *testing.T) {
	s, clock := newTestService(t)
	mustCreate(t, s)
	mustAdd(t, s, "RFC", "6238", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")

	clock.at = time.Unix(59, 0).UTC()
	set, err := s.Codes()
	if err != nil {
		t.Fatalf("Codes: %v", err)
	}
	if len(set.Codes) != 1 {
		t.Fatalf("got %d codes", len(set.Codes))
	}
	// The six-digit truncation of the RFC's eight-digit 94287082.
	if set.Codes[0].Code != "287082" {
		t.Errorf("Code = %q, want %q", set.Codes[0].Code, "287082")
	}
	if set.Codes[0].ExpiresIn != 1 {
		t.Errorf("ExpiresIn = %d, want 1", set.Codes[0].ExpiresIn)
	}
	if set.AtUnix != 59 {
		t.Errorf("AtUnix = %d, want 59", set.AtUnix)
	}
}

func TestExportAndImport(t *testing.T) {
	source, _ := newTestService(t)
	mustCreate(t, source)
	mustAdd(t, source, "Firewall", "admin", "GEZDGNBVGY3TQOJQ")
	mustAdd(t, source, "Registrar", "billing", "MFRGGZDFMZTWQ2LK")

	file, err := source.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	t.Run("the exported file is still encrypted", func(t *testing.T) {
		for _, leak := range []string{"GEZDGNBVGY3TQOJQ", "Firewall", testPassphrase} {
			if strings.Contains(file, leak) {
				t.Errorf("the exported file contains %q in the clear", leak)
			}
		}
	})

	t.Run("it imports into another machine's vault", func(t *testing.T) {
		target, _ := newTestService(t)
		if _, err := target.Create("a different passphrase here"); err != nil {
			t.Fatal(err)
		}
		mustAdd(t, target, "Local", "only", "MFRGGZDFMZ")

		report, err := target.Import(file, testPassphrase)
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if report.Added != 2 || report.Skipped != 0 {
			t.Fatalf("report = %+v, want 2 added", report)
		}
		list, err := target.List()
		if err != nil {
			t.Fatal(err)
		}
		if len(list.Accounts) != 3 {
			t.Fatalf("got %d accounts after the import, want 3", len(list.Accounts))
		}
	})

	t.Run("importing it twice adds nothing the second time", func(t *testing.T) {
		target, _ := newTestService(t)
		if _, err := target.Create("a different passphrase here"); err != nil {
			t.Fatal(err)
		}
		if _, err := target.Import(file, testPassphrase); err != nil {
			t.Fatal(err)
		}
		report, err := target.Import(file, testPassphrase)
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if report.Added != 0 || report.Skipped != 2 {
			t.Fatalf("report = %+v, want 0 added and 2 skipped", report)
		}
	})

	t.Run("with the wrong passphrase", func(t *testing.T) {
		target, _ := newTestService(t)
		if _, err := target.Create("a different passphrase here"); err != nil {
			t.Fatal(err)
		}
		_, err := target.Import(file, "a different passphrase here")
		if err == nil || err.Error() != msgImportWrongPassphrase {
			t.Fatalf("err = %v, want %q", err, msgImportWrongPassphrase)
		}
	})

	t.Run("a file that is not a vault", func(t *testing.T) {
		target, _ := newTestService(t)
		mustCreate(t, target)
		for _, body := range []string{"", "not json", `{"version":1}`, `{"devices":[]}`} {
			_, err := target.Import(body, testPassphrase)
			if err == nil || err.Error() != msgNotAVault {
				t.Errorf("Import(%q): err = %v, want %q", body, err, msgNotAVault)
			}
		}
	})

	t.Run("a file that is far too big", func(t *testing.T) {
		target, _ := newTestService(t)
		mustCreate(t, target)
		_, err := target.Import(strings.Repeat("x", maxImportBytes+1), testPassphrase)
		if err == nil || err.Error() != msgImportTooBig {
			t.Fatalf("err = %v, want %q", err, msgImportTooBig)
		}
	})

	t.Run("exporting works while locked", func(t *testing.T) {
		if _, err := source.Lock(); err != nil {
			t.Fatal(err)
		}
		again, err := source.Export()
		if err != nil {
			t.Fatalf("Export while locked: %v", err)
		}
		if again != file {
			t.Error("the exported bytes changed between calls")
		}
	})

	t.Run("exporting with no vault", func(t *testing.T) {
		empty, _ := newTestService(t)
		_, err := empty.Export()
		if err == nil || err.Error() != msgNoVault {
			t.Fatalf("err = %v, want %q", err, msgNoVault)
		}
	})
}

func TestStoredDocumentShape(t *testing.T) {
	s, _ := newTestService(t)
	mustCreate(t, s)

	raw, err := s.store.Get(Namespace)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("the stored document is not JSON: %v", err)
	}
	version, ok := doc["version"].(float64)
	if !ok || int(version) != docVersion {
		t.Fatalf("version = %v, want %d", doc["version"], docVersion)
	}
	for _, key := range []string{"kdf", "iterations", "salt", "nonce", "ciphertext"} {
		if _, present := doc[key]; !present {
			t.Errorf("the stored document has no %q", key)
		}
	}
	if doc["kdf"] != kdfName {
		t.Errorf("kdf = %v, want %q", doc["kdf"], kdfName)
	}
}

// TestNoSecretFieldsCrossTheBoundary walks every type a bound method returns and
// fails if any of them could carry a secret or a passphrase to the frontend.
func TestNoSecretFieldsCrossTheBoundary(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(Status{}),
		reflect.TypeOf(Account{}),
		reflect.TypeOf(AccountList{}),
		reflect.TypeOf(Code{}),
		reflect.TypeOf(CodeSet{}),
		reflect.TypeOf(ImportReport{}),
	}
	banned := []string{"secret", "passphrase", "key", "password"}

	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, path string) {
		switch rt.Kind() {
		case reflect.Slice, reflect.Array, reflect.Ptr:
			walk(rt.Elem(), path)
			return
		case reflect.Struct:
			for i := 0; i < rt.NumField(); i++ {
				field := rt.Field(i)
				name := strings.ToLower(field.Name)
				for _, bad := range banned {
					if strings.Contains(name, bad) {
						t.Errorf("%s.%s could carry a secret across the boundary", path, field.Name)
					}
				}
				walk(field.Type, path+"."+field.Name)
			}
		}
	}
	for _, rt := range types {
		walk(rt, rt.Name())
	}
}

// TestCodeShape checks a returned code really is the digits it claims, so a
// formatting mistake cannot ship a code the UI renders as something else.
func TestCodeShape(t *testing.T) {
	s, _ := newTestService(t)
	mustCreate(t, s)
	if _, err := s.Add(NewAccount{URI: "otpauth://totp/Eight:digits?secret=GEZDGNBVGY3TQOJQ&digits=8"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(NewAccount{URI: "otpauth://totp/Six:digits?secret=MFRGGZDFMZTWQ2LK"}); err != nil {
		t.Fatal(err)
	}

	set, err := s.Codes()
	if err != nil {
		t.Fatalf("Codes: %v", err)
	}
	for _, code := range set.Codes {
		if len(code.Code) != code.Digits {
			t.Errorf("%s %s: code %q is %d characters, Digits says %d", code.Issuer, code.Label, code.Code, len(code.Code), code.Digits)
		}
		if strings.Trim(code.Code, "0123456789") != "" {
			t.Errorf("%s: code %q is not all digits", code.Issuer, code.Code)
		}
		if code.ExpiresIn < 1 || code.ExpiresIn > code.Period {
			t.Errorf("%s: ExpiresIn %d is outside 1..%d", code.Issuer, code.ExpiresIn, code.Period)
		}
	}
}

// TestEveryErrorIsAUserError makes sure no raw stdlib text can reach a tech, and
// that no message ever repeats a secret or a passphrase back at them.
func TestEveryErrorIsAUserError(t *testing.T) {
	s, _ := newTestService(t)

	const secret = "GEZDGNBVGY3TQOJQ"
	errs := []error{}
	collect := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	_, err := s.Unlock(testPassphrase)
	collect(err)
	_, err = s.Create("short")
	collect(err)
	mustCreate(t, s)
	_, err = s.Create(testPassphrase)
	collect(err)
	_, err = s.Add(NewAccount{})
	collect(err)
	_, err = s.Add(NewAccount{Secret: secret})
	collect(err)
	_, err = s.Add(NewAccount{Secret: "111", Issuer: "X"})
	collect(err)
	_, err = s.Add(NewAccount{URI: "https://example.com"})
	collect(err)
	_, err = s.Remove("otp-nothing")
	collect(err)
	_, err = s.Import("not a vault", testPassphrase)
	collect(err)

	if len(errs) < 9 {
		t.Fatalf("only collected %d errors, so the cases above are not all failing", len(errs))
	}
	for _, err := range errs {
		var userErr *core.UserError
		if !isUserError(err, &userErr) {
			t.Errorf("a raw error can reach the user: %v", err)
			continue
		}
		if strings.Contains(userErr.Message, secret) {
			t.Errorf("a message echoes the secret: %q", userErr.Message)
		}
		if strings.Contains(userErr.Message, testPassphrase) {
			t.Errorf("a message echoes the passphrase: %q", userErr.Message)
		}
		if userErr.Message == "" {
			t.Error("an empty message reached the user")
		}
	}
}

func isUserError(err error, out **core.UserError) bool {
	userErr, ok := err.(*core.UserError)
	if ok {
		*out = userErr
	}
	return ok
}
