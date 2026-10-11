package store

import (
	"fmt"

	"github.com/Kcchouette/gowlarr/internal/crypt"
)

// marshalSecret converts a plaintext payload into the (plain, encrypted) pair
// persisted by the *_json / *_enc column couples.
//
// With a nil key the payload stays in the plain column (the current default
// everywhere in Gowlarr — encryption exists but is not wired up) and the
// encrypted column is left untouched. With a non-nil key the payload is
// encrypted and the plain column is emptied. Callers keep the two column
// writes on separate upserts so a key toggled off cannot silently revive a
// stale encrypted blob.
func marshalSecret(plaintext, key []byte) (plain string, encrypted []byte, err error) {
	if key != nil {
		encrypted, err = crypt.Encrypt(plaintext, key)
		if err != nil {
			return "", nil, fmt.Errorf("encrypting secret: %w", err)
		}
		return "", encrypted, nil
	}
	return string(plaintext), nil, nil
}

// unmarshalSecret is the read-side counterpart of marshalSecret: it prefers the
// encrypted column when a key is supplied and ciphertext is present, and falls
// back to the plaintext column otherwise. It returns nil for an empty payload
// (no row yet), which callers treat as "nothing stored".
func unmarshalSecret(plain string, encrypted []byte, key []byte) ([]byte, error) {
	if key != nil && len(encrypted) > 0 {
		decrypted, err := crypt.Decrypt(encrypted, key)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret: %w", err)
		}
		return decrypted, nil
	}
	if plain == "" {
		return nil, nil
	}
	return []byte(plain), nil
}
