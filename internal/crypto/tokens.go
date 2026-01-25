package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
)

var (
	ErrNoEncryptionKey = errors.New("TOKEN_ENCRYPTION_KEY environment variable not set")
	ErrInvalidKey      = errors.New("encryption key must be 32 bytes for AES-256")
	ErrCiphertextShort = errors.New("ciphertext too short")
)

// getKey retrieves the encryption key from environment
func getKey() ([]byte, error) {
	keyStr := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if keyStr == "" {
		// In development, allow unencrypted tokens
		if os.Getenv("ENV") != "production" {
			return nil, nil
		}
		return nil, ErrNoEncryptionKey
	}

	key, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, err
	}

	if len(key) != 32 {
		return nil, ErrInvalidKey
	}

	return key, nil
}

// EncryptToken encrypts an OAuth token using AES-256-GCM
func EncryptToken(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key, err := getKey()
	if err != nil {
		return "", err
	}

	// No encryption key in development - return plaintext
	if key == nil {
		return plaintext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptToken decrypts an OAuth token encrypted with AES-256-GCM
func DecryptToken(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	key, err := getKey()
	if err != nil {
		return "", err
	}

	// No encryption key in development - return as-is
	if key == nil {
		return ciphertext, nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		// Token might be unencrypted (legacy) - return as-is
		return ciphertext, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		// Token might be unencrypted (legacy) - return as-is
		return ciphertext, nil
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		// Decryption failed - token might be unencrypted (legacy)
		return ciphertext, nil
	}

	return string(plaintext), nil
}

// GenerateEncryptionKey generates a new 32-byte key for AES-256
func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
