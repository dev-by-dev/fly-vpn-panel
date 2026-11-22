package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token lifetimes and cookie names (spec-fixed).
const (
	AccessCookie  = "fp_access"
	RefreshCookie = "fp_refresh"
	AccessTTL     = 15 * time.Minute
	RefreshTTL    = 7 * 24 * time.Hour
)

// JWT issues and verifies the panel's HS256 access tokens. The signing key
// is supplied by the serve wiring (Task 10); tests use a random 32-byte key.
type JWT struct{ key []byte }

// NewJWT returns a JWT signer/verifier for key.
func NewJWT(key []byte) *JWT { return &JWT{key: key} }

// IssueAccess returns a signed token with claims {sub:"admin", exp: now+15m}.
func (j *JWT) IssueAccess() (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   "admin",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
	})
	return tok.SignedString(j.key)
}

// VerifyAccess validates the token's signature, algorithm, expiry and admin
// subject. Any problem is an error.
func (j *JWT) VerifyAccess(token string) error {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return j.key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return err
	}
	if claims.Subject != "admin" {
		return errors.New("token subject is not admin")
	}
	return nil
}

// IssueRefresh returns a random 32-byte base64url refresh token together
// with its SHA-256 hash (the DB storage form). Expiry is tracked by the
// store when inserting the hash.
func (j *JWT) IssueRefresh() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

// hashToken is the SHA-256 hex digest used as the storage key for refresh
// tokens and backup codes.
func hashToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// IssueAPIToken returns a new long-lived API token ("fp_" + 32 random bytes
// base64url) together with its storage hash.
func IssueAPIToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = "fp_" + base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}
