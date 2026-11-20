package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
)

// NewKeyPair generates a Reality X25519 key pair encoded as xray does:
// base64 RawURL, 43 characters per key. The private key is for the server
// inbound; the public key goes into client subscriptions.
func NewKeyPair() (private, public string, err error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}
