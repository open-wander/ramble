package crypto

import (
	"encoding/base64"
	"testing"
)

func TestGenerateEncryptionKey(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", err)
	}

	// Should be base64 encoded
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("GenerateEncryptionKey() returned non-base64 string: %v", err)
	}

	// Should be 32 bytes (AES-256)
	if len(decoded) != 32 {
		t.Errorf("GenerateEncryptionKey() key length = %d, want 32", len(decoded))
	}

	// Keys should be unique
	key2, _ := GenerateEncryptionKey()
	if key == key2 {
		t.Error("GenerateEncryptionKey() generated duplicate keys")
	}
}

func TestEncryptToken_EmptyInput(t *testing.T) {
	result, err := EncryptToken("")
	if err != nil {
		t.Fatalf("EncryptToken(\"\") error = %v", err)
	}
	if result != "" {
		t.Errorf("EncryptToken(\"\") = %q, want empty string", result)
	}
}

func TestDecryptToken_EmptyInput(t *testing.T) {
	result, err := DecryptToken("")
	if err != nil {
		t.Fatalf("DecryptToken(\"\") error = %v", err)
	}
	if result != "" {
		t.Errorf("DecryptToken(\"\") = %q, want empty string", result)
	}
}

func TestEncryptToken_NoKeyDevelopment(t *testing.T) {
	// Clear encryption key and ensure not in production
	t.Setenv("TOKEN_ENCRYPTION_KEY", "")
	t.Setenv("ENV", "development")

	plaintext := "test-oauth-token-12345"
	result, err := EncryptToken(plaintext)
	if err != nil {
		t.Fatalf("EncryptToken() error = %v", err)
	}

	// In development without key, should return plaintext
	if result != plaintext {
		t.Errorf("EncryptToken() = %q, want %q (passthrough in dev)", result, plaintext)
	}
}

func TestDecryptToken_NoKeyDevelopment(t *testing.T) {
	// Clear encryption key and ensure not in production
	t.Setenv("TOKEN_ENCRYPTION_KEY", "")
	t.Setenv("ENV", "development")

	ciphertext := "some-unencrypted-token"
	result, err := DecryptToken(ciphertext)
	if err != nil {
		t.Fatalf("DecryptToken() error = %v", err)
	}

	// In development without key, should return as-is
	if result != ciphertext {
		t.Errorf("DecryptToken() = %q, want %q (passthrough in dev)", result, ciphertext)
	}
}

func TestEncryptToken_ProductionNoKey(t *testing.T) {
	t.Setenv("TOKEN_ENCRYPTION_KEY", "")
	t.Setenv("ENV", "production")

	_, err := EncryptToken("test-token")
	if err != ErrNoEncryptionKey {
		t.Errorf("EncryptToken() error = %v, want %v", err, ErrNoEncryptionKey)
	}
}

func TestDecryptToken_ProductionNoKey(t *testing.T) {
	t.Setenv("TOKEN_ENCRYPTION_KEY", "")
	t.Setenv("ENV", "production")

	_, err := DecryptToken("encrypted-data")
	if err != ErrNoEncryptionKey {
		t.Errorf("DecryptToken() error = %v, want %v", err, ErrNoEncryptionKey)
	}
}

func TestEncryptToken_WithValidKey(t *testing.T) {
	// Generate a valid key
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	plaintext := "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	encrypted, err := EncryptToken(plaintext)
	if err != nil {
		t.Fatalf("EncryptToken() error = %v", err)
	}

	// Encrypted output should be base64 and different from input
	if encrypted == plaintext {
		t.Error("EncryptToken() returned plaintext, expected encrypted value")
	}

	// Should be valid base64
	_, err = base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Errorf("EncryptToken() returned non-base64 string: %v", err)
	}
}

func TestDecryptToken_WithValidKey(t *testing.T) {
	// Generate a valid key
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	plaintext := "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	// First encrypt
	encrypted, err := EncryptToken(plaintext)
	if err != nil {
		t.Fatalf("EncryptToken() error = %v", err)
	}

	// Then decrypt
	decrypted, err := DecryptToken(encrypted)
	if err != nil {
		t.Fatalf("DecryptToken() error = %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("DecryptToken() = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	testCases := []string{
		"simple-token",
		"ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"token with spaces and special chars !@#$%^&*()",
		"unicode-token-\u4e2d\u6587-\u65e5\u672c\u8a9e",
		"a",                        // single char
		string(make([]byte, 1000)), // long token
	}

	for _, plaintext := range testCases {
		t.Run(plaintext[:min(20, len(plaintext))], func(t *testing.T) {
			encrypted, err := EncryptToken(plaintext)
			if err != nil {
				t.Fatalf("EncryptToken() error = %v", err)
			}

			decrypted, err := DecryptToken(encrypted)
			if err != nil {
				t.Fatalf("DecryptToken() error = %v", err)
			}

			if decrypted != plaintext {
				t.Errorf("Round trip failed: got %q, want %q", decrypted, plaintext)
			}
		})
	}
}

func TestEncryptToken_DifferentNonces(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	plaintext := "same-token"

	// Encrypt same plaintext twice
	encrypted1, _ := EncryptToken(plaintext)
	encrypted2, _ := EncryptToken(plaintext)

	// Should produce different ciphertexts due to random nonce
	if encrypted1 == encrypted2 {
		t.Error("EncryptToken() produced same ciphertext for same plaintext (nonce reuse)")
	}

	// Both should decrypt to same value
	decrypted1, _ := DecryptToken(encrypted1)
	decrypted2, _ := DecryptToken(encrypted2)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Different encryptions didn't decrypt to same plaintext")
	}
}

func TestGetKey_InvalidBase64(t *testing.T) {
	t.Setenv("TOKEN_ENCRYPTION_KEY", "not-valid-base64!!!")
	t.Setenv("ENV", "production")

	_, err := EncryptToken("test")
	if err == nil {
		t.Error("EncryptToken() should error on invalid base64 key")
	}
}

func TestGetKey_WrongLength(t *testing.T) {
	// Create a key that's too short (16 bytes instead of 32)
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	t.Setenv("TOKEN_ENCRYPTION_KEY", shortKey)
	t.Setenv("ENV", "production")

	_, err := EncryptToken("test")
	if err != ErrInvalidKey {
		t.Errorf("EncryptToken() error = %v, want %v", err, ErrInvalidKey)
	}
}

func TestGetKey_TooLong(t *testing.T) {
	// Create a key that's too long (64 bytes instead of 32)
	longKey := base64.StdEncoding.EncodeToString(make([]byte, 64))
	t.Setenv("TOKEN_ENCRYPTION_KEY", longKey)
	t.Setenv("ENV", "production")

	_, err := EncryptToken("test")
	if err != ErrInvalidKey {
		t.Errorf("EncryptToken() error = %v, want %v", err, ErrInvalidKey)
	}
}

func TestDecryptToken_LegacyUnencrypted(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	// Simulate a legacy unencrypted token (not base64)
	legacyToken := "ghp_plaintext_legacy_token"
	result, err := DecryptToken(legacyToken)
	if err != nil {
		t.Fatalf("DecryptToken() error = %v", err)
	}

	// Should return as-is for legacy tokens
	if result != legacyToken {
		t.Errorf("DecryptToken() = %q, want %q (legacy passthrough)", result, legacyToken)
	}
}

func TestDecryptToken_ShortCiphertext(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	// Base64 that decodes to something shorter than nonce size
	shortData := base64.StdEncoding.EncodeToString([]byte("short"))
	result, err := DecryptToken(shortData)
	if err != nil {
		t.Fatalf("DecryptToken() error = %v", err)
	}

	// Should return as-is when too short (legacy token handling)
	if result != shortData {
		t.Errorf("DecryptToken() = %q, want %q (legacy passthrough for short data)", result, shortData)
	}
}

func TestDecryptToken_InvalidCiphertext(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	t.Setenv("TOKEN_ENCRYPTION_KEY", key)
	t.Setenv("ENV", "production")

	// Valid base64, long enough, but not actually encrypted with our key
	fakeData := make([]byte, 50)
	for i := range fakeData {
		fakeData[i] = byte(i)
	}
	fakeCiphertext := base64.StdEncoding.EncodeToString(fakeData)

	result, err := DecryptToken(fakeCiphertext)
	if err != nil {
		t.Fatalf("DecryptToken() error = %v", err)
	}

	// Should return as-is when decryption fails (legacy token handling)
	if result != fakeCiphertext {
		t.Errorf("DecryptToken() = %q, want %q (legacy passthrough for invalid ciphertext)", result, fakeCiphertext)
	}
}
