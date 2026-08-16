
package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

const KeyLength = 32

const NonceLength = 12

var ErrDecryptFailed = errors.New("crypto/aes: decrypt failed")

var ErrInvalidKey = errors.New("crypto/aes: master key must be 32 bytes")

type Cipher struct {
	key [KeyLength]byte
}

func NewCipher(key [KeyLength]byte) *Cipher {
	return &Cipher{key: key}
}

func LoadMasterKey(getenv func(string) string) ([KeyLength]byte, error) {
	raw := getenv("KUBECONFIG_ENC_KEY")
	if raw == "" {
		return [KeyLength]byte{}, fmt.Errorf("%w: KUBECONFIG_ENC_KEY not set", ErrInvalidKey)
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != KeyLength {
		return [KeyLength]byte{}, fmt.Errorf("%w: got %d bytes (want %d)", ErrInvalidKey, len(b), KeyLength)
	}
	var k [KeyLength]byte
	copy(k[:], b)
	return k, nil
}

func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, NonceLength)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	env := append(nonce, sealed...)
	return base64.StdEncoding.EncodeToString(env), nil
}

func (c *Cipher) Decrypt(envelope string) ([]byte, error) {
	if envelope == "" {
		return nil, ErrDecryptFailed
	}
	raw, err := base64.StdEncoding.DecodeString(envelope)
	if err != nil || len(raw) < NonceLength {
		return nil, ErrDecryptFailed
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	nonce, ct := raw[:NonceLength], raw[NonceLength:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plain, nil
}

func (c *Cipher) EncryptString(s string) (string, error) { return c.Encrypt([]byte(s)) }
func (c *Cipher) DecryptString(s string) (string, error) {
	b, err := c.Decrypt(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}