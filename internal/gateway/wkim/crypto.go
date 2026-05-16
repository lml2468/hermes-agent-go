package wkim

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"

	"golang.org/x/crypto/curve25519"
)

// DHKeyPair holds a Curve25519 key pair for Diffie-Hellman exchange.
type DHKeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

// GenerateDHKeyPair creates a new Curve25519 key pair.
func GenerateDHKeyPair() (*DHKeyPair, error) {
	var privateKey [32]byte
	if _, err := rand.Read(privateKey[:]); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("compute public key: %w", err)
	}

	return &DHKeyPair{
		PrivateKey: privateKey[:],
		PublicKey:  publicKey,
	}, nil
}

// PublicKeyBase64 returns the base64-encoded public key.
func (kp *DHKeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PublicKey)
}

// DeriveAESKeys computes the shared secret and derives AES key/IV.
func DeriveAESKeys(privateKey []byte, serverPubKeyB64, salt string) (aesKey, aesIV []byte, err error) {
	serverPubKey, err := base64.StdEncoding.DecodeString(serverPubKeyB64)
	if err != nil {
		return nil, nil, fmt.Errorf("decode server key: %w", err)
	}

	sharedSecret, err := curve25519.X25519(privateKey, serverPubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("compute shared secret: %w", err)
	}

	secretB64 := base64.StdEncoding.EncodeToString(sharedSecret)
	aesKeyFull := fmt.Sprintf("%x", md5.Sum([]byte(secretB64)))
	aesKey = []byte(aesKeyFull[:16])

	if len(salt) > 16 {
		aesIV = []byte(salt[:16])
	} else {
		aesIV = []byte(salt)
	}

	return aesKey, aesIV, nil
}

// AESDecryptCBC decrypts data using AES-CBC with PKCS7 padding removal.
// The input may be base64-encoded (as in WuKongIM protocol).
func AESDecryptCBC(encrypted, key, iv []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(string(encrypted))
	if err != nil {
		ciphertext = encrypted
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	if len(ciphertext) < aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext length: %d", len(ciphertext))
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	plaintext, err = pkcs7Unpad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("pkcs7 unpad: %w", err)
	}

	return plaintext, nil
}

// AESEncryptCBC encrypts data using AES-CBC with PKCS7 padding.
func AESEncryptCBC(plaintext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return ciphertext, nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding > aes.BlockSize || padding == 0 {
		return nil, fmt.Errorf("invalid padding: %d", padding)
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid pkcs7 padding")
		}
	}
	return data[:len(data)-padding], nil
}

// GenerateDeviceID produces a random 32-character hex device identifier.
func GenerateDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	id := new(big.Int).SetBytes(b)
	return fmt.Sprintf("%032x", id)[:32]
}
