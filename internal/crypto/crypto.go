package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	saltLen   = 16
	nonceLen  = 12
	keyLen    = 32
	argonTime = 3
	argonMem  = 64 * 1024
	argonPar  = 4
)

// Encrypt encrypts data with AES-256-GCM using an Argon2id-derived key.
// Output format: salt (16B) || nonce (12B) || ciphertext+tag
func Encrypt(data []byte, passphrase string) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMem, argonPar, keyLen)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// salt + nonce + ciphertext
	out := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// Decrypt reverses Encrypt. Returns error on wrong passphrase or corrupt data.
func Decrypt(data []byte, passphrase string) ([]byte, error) {
	if len(data) < saltLen+nonceLen+aes.BlockSize {
		return nil, errors.New("encrypted data too short")
	}

	salt := data[:saltLen]
	nonce := data[saltLen : saltLen+nonceLen]
	ciphertext := data[saltLen+nonceLen:]

	key := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMem, argonPar, keyLen)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decryption failed: wrong passphrase or corrupt data")
	}

	return plaintext, nil
}
