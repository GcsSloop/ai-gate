package secrets_test

import (
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/secrets"
)

func TestCipherRoundTrip(t *testing.T) {
	t.Parallel()

	cipher, err := secrets.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}

	encrypted, err := cipher.EncryptString("top-secret-token")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}

	decrypted, err := cipher.DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString returned error: %v", err)
	}

	if decrypted != "top-secret-token" {
		t.Fatalf("DecryptString = %q, want %q", decrypted, "top-secret-token")
	}
}

func TestNewCipherRejectsShortKeys(t *testing.T) {
	t.Parallel()

	_, err := secrets.NewCipher("short")
	if err == nil {
		t.Fatal("NewCipher returned nil error, want validation error")
	}
}

func TestEncryptStringUsesUniqueNonce(t *testing.T) {
	t.Parallel()

	cipher, err := secrets.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewCipher returned error: %v", err)
	}

	first, err := cipher.EncryptString("same-plaintext")
	if err != nil {
		t.Fatalf("EncryptString(first) returned error: %v", err)
	}

	second, err := cipher.EncryptString("same-plaintext")
	if err != nil {
		t.Fatalf("EncryptString(second) returned error: %v", err)
	}

	if first == second {
		t.Fatal("EncryptString returned identical ciphertexts, want unique values")
	}
}
