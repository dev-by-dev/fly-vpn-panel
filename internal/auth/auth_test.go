package auth

import (
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestArgon2Roundtrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword(): %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=1,p=4$") {
		t.Errorf("hash = %q, want argon2id PHC header $argon2id$v=19$m=65536,t=1,p=4$", hash)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Error("VerifyPassword(correct) = false, want true")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Error("VerifyPassword(wrong) = true, want false")
	}
}

func TestJWTExpired(t *testing.T) {
	j := NewJWT(newTestKey(t))
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "admin",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
	}).SignedString(j.key)
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}
	if err := j.VerifyAccess(tok); err == nil {
		t.Error("VerifyAccess(expired) = nil, want error")
	}
}

func TestJWTTampered(t *testing.T) {
	j := NewJWT(newTestKey(t))
	tok, err := j.IssueAccess()
	if err != nil {
		t.Fatalf("IssueAccess(): %v", err)
	}
	// Flip the last byte of the signature.
	tampered := tok[:len(tok)-2] + string('A'+'B'-tok[len(tok)-1])
	if err := j.VerifyAccess(tampered); err == nil {
		t.Error("VerifyAccess(tampered) = nil, want error")
	}
}

func TestTOTPRFC6238(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	// RFC 6238 test vector: T=59s, SHA1, 6 digits -> 287082.
	if !validateTOTPAt(secret, "287082", time.Unix(59, 0)) {
		t.Error("validateTOTPAt(RFC 6238 vector 287082) = false, want true")
	}
	if validateTOTPAt(secret, "000000", time.Unix(59, 0)) {
		t.Error("validateTOTPAt(wrong code) = true, want false")
	}
	// The pquerna defaults our ValidateTOTP relies on must match the vector.
	ok, err := totp.ValidateCustom("287082", secret, time.Unix(59, 0),
		totp.ValidateOpts{Period: 30, Skew: 1, Digits: otp.DigitsSix})
	if err != nil || !ok {
		t.Errorf("totp.ValidateCustom vector = %v, %v; want true, nil", ok, err)
	}
}

func TestBackupCodes(t *testing.T) {
	codes, hashes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes(): %v", err)
	}
	if len(codes) != 10 || len(hashes) != 10 {
		t.Fatalf("got %d codes / %d hashes, want 10/10", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 8 {
			t.Errorf("code %q: length = %d, want 8", c, len(c))
		}
		if !isBase32(c) {
			t.Errorf("code %q is not base32", c)
		}
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
	}
	for i, c := range codes {
		if hashes[i] != backupHash(c) {
			t.Errorf("hash[%d] does not match sha256(code)", i)
		}
	}
}

func isBase32(s string) bool {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	for _, r := range s {
		if !strings.ContainsRune(alphabet, r) {
			return false
		}
	}
	return true
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(5, 60*time.Millisecond)
	const ip = "203.0.113.7"
	for i := 0; i < 5; i++ {
		if !rl.Allow(ip) {
			t.Fatalf("Allow #%d = false, want true", i+1)
		}
	}
	if rl.Allow(ip) {
		t.Fatal("Allow #6 = true, want false")
	}
	time.Sleep(80 * time.Millisecond)
	if !rl.Allow(ip) {
		t.Fatal("Allow after window = false, want true")
	}
	// Other IPs are independent.
	if !rl.Allow("198.51.100.9") {
		t.Fatal("Allow(other ip) = false, want true")
	}
}

func newTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand key: %v", err)
	}
	return key
}
