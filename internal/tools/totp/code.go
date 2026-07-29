package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"strconv"
	"strings"
	"time"

	"chit/internal/core"
)

const (
	algoSHA1   = "SHA1"
	algoSHA256 = "SHA256"
	algoSHA512 = "SHA512"

	defaultDigits = 6
	minDigits     = 6
	maxDigits     = 8

	defaultPeriod = 30
	minPeriod     = 15
	maxPeriod     = 300

	// A base32 secret shorter than this is not a real one: the shortest any
	// service issues is 16 characters, and 10 already allows for an odd one.
	minSecretChars = 10
	maxSecretChars = 1024
)

func hasherFor(algorithm string) (func() hash.Hash, bool) {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "", algoSHA1:
		return sha1.New, true
	case algoSHA256:
		return sha256.New, true
	case algoSHA512:
		return sha512.New, true
	}
	return nil, false
}

// normalizeAlgorithm returns the canonical name, or an error naming what was
// asked for.
func normalizeAlgorithm(algorithm string) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(algorithm))
	if name == "" {
		return algoSHA1, nil
	}
	if _, ok := hasherFor(name); !ok {
		return "", core.Errorf(core.CodeInvalidInput, msgAlgorithm, algorithm)
	}
	return name, nil
}

// Compute is RFC 6238: HMAC over the number of periods since the epoch, then
// the RFC 4226 dynamic truncation of that MAC.
func Compute(secret []byte, at time.Time, period, digits int, algorithm string) (string, error) {
	newHash, ok := hasherFor(algorithm)
	if !ok {
		return "", core.Errorf(core.CodeInvalidInput, msgAlgorithm, algorithm)
	}
	if period < minPeriod || period > maxPeriod {
		return "", core.Errorf(core.CodeInvalidInput, msgPeriod, strconv.Itoa(period))
	}
	if digits < minDigits || digits > maxDigits {
		return "", core.Errorf(core.CodeInvalidInput, msgDigits, strconv.Itoa(digits))
	}

	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/int64(period)))

	mac := hmac.New(newHash, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%mod), nil
}

// secondsLeft is how long the code showing at "at" stays valid.
func secondsLeft(at time.Time, period int) int {
	return period - int(at.Unix()%int64(period))
}

// parseSecret accepts a base32 secret the way people paste it: lower case, in
// groups separated by spaces or dashes, with or without the padding.
func parseSecret(text string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '\t', '\n', '\r':
			return -1
		}
		return r
	}, text)
	cleaned = strings.ToUpper(strings.TrimRight(cleaned, "="))

	if len(cleaned) < minSecretChars {
		return nil, core.Errorf(core.CodeInvalidInput, msgSecretShort)
	}
	if len(cleaned) > maxSecretChars {
		return nil, core.Errorf(core.CodeInvalidInput, msgSecretLong)
	}

	padding := strings.Repeat("=", (8-len(cleaned)%8)%8)
	raw, err := base32.StdEncoding.DecodeString(cleaned + padding)
	if err != nil {
		return nil, core.Errorf(core.CodeInvalidInput, msgSecretFormat)
	}
	return raw, nil
}
