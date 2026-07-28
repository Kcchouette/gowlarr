package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 256 * 1024
	argonThreads = 4
	keyLen       = 32
	saltLen      = 16
	nonceLen     = 12
)

func Encrypt(plaintext, key []byte) ([]byte, error) {
	if len(key) != keyLen {
		return nil, errors.New("crypt: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(ciphertext, key []byte) ([]byte, error) {
	if len(key) != keyLen {
		return nil, errors.New("crypt: key must be 32 bytes")
	}
	if len(ciphertext) < nonceLen {
		return nil, errors.New("crypt: ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, ct := ciphertext[:nonceLen], ciphertext[nonceLen:]
	return gcm.Open(nil, nonce, ct, nil)
}

func DeriveKey(passphrase string, salt []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, errors.New("crypt: empty passphrase")
	}
	if len(salt) < saltLen {
		return nil, errors.New("crypt: salt must be at least 16 bytes")
	}
	return argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, keyLen), nil
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	_, err := io.ReadFull(rand.Reader, salt)
	return salt, err
}
