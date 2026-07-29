package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strconv"

	"chit/internal/core"
)

// Namespace is the store document holding the encrypted vault.
const Namespace = "totp-generator"

const (
	docVersion = 1

	kdfName = "pbkdf2-hmac-sha256"

	// kdfIterations is the OWASP figure for PBKDF2-HMAC-SHA256. It costs about a
	// third of a second on a laptop, which is the right price for something a
	// person types once per session.
	kdfIterations = 600000

	// A file claiming far fewer iterations has been edited; one claiming far more
	// would hang the app for minutes before failing.
	minIterations = 1000
	maxIterations = 10000000

	saltLen  = 16
	nonceLen = 12
	keyLen   = 32
)

// vaultDoc is the file on disk, and the whole of what a colleague receives when
// a vault is exported. Nothing outside Ciphertext may be sensitive.
type vaultDoc struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// secretAccount is one entry as it exists inside the encrypted plaintext. It
// never crosses the bound-method boundary; Account does, and has no Secret.
type secretAccount struct {
	ID        string `json:"id"`
	Issuer    string `json:"issuer"`
	Label     string `json:"label"`
	Secret    string `json:"secret"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Algorithm string `json:"algorithm"`
	AddedAt   string `json:"addedAt"`
}

// plaintext is what gets encrypted.
type plaintext struct {
	Accounts []secretAccount `json:"accounts"`
}

// pbkdf2SHA256 is PBKDF2 with HMAC-SHA256 (RFC 8018). It is written out here
// because Go's only implementation lives in golang.org/x/crypto, which is an
// indirect dependency of Wails: importing it would make it direct and rewrite
// go.mod, which this project does not allow.
func pbkdf2SHA256(password, salt []byte, iterations, length int) []byte {
	mac := hmac.New(sha256.New, password)
	hashLen := mac.Size()
	blocks := (length + hashLen - 1) / hashLen

	out := make([]byte, 0, blocks*hashLen)
	block := make([]byte, len(salt)+4)
	copy(block, salt)
	u := make([]byte, 0, hashLen)
	t := make([]byte, hashLen)

	for i := 1; i <= blocks; i++ {
		binary.BigEndian.PutUint32(block[len(salt):], uint32(i))
		mac.Reset()
		mac.Write(block)
		u = mac.Sum(u[:0])
		copy(t, u)
		for n := 2; n <= iterations; n++ {
			mac.Reset()
			mac.Write(u)
			u = mac.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:length]
}

// aadOf binds the header to the ciphertext, so lowering the iteration count or
// swapping the salt in the file fails authentication instead of being accepted.
func aadOf(doc vaultDoc) []byte {
	return []byte(doc.KDF + "|" + strconv.Itoa(doc.Iterations) + "|" + doc.Salt)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, core.Errorf(core.CodeInternal, msgSaveFailed)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, core.Errorf(core.CodeInternal, msgSaveFailed)
	}
	return gcm, nil
}

// sealVault encrypts pt under key. The nonce is fresh on every call, so the
// same vault written twice never produces the same bytes.
func sealVault(key, salt []byte, iterations int, pt plaintext) (vaultDoc, error) {
	body, err := json.Marshal(pt)
	if err != nil {
		return vaultDoc{}, core.Errorf(core.CodeInternal, msgSaveFailed)
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return vaultDoc{}, core.Errorf(core.CodeInternal, msgNoRandom)
	}
	doc := vaultDoc{
		Version:    docVersion,
		KDF:        kdfName,
		Iterations: iterations,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
	}
	gcm, err := newGCM(key)
	if err != nil {
		return vaultDoc{}, err
	}
	doc.Ciphertext = base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, body, aadOf(doc)))
	return doc, nil
}

// openVault derives the key from passphrase and decrypts doc. The key comes
// back so the caller can keep it and re-encrypt without asking again.
func openVault(doc vaultDoc, passphrase string) (key, salt []byte, pt plaintext, err error) {
	if doc.Version > docVersion {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgNewerVault)
	}
	if doc.KDF != kdfName {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgTampered)
	}
	if doc.Iterations < minIterations || doc.Iterations > maxIterations {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgTampered)
	}
	salt, err = base64.StdEncoding.DecodeString(doc.Salt)
	if err != nil || len(salt) != saltLen {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgTampered)
	}
	nonce, err := base64.StdEncoding.DecodeString(doc.Nonce)
	if err != nil || len(nonce) != nonceLen {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgTampered)
	}
	body, err := base64.StdEncoding.DecodeString(doc.Ciphertext)
	if err != nil {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgTampered)
	}

	key = pbkdf2SHA256([]byte(passphrase), salt, doc.Iterations, keyLen)
	gcm, err := newGCM(key)
	if err != nil {
		return nil, nil, pt, err
	}
	clear, err := gcm.Open(nil, nonce, body, aadOf(doc))
	if err != nil {
		// AES-GCM cannot tell a wrong key from an edited file: both are just a
		// failed tag check. The message names both, most likely cause first.
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgWrongPassphrase)
	}
	if err := json.Unmarshal(clear, &pt); err != nil {
		return nil, nil, pt, core.Errorf(core.CodeInvalidInput, msgTampered)
	}
	if pt.Accounts == nil {
		pt.Accounts = []secretAccount{}
	}
	return key, salt, pt, nil
}
