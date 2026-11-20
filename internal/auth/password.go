// Package auth provides password hashing, JWT tokens, TOTP enrollment,
// backup codes, login rate limiting and the auth HTTP handlers.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters for admin password hashing (spec: t=1, m=64MiB, p=4).
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB, i.e. 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword hashes pw with argon2id and returns the PHC string
// $argon2id$v=19$m=65536,t=1,p=4$<salt>$<key>.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2 salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// VerifyPassword reports whether pw matches the stored PHC hash. It returns
// false (never an error) for malformed or foreign-format hashes.
func VerifyPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	version, err := parseKV(parts[2])
	if err != nil || version.key != "v" || version.val != argon2.Version {
		return false
	}
	m, t, p, ok := parseMTP(parts[3])
	if !ok {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

type kv struct {
	key string
	val uint64
}

func parseKV(s string) (kv, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return kv{}, fmt.Errorf("bad param %q", s)
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return kv{}, err
	}
	return kv{key: k, val: n}, nil
}

func parseMTP(s string) (m, t uint32, p uint8, ok bool) {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return 0, 0, 0, false
	}
	got := map[string]uint32{}
	for _, f := range fields {
		pair, err := parseKV(f)
		if err != nil {
			return 0, 0, 0, false
		}
		got[pair.key] = uint32(pair.val)
	}
	m, okM := got["m"]
	t, okT := got["t"]
	pv, okP := got["p"]
	if !okM || !okT || !okP || pv > 255 {
		return 0, 0, 0, false
	}
	return m, t, uint8(pv), true
}
