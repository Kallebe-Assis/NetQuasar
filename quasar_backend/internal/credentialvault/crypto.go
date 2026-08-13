// Package credentialvault cifra senhas de acesso (equipamento/servidor/site) em AES-GCM.
package credentialvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

const keyContext = "netquasar-credential-vault-v1"

var (
	errEmptySecret = errors.New("credentialvault: segredo vazio")
	errBadBlob     = errors.New("credentialvault: blob inválido")
)

// DeriveKey deriva uma chave AES-256 a partir do segredo de sessão.
func DeriveKey(secret []byte) ([]byte, error) {
	if len(secret) == 0 {
		return nil, errEmptySecret
	}
	sum := sha256.Sum256(append([]byte(keyContext+"\x00"), secret...))
	return sum[:], nil
}

// Encrypt devolve nonce || ciphertext+tag.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt lê nonce || ciphertext+tag.
func Decrypt(key, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < ns+gcm.Overhead() {
		return nil, errBadBlob
	}
	nonce, ct := blob[:ns], blob[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}
