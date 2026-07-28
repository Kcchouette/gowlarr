package crypt

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	salt, _ := GenerateSalt()
	key, _ := DeriveKey("test-passphrase", salt)

	plaintext := []byte("hello gowlarr secrets")
	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_WrongKey(t *testing.T) {
	salt, _ := GenerateSalt()
	key1, _ := DeriveKey("passphrase-one", salt)
	key2, _ := DeriveKey("passphrase-two", salt)

	encrypted, err := Encrypt([]byte("secret"), key1)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = Decrypt(encrypted, key2)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestEncrypt_TamperedCiphertext(t *testing.T) {
	salt, _ := GenerateSalt()
	key, _ := DeriveKey("test", salt)

	encrypted, err := Encrypt([]byte("secret"), key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip a bit in the ciphertext
	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = Decrypt(tampered, key)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	salt, _ := GenerateSalt()
	key, _ := DeriveKey("test", salt)

	encrypted, err := Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("decrypted length = %d, want 0", len(decrypted))
	}
}

func TestDeriveKey_Deterministic(t *testing.T) {
	salt := make([]byte, saltLen)
	for i := range salt {
		salt[i] = byte(i)
	}

	key1, _ := DeriveKey("passphrase", salt)
	key2, _ := DeriveKey("passphrase", salt)

	if !bytes.Equal(key1, key2) {
		t.Error("same passphrase+salt should produce same key")
	}
}

func TestDeriveKey_EmptyPassphrase(t *testing.T) {
	salt, _ := GenerateSalt()
	_, err := DeriveKey("", salt)
	if err == nil {
		t.Fatal("expected error for empty passphrase")
	}
}

func TestDeriveKey_ShortSalt(t *testing.T) {
	_, err := DeriveKey("passphrase", []byte("short"))
	if err == nil {
		t.Fatal("expected error for short salt")
	}
}

func TestDeriveKey_DifferentSalts(t *testing.T) {
	salt1, _ := GenerateSalt()
	salt2, _ := GenerateSalt()

	key1, _ := DeriveKey("passphrase", salt1)
	key2, _ := DeriveKey("passphrase", salt2)

	if bytes.Equal(key1, key2) {
		t.Error("different salts should produce different keys")
	}
}

func TestGenerateSalt_Uniqueness(t *testing.T) {
	salt1, _ := GenerateSalt()
	salt2, _ := GenerateSalt()

	if bytes.Equal(salt1, salt2) {
		t.Error("two generated salts should be different")
	}
}

func TestEncrypt_Decrypt_TooShort(t *testing.T) {
	salt, _ := GenerateSalt()
	key, _ := DeriveKey("test", salt)

	_, err := Decrypt([]byte("short"), key)
	if err == nil {
		t.Fatal("expected error for too-short ciphertext")
	}
}

func TestEncrypt_Decrypt_WrongKeyLength(t *testing.T) {
	_, err := Encrypt([]byte("test"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}

	_, err = Decrypt([]byte("ciphertext"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for wrong key length")
	}
}
