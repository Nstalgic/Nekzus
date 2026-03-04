package crypto

import (
	"bytes"
	"testing"
)

func TestNewCookieEncryptor(t *testing.T) {
	tests := []struct {
		name    string
		keyLen  int
		wantErr bool
	}{
		{"valid 32-byte key", 32, false},
		{"invalid 16-byte key", 16, true},
		{"invalid 24-byte key", 24, true},
		{"invalid empty key", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keyLen)
			_, err := NewCookieEncryptor(key)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCookieEncryptor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCookieEncryptor_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	encryptor, err := NewCookieEncryptor(key)
	if err != nil {
		t.Fatalf("NewCookieEncryptor() error = %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple cookie", "session_id=abc123"},
		{"empty value", ""},
		{"special characters", "token=abc+def/ghi="},
		{"unicode", "user=\u4e2d\u6587"},
		{"long value", string(make([]byte, 4096))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := encryptor.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Ciphertext should be different from plaintext
			if tt.plaintext != "" && bytes.Equal(ciphertext, []byte(tt.plaintext)) {
				t.Error("Encrypt() ciphertext equals plaintext")
			}

			decrypted, err := encryptor.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestCookieEncryptor_DifferentNonces(t *testing.T) {
	key := make([]byte, 32)
	encryptor, err := NewCookieEncryptor(key)
	if err != nil {
		t.Fatalf("NewCookieEncryptor() error = %v", err)
	}

	plaintext := "same_cookie_value"

	// Encrypt the same value twice
	ciphertext1, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	ciphertext2, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	// Ciphertexts should be different due to random nonces
	if bytes.Equal(ciphertext1, ciphertext2) {
		t.Error("Encrypt() produced identical ciphertexts for same plaintext")
	}

	// Both should decrypt to the same value
	decrypted1, _ := encryptor.Decrypt(ciphertext1)
	decrypted2, _ := encryptor.Decrypt(ciphertext2)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Decrypt() failed for one of the ciphertexts")
	}
}

func TestCookieEncryptor_DecryptInvalid(t *testing.T) {
	key := make([]byte, 32)
	encryptor, err := NewCookieEncryptor(key)
	if err != nil {
		t.Fatalf("NewCookieEncryptor() error = %v", err)
	}

	tests := []struct {
		name       string
		ciphertext []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte("short")},
		{"corrupted", make([]byte, 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encryptor.Decrypt(tt.ciphertext)
			if err == nil {
				t.Error("Decrypt() expected error for invalid ciphertext")
			}
		})
	}
}

func TestCookieEncryptor_DifferentKeys(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1 // Different key

	encryptor1, _ := NewCookieEncryptor(key1)
	encryptor2, _ := NewCookieEncryptor(key2)

	plaintext := "secret_cookie"
	ciphertext, _ := encryptor1.Encrypt(plaintext)

	// Decrypting with wrong key should fail
	_, err := encryptor2.Decrypt(ciphertext)
	if err == nil {
		t.Error("Decrypt() should fail with different key")
	}
}

func TestGenerateEncryptionKey(t *testing.T) {
	key1, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("GenerateEncryptionKey() key length = %d, want 32", len(key1))
	}

	key2, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	// Keys should be different
	if bytes.Equal(key1, key2) {
		t.Error("GenerateEncryptionKey() produced identical keys")
	}
}
