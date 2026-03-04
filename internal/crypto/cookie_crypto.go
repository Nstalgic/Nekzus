package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// CookieEncryptor handles encryption and decryption of cookie values using AES-256-GCM.
type CookieEncryptor struct {
	aead cipher.AEAD
}

// NewCookieEncryptor creates a new encryptor with the given 32-byte key.
// Returns an error if the key is not exactly 32 bytes (AES-256).
func NewCookieEncryptor(key []byte) (*CookieEncryptor, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &CookieEncryptor{aead: aead}, nil
}

// Encrypt encrypts a cookie value using AES-256-GCM.
// The nonce is prepended to the ciphertext.
func (e *CookieEncryptor) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends the encrypted data to nonce
	ciphertext := e.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertext, nil
}

// Decrypt decrypts an encrypted cookie value.
// Expects the nonce to be prepended to the ciphertext.
func (e *CookieEncryptor) Decrypt(ciphertext []byte) (string, error) {
	nonceSize := e.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, encrypted := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// GenerateEncryptionKey generates a cryptographically secure 32-byte key.
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}
