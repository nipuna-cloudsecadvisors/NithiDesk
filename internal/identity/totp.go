package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP per RFC 6238: SHA-1, 30-second steps, 6 digits. These are the
// defaults every authenticator app supports.
const (
	totpPeriod = 30
	totpDigits = 6
	// totpSkew accepts the previous and next step to absorb phone clock drift.
	totpSkew = 1
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns 20 random bytes (160 bits, RFC 4226 recommendation).
func NewTOTPSecret() ([]byte, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("totp secret: %w", err)
	}
	return secret, nil
}

// EncodeSecret is the base32 form shown to users who type the key manually.
func EncodeSecret(secret []byte) string { return base32NoPad.EncodeToString(secret) }

// OTPAuthURI is the provisioning URI an authenticator app reads from a QR
// code. The label carries the firm and email only.
func OTPAuthURI(secret []byte, issuer, account string) string {
	label := url.PathEscape(issuer + ":" + account)
	v := url.Values{}
	v.Set("secret", EncodeSecret(secret))
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprint(totpDigits))
	v.Set("period", fmt.Sprint(totpPeriod))
	return "otpauth://totp/" + label + "?" + v.Encode()
}

func hotp(secret []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code)
}

// TOTPCode returns the code for time t (used by tests and tooling).
func TOTPCode(secret []byte, t time.Time) string {
	return hotp(secret, uint64(t.Unix()/totpPeriod))
}

// VerifyTOTP checks code at time now and returns the matched time step.
// A step at or before lastUsed is rejected, so a code cannot be replayed.
func VerifyTOTP(secret []byte, code string, now time.Time, lastUsed int64) (int64, bool) {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != totpDigits {
		return 0, false
	}
	current := now.Unix() / totpPeriod
	for delta := int64(-totpSkew); delta <= totpSkew; delta++ {
		step := current + delta
		if step <= lastUsed {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(secret, uint64(step))), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// Recovery codes are 10 random groups of 4+4 base32 characters. Only their
// SHA-256 is stored; the codes are shown once.
const recoveryCodeCount = 10

// NewRecoveryCodes returns fresh codes and their hashes.
func NewRecoveryCodes() ([]string, [][]byte, error) {
	codes := make([]string, recoveryCodeCount)
	hashes := make([][]byte, recoveryCodeCount)
	for i := range codes {
		raw := make([]byte, 5)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, fmt.Errorf("recovery code: %w", err)
		}
		s := strings.ToLower(base32NoPad.EncodeToString(raw)) // 8 chars
		codes[i] = s[:4] + "-" + s[4:]
		hashes[i] = HashRecoveryCode(codes[i])
	}
	return codes, hashes, nil
}

// HashRecoveryCode normalises and hashes a recovery code.
func HashRecoveryCode(code string) []byte {
	c := strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	sum := sha256.Sum256([]byte("nithidesk-recovery:" + c))
	return sum[:]
}
