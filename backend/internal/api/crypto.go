package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// encryptSecret encrypts plaintext with AES-256-GCM using masterSecret as the key material.
// The key is derived by SHA-256 hashing masterSecret, giving a stable 32-byte key.
// Output format: base64(12-byte nonce || ciphertext || 16-byte GCM tag).
// This is safe to store in the database; the only way to decrypt is with the same masterSecret.
func encryptSecret(masterSecret, plaintext string) (string, error) {
	if masterSecret == "" {
		return plaintext, nil // dev mode: no encryption when master secret not set
	}
	key := sha256.Sum256([]byte(masterSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// decryptSecret reverses encryptSecret.
// Returns the original plaintext or an error if decryption fails.
// If masterSecret is empty, returns ciphertext as-is (dev mode passthrough).
func decryptSecret(masterSecret, ciphertext string) (string, error) {
	if masterSecret == "" {
		return ciphertext, nil // dev mode passthrough
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		// If decoding fails, the value may be legacy plaintext — return as-is
		return ciphertext, nil
	}
	key := sha256.Sum256([]byte(masterSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		// Too short to be valid ciphertext — treat as legacy plaintext
		return ciphertext, nil
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		// Decryption failed — may be legacy plaintext stored before encryption was added
		return ciphertext, nil
	}
	return string(pt), nil
}
