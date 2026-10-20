package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters (OWASP password storage guidance, 2024+): 19 MiB,
// 2 passes, 1 lane. Encoded with the parameters so they can be raised later
// without breaking existing hashes.
const (
	argonTime    uint32 = 2
	argonMemory  uint32 = 19 * 1024
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

// MinPasswordLength follows NIST SP 800-63B: length over composition rules,
// no forced rotation (doc 10 §3).
const MinPasswordLength = 12

// HashPassword returns an encoded argon2id hash.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

var errBadHash = errors.New("malformed password hash")

// VerifyPassword reports whether password matches the encoded hash, in
// constant time.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash is verified against when an account does not exist, so a login
// for an unknown email costs the same time as a wrong password.
var dummyHash, _ = HashPassword("nithidesk-timing-equaliser")

// commonPasswords is a small offline deny-list. A full breached-password
// check (k-anonymity range queries) sends a hash prefix to a third party, so
// it is a configuration decision for the firm, not a default.
var commonPasswords = map[string]bool{
	"password1234": true, "123456789012": true, "qwertyuiop12": true, "passwordpassword": true,
	"iloveyou1234": true, "letmein12345": true, "welcome12345": true, "srilanka1234": true,
	"colombo12345": true, "administrator": true, "changeme1234": true, "nithidesk123": true,
}

// CheckPasswordPolicy returns a field error code, or "" when acceptable.
func CheckPasswordPolicy(password, email string) string {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return "password.too_short"
	}
	if utf8.RuneCountInString(password) > 256 {
		return "password.too_long"
	}
	lower := strings.ToLower(password)
	if commonPasswords[lower] {
		return "password.too_common"
	}
	if local, _, ok := strings.Cut(strings.ToLower(email), "@"); ok && len(local) >= 4 && strings.Contains(lower, local) {
		return "password.contains_email"
	}
	return ""
}
