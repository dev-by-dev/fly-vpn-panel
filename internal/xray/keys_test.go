package xray

import (
	"crypto/ecdh"
	"encoding/base64"
	"testing"
)

func TestKeyPairFormat(t *testing.T) {
	priv, pub, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair(): %v", err)
	}
	if len(priv) != 43 || len(pub) != 43 {
		t.Errorf("key lengths = %d/%d chars, want 43/43 (base64 RawURL of 32 bytes)", len(priv), len(pub))
	}
	privBytes, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil {
		t.Fatalf("private key is not RawURL base64: %v", err)
	}
	if len(privBytes) != 32 {
		t.Errorf("decoded private key = %d bytes, want 32", len(privBytes))
	}

	// The public key must be deterministically derivable from the private one.
	key, err := ecdh.X25519().NewPrivateKey(privBytes)
	if err != nil {
		t.Fatalf("priv bytes as X25519 key: %v", err)
	}
	want := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	if pub != want {
		t.Errorf("public = %q, want %q derived from private key", pub, want)
	}

	priv2, pub2, err := NewKeyPair()
	if err != nil {
		t.Fatalf("NewKeyPair() second call: %v", err)
	}
	if priv2 == priv || pub2 == pub {
		t.Error("second key pair identical to first, want fresh randomness")
	}
}
