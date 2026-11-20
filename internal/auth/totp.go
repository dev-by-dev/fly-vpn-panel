package auth

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	totpIssuer = "flypanel"
	// Backup codes: 10 codes of 8 base32 chars (5 random bytes each).
	backupCodeCount = 10
	backupCodeBytes = 5
)

// EnrollTOTP generates a new TOTP secret and otpauth URI for account.
func EnrollTOTP(account string) (secret, otpauthURI string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: account,
	})
	if err != nil {
		return "", "", fmt.Errorf("totp generate: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTP reports whether code is valid for secret at the current
// time (30s period, 6 digits, SHA1, clock skew of 1 step allowed).
func ValidateTOTP(secret, code string) bool {
	return validateTOTPAt(secret, code, time.Now())
}

// validateTOTPAt validates a code for a fixed point in time; the RFC 6238
// test vector pins time through here.
func validateTOTPAt(secret, code string, at time.Time) bool {
	ok, err := totp.ValidateCustom(code, secret, at, totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateBackupCodes returns 10 single-use codes (8 base32 chars) and their
// SHA-256 hashes. The codes are only ever shown once; only hashes are stored.
func GenerateBackupCodes() (codes, hashes []string, err error) {
	b32 := base32.StdEncoding.WithPadding(base32.NoPadding)
	codes = make([]string, 0, backupCodeCount)
	hashes = make([]string, 0, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		b := make([]byte, backupCodeBytes)
		if _, err = rand.Read(b); err != nil {
			return nil, nil, fmt.Errorf("backup code rand: %w", err)
		}
		code := b32.EncodeToString(b)
		codes = append(codes, code)
		hashes = append(hashes, backupHash(code))
	}
	return codes, hashes, nil
}

// backupHash is the storage digest of a backup code.
func backupHash(code string) string { return hashToken(code) }
