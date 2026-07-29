// Package totp holds the two-factor seeds for accounts a team shares, in a
// vault encrypted with a passphrase, and works out the code showing right now.
// It is the service layer the TOTP Code Generator page talks to. A secret only
// ever exists in plaintext inside this process, between decryption and use.
package totp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"chit/internal/core"
	"chit/internal/store"
)

const (
	// idleTimeout wipes the key when nobody has asked for a code in a while, so
	// a laptop left open on a desk stops showing them.
	idleTimeout = 15 * time.Minute

	minPassphrase = 12
	maxPassphrase = 1024

	maxAccounts = 200

	// maxImportBytes is well above any real vault and well below anything that
	// would be slow to parse.
	maxImportBytes = 1 << 20
)

// Status is everything the page needs to pick which panel to show. It never
// carries a secret, a code or a passphrase.
type Status struct {
	HasVault bool   `json:"hasVault"`
	Unlocked bool   `json:"unlocked"`
	Accounts int    `json:"accounts"`
	Note     string `json:"note"`
}

// Account describes one entry without its secret.
type Account struct {
	ID        string `json:"id"`
	Issuer    string `json:"issuer"`
	Label     string `json:"label"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Algorithm string `json:"algorithm"`
	AddedAt   string `json:"addedAt"`
}

// AccountList wraps the slice so an empty vault marshals as [] and not null.
type AccountList struct {
	Accounts []Account `json:"accounts"`
}

// NewAccount is what the add form sends. Either URI is filled in, or Secret
// plus at least one of Issuer and Label.
type NewAccount struct {
	URI    string `json:"uri"`
	Secret string `json:"secret"`
	Issuer string `json:"issuer"`
	Label  string `json:"label"`
}

// Code is one account's code right now.
type Code struct {
	ID        string `json:"id"`
	Issuer    string `json:"issuer"`
	Label     string `json:"label"`
	Code      string `json:"code"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	ExpiresIn int    `json:"expiresIn"`
}

// CodeSet is every account's code, read at one instant so they agree with each
// other.
type CodeSet struct {
	Codes  []Code `json:"codes"`
	AtUnix int64  `json:"atUnix"`
	Note   string `json:"note"`
}

// ImportReport says what an imported vault added.
type ImportReport struct {
	Added   int    `json:"added"`
	Skipped int    `json:"skipped"`
	Note    string `json:"note"`
}

// Service owns the vault. It holds the derived key and the decrypted accounts
// while the vault is unlocked, which is why there is exactly one of it.
type Service struct {
	store *store.Store
	now   func() time.Time

	// iterations is what a vault created here is written with. A vault opened
	// from a file keeps its own, so an older or a colleague's file still opens.
	iterations int

	mu              sync.Mutex
	key             []byte
	salt            []byte
	vaultIterations int
	accounts        []secretAccount
	lastUsed        time.Time
}

var (
	shared     *Service
	sharedOnce sync.Once
)

// Shared is the one service the app uses. The vault has to survive between
// bound calls, because the key stays in memory while it is unlocked, and
// app.go is frozen, so the instance lives here rather than on App.
func Shared(st *store.Store) *Service {
	sharedOnce.Do(func() { shared = New(st) })
	return shared
}

func New(st *store.Store) *Service {
	return newService(st, kdfIterations, time.Now)
}

// newService exists so the tests can lower the iteration count and inject a
// clock. At the shipping 600,000 iterations every unlock costs about a third of
// a second, which is right for a person typing a passphrase and wrong for a
// suite that unlocks a hundred times.
func newService(st *store.Store, iterations int, now func() time.Time) *Service {
	return &Service{store: st, now: now, iterations: iterations}
}

func newID(prefix string) (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", core.Errorf(core.CodeInternal, msgNoRandom)
	}
	return prefix + hex.EncodeToString(buf), nil
}

// readDoc returns the stored vault. ok is false when there is no vault yet,
// which is the normal state on a fresh machine and never an error.
func (s *Service) readDoc() (doc vaultDoc, ok bool, err error) {
	raw, err := s.store.Get(Namespace)
	if err != nil {
		return doc, false, core.Errorf(core.CodeInternal, msgReadFailed)
	}
	if strings.TrimSpace(raw) == "" {
		return doc, false, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, true, core.Errorf(core.CodeInvalidInput, msgTampered)
	}
	return doc, true, nil
}

func (s *Service) writeDoc(doc vaultDoc) error {
	body, err := json.Marshal(doc)
	if err != nil {
		return core.Errorf(core.CodeInternal, msgSaveFailed)
	}
	if err := s.store.Set(Namespace, string(body)); err != nil {
		return core.Errorf(core.CodeInternal, msgSaveFailed)
	}
	return nil
}

// wipeLocked forgets the key and every secret. Zeroing the key first shortens
// the window, though Go's collector may have copied it already: see the spec.
func (s *Service) wipeLocked() {
	for i := range s.key {
		s.key[i] = 0
	}
	s.key = nil
	s.salt = nil
	s.vaultIterations = 0
	s.accounts = nil
}

// requireUnlockedLocked is the gate every operation on the contents goes
// through. It also applies the idle timeout, so the rule cannot be forgotten in
// one method.
func (s *Service) requireUnlockedLocked() error {
	if s.key == nil {
		return core.Errorf(core.CodeInvalidInput, msgLocked)
	}
	if s.now().Sub(s.lastUsed) > idleTimeout {
		s.wipeLocked()
		return core.Errorf(core.CodeInvalidInput, msgAutoLocked)
	}
	s.lastUsed = s.now()
	return nil
}

func (s *Service) statusLocked(hasVault bool, note string) Status {
	return Status{
		HasVault: hasVault,
		Unlocked: s.key != nil,
		Accounts: len(s.accounts),
		Note:     note,
	}
}

// Status reports whether a vault exists and whether it is open. It is the only
// method that is happy to be called at any time.
func (s *Service) Status() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// An idle unlocked vault reports itself locked, so the page does not offer
	// codes it will refuse a moment later.
	if s.key != nil && s.now().Sub(s.lastUsed) > idleTimeout {
		s.wipeLocked()
	}

	doc, ok, err := s.readDoc()
	if err != nil {
		return Status{}, err
	}
	if !ok {
		return s.statusLocked(false, msgNoVault), nil
	}
	if doc.Version > docVersion {
		return s.statusLocked(true, msgNewerVault), nil
	}
	return s.statusLocked(true, ""), nil
}

func validatePassphrase(passphrase string) error {
	if len(passphrase) < minPassphrase {
		return core.Errorf(core.CodeInvalidInput, msgPassphraseShort)
	}
	if len(passphrase) > maxPassphrase {
		return core.Errorf(core.CodeInvalidInput, msgPassphraseLong)
	}
	return nil
}

// Create writes a new, empty vault and leaves it unlocked. It refuses when one
// already exists, so nothing can be overwritten by accident.
func (s *Service) Create(passphrase string) (Status, error) {
	if err := validatePassphrase(passphrase); err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok, err := s.readDoc()
	if err != nil {
		return Status{}, err
	}
	if ok {
		return Status{}, core.Errorf(core.CodeInvalidInput, msgVaultExists)
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return Status{}, core.Errorf(core.CodeInternal, msgNoRandom)
	}
	key := pbkdf2SHA256([]byte(passphrase), salt, s.iterations, keyLen)

	doc, err := sealVault(key, salt, s.iterations, plaintext{Accounts: []secretAccount{}})
	if err != nil {
		return Status{}, err
	}
	if err := s.writeDoc(doc); err != nil {
		return Status{}, err
	}

	s.key = key
	s.salt = salt
	s.vaultIterations = s.iterations
	s.accounts = []secretAccount{}
	s.lastUsed = s.now()
	return s.statusLocked(true, ""), nil
}

// Unlock decrypts the vault and keeps the key until Lock or the idle timeout.
func (s *Service) Unlock(passphrase string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok, err := s.readDoc()
	if err != nil {
		return Status{}, err
	}
	if !ok {
		return Status{}, core.Errorf(core.CodeNotFound, msgNoVault)
	}

	key, salt, pt, err := openVault(doc, passphrase)
	if err != nil {
		return Status{}, err
	}

	s.key = key
	s.salt = salt
	s.vaultIterations = doc.Iterations
	s.accounts = pt.Accounts
	s.lastUsed = s.now()
	return s.statusLocked(true, ""), nil
}

// Lock forgets the key and every secret held in memory.
func (s *Service) Lock() (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.wipeLocked()
	_, ok, err := s.readDoc()
	if err != nil {
		return Status{}, err
	}
	return s.statusLocked(ok, ""), nil
}

// List returns the accounts without their secrets.
func (s *Service) List() (AccountList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlockedLocked(); err != nil {
		return AccountList{Accounts: []Account{}}, err
	}
	return AccountList{Accounts: s.publicAccountsLocked()}, nil
}

func (s *Service) publicAccountsLocked() []Account {
	out := make([]Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		out = append(out, Account{
			ID:        a.ID,
			Issuer:    a.Issuer,
			Label:     a.Label,
			Digits:    a.Digits,
			Period:    a.Period,
			Algorithm: a.Algorithm,
			AddedAt:   a.AddedAt,
		})
	}
	return out
}

// Codes returns the code showing right now for every account. All of them are
// read at one instant, so two accounts on the same period always agree.
func (s *Service) Codes() (CodeSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlockedLocked(); err != nil {
		return CodeSet{Codes: []Code{}}, err
	}

	at := s.now()
	out := CodeSet{Codes: make([]Code, 0, len(s.accounts)), AtUnix: at.Unix()}
	for _, a := range s.accounts {
		secret, err := parseSecret(a.Secret)
		if err != nil {
			// A stored secret that no longer parses would have been rejected on
			// the way in, so this is a file somebody edited by hand.
			return CodeSet{Codes: []Code{}}, core.Errorf(core.CodeInvalidInput, msgTampered)
		}
		code, err := Compute(secret, at, a.Period, a.Digits, a.Algorithm)
		if err != nil {
			return CodeSet{Codes: []Code{}}, err
		}
		out.Codes = append(out.Codes, Code{
			ID:        a.ID,
			Issuer:    a.Issuer,
			Label:     a.Label,
			Code:      code,
			Digits:    a.Digits,
			Period:    a.Period,
			ExpiresIn: secondsLeft(at, a.Period),
		})
	}
	return out, nil
}

// draftFrom turns what the add form sent into an account, whichever of the two
// ways it was filled in.
func draftFrom(in NewAccount) (parsedAccount, error) {
	uri := strings.TrimSpace(in.URI)
	secret := strings.TrimSpace(in.Secret)

	if uri != "" {
		return parseURI(uri)
	}
	if secret == "" {
		return parsedAccount{}, core.Errorf(core.CodeInvalidInput, msgNothingToAdd)
	}
	issuer := strings.TrimSpace(in.Issuer)
	label := strings.TrimSpace(in.Label)
	if issuer == "" && label == "" {
		return parsedAccount{}, core.Errorf(core.CodeInvalidInput, msgNoName)
	}
	return parsedAccount{
		Issuer:    issuer,
		Label:     label,
		Secret:    secret,
		Digits:    defaultDigits,
		Period:    defaultPeriod,
		Algorithm: algoSHA1,
	}, nil
}

// Add saves one account and re-encrypts the vault.
func (s *Service) Add(in NewAccount) (Account, error) {
	draft, err := draftFrom(in)
	if err != nil {
		return Account{}, err
	}
	if draft.Issuer == "" && draft.Label == "" {
		return Account{}, core.Errorf(core.CodeInvalidInput, msgNoName)
	}
	// Parsed here as well as when a code is asked for, so a secret that is not
	// base32 is refused at the point the tech can still fix it.
	if _, err := parseSecret(draft.Secret); err != nil {
		return Account{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlockedLocked(); err != nil {
		return Account{}, err
	}
	if len(s.accounts) >= maxAccounts {
		return Account{}, core.Errorf(core.CodeInvalidInput, msgTooManyAccounts)
	}
	for _, a := range s.accounts {
		if sameAccount(a, draft) {
			return Account{}, core.Errorf(core.CodeInvalidInput, msgDuplicate,
				displayName(draft.Issuer), displayName(draft.Label))
		}
	}

	id, err := newID("otp-")
	if err != nil {
		return Account{}, err
	}
	entry := secretAccount{
		ID:        id,
		Issuer:    draft.Issuer,
		Label:     draft.Label,
		Secret:    draft.Secret,
		Digits:    draft.Digits,
		Period:    draft.Period,
		Algorithm: draft.Algorithm,
		AddedAt:   s.now().UTC().Format(time.RFC3339),
	}

	next := append(append([]secretAccount{}, s.accounts...), entry)
	if err := s.saveLocked(next); err != nil {
		return Account{}, err
	}
	return Account{
		ID:        entry.ID,
		Issuer:    entry.Issuer,
		Label:     entry.Label,
		Digits:    entry.Digits,
		Period:    entry.Period,
		Algorithm: entry.Algorithm,
		AddedAt:   entry.AddedAt,
	}, nil
}

// displayName keeps a duplicate message readable when one half is missing.
func displayName(text string) string {
	if text == "" {
		return "That account"
	}
	return text
}

func sameAccount(a secretAccount, draft parsedAccount) bool {
	return strings.EqualFold(a.Issuer, draft.Issuer) &&
		strings.EqualFold(a.Label, draft.Label) &&
		a.Secret == draft.Secret
}

// saveLocked re-encrypts and writes, and only touches the in-memory list once
// the write succeeded, so a failed save leaves the service exactly as it was.
func (s *Service) saveLocked(accounts []secretAccount) error {
	doc, err := sealVault(s.key, s.salt, s.vaultIterations, plaintext{Accounts: accounts})
	if err != nil {
		return err
	}
	if err := s.writeDoc(doc); err != nil {
		return err
	}
	s.accounts = accounts
	return nil
}

// Remove deletes one account and re-encrypts the vault.
func (s *Service) Remove(id string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlockedLocked(); err != nil {
		return Status{}, err
	}

	next := make([]secretAccount, 0, len(s.accounts))
	found := false
	for _, a := range s.accounts {
		if a.ID == id {
			found = true
			continue
		}
		next = append(next, a)
	}
	if !found {
		return Status{}, core.Errorf(core.CodeNotFound, msgNotFound)
	}
	if err := s.saveLocked(next); err != nil {
		return Status{}, err
	}
	return s.statusLocked(true, ""), nil
}

// Export returns the vault file exactly as stored, still encrypted. It works
// while locked on purpose: the file is no more readable than the copy already
// sitting in the data folder, and a tech handing it to a colleague should not
// have to unlock it first.
func (s *Service) Export() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := s.store.Get(Namespace)
	if err != nil {
		return "", core.Errorf(core.CodeInternal, msgReadFailed)
	}
	if strings.TrimSpace(raw) == "" {
		return "", core.Errorf(core.CodeNotFound, msgNoVault)
	}
	return raw, nil
}

// Import decrypts another vault file with its own passphrase and adds the
// accounts it holds to the vault that is currently open.
func (s *Service) Import(fileJSON, passphrase string) (ImportReport, error) {
	if len(fileJSON) > maxImportBytes {
		return ImportReport{}, core.Errorf(core.CodeInvalidInput, msgImportTooBig)
	}

	var doc vaultDoc
	if err := json.Unmarshal([]byte(fileJSON), &doc); err != nil || doc.Ciphertext == "" {
		return ImportReport{}, core.Errorf(core.CodeInvalidInput, msgNotAVault)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.key == nil {
		return ImportReport{}, core.Errorf(core.CodeInvalidInput, msgImportLocked)
	}
	if err := s.requireUnlockedLocked(); err != nil {
		return ImportReport{}, err
	}

	_, _, pt, err := openVault(doc, passphrase)
	if err != nil {
		// The file opened with a different passphrase from this machine's, so
		// say which one is being asked for.
		if core.CodeOf(err) == core.CodeInvalidInput && core.MessageOf(err) == msgWrongPassphrase {
			return ImportReport{}, core.Errorf(core.CodeInvalidInput, msgImportWrongPassphrase)
		}
		return ImportReport{}, err
	}

	next := append([]secretAccount{}, s.accounts...)
	report := ImportReport{}
	for _, incoming := range pt.Accounts {
		draft := parsedAccount{
			Issuer: incoming.Issuer, Label: incoming.Label, Secret: incoming.Secret,
			Digits: incoming.Digits, Period: incoming.Period, Algorithm: incoming.Algorithm,
		}
		duplicate := false
		for _, existing := range next {
			if sameAccount(existing, draft) {
				duplicate = true
				break
			}
		}
		if duplicate {
			report.Skipped++
			continue
		}
		if len(next) >= maxAccounts {
			report.Skipped++
			continue
		}
		id, err := newID("otp-")
		if err != nil {
			return ImportReport{}, err
		}
		incoming.ID = id
		next = append(next, incoming)
		report.Added++
	}

	if err := s.saveLocked(next); err != nil {
		return ImportReport{}, err
	}
	if report.Added == 0 && report.Skipped == 0 {
		report.Note = "That vault file has no accounts in it."
	} else if len(next) >= maxAccounts && report.Skipped > 0 {
		report.Note = msgTooManyAccounts
	}
	return report, nil
}
