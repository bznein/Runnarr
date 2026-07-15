package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type TokenCipher struct {
	aead cipher.AEAD
}

func NewTokenCipher(key []byte) (*TokenCipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &TokenCipher{aead: aead}, nil
}

func (c *TokenCipher) EncryptString(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := c.aead.Seal(nonce, nonce, []byte(value), nil)
	return out, nil
}

func (c *TokenCipher) DecryptString(value []byte) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	nonceSize := c.aead.NonceSize()
	if len(value) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	plain, err := c.aead.Open(nil, value[:nonceSize], value[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
