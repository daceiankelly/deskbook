package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// PBKDF2-HMAC-SHA256 from the Go standard library (Go 1.24+), at the OWASP
// 2023 recommended 600,000 iterations. Stored as:
//
//	pbkdf2-sha256$<iterations>$<salt b64>$<hash b64>
//
// The iteration count is stored per hash, so it can be raised later.
var Iterations = 600_000 // tests lower this

const MinPasswordLength = 10

var ErrWeakPassword = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

func HashPassword(pw string) (string, error) {
	if utf8.RuneCountInString(pw) < MinPasswordLength {
		return "", ErrWeakPassword
	}
	if len(pw) > 1024 {
		return "", errors.New("password too long")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, Iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", Iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || len(pw) > 1024 {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > 10_000_000 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is verified against when a username does not exist so that
// response timing does not reveal which usernames are valid.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("dummy-password-not-used")
	return h
})

func VerifyDummy(pw string) { VerifyPassword(pw, dummyHash()) }
