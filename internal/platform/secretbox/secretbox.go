// Package secretbox encrypts small secrets stored in the database (TOTP
// seeds) with AES-256-GCM under the deployment's NITHIDESK_SECRETS_KEY.
//
// The ciphertext is version || nonce || sealed. The additional data binds a
// ciphertext to the row it belongs to, so a secret copied onto another
// user's row does not decrypt.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const version byte = 1

// Box seals and opens secrets with one key.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("secretbox: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext bound to additional data ad.
func (b *Box) Seal(plaintext, ad []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secretbox: nonce: %w", err)
	}
	out := append([]byte{version}, nonce...)
	return b.aead.Seal(out, nonce, plaintext, ad), nil
}

// ErrOpen is returned for any ciphertext that does not decrypt.
var ErrOpen = errors.New("secretbox: cannot open")

// Open decrypts a value produced by Seal with the same ad.
func (b *Box) Open(ciphertext, ad []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(ciphertext) < 1+n || ciphertext[0] != version {
		return nil, ErrOpen
	}
	plain, err := b.aead.Open(nil, ciphertext[1:1+n], ciphertext[1+n:], ad)
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}
