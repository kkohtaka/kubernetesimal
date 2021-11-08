package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

type KeyPairOption struct {
	BitSize int
	Random  io.Reader
}

func newDefaultKeyPairOption() *KeyPairOption {
	return &KeyPairOption{
		BitSize: 4096,
		Random:  rand.Reader,
	}
}

func newPrivateKeyPEM(privateKey *rsa.PrivateKey) []byte {
	der := x509.MarshalPKCS1PrivateKey(privateKey)
	block := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   der,
	}
	return pem.EncodeToMemory(&block)
}

func GenerateKeyPair(opts ...func(*KeyPairOption)) ([]byte, []byte, error) {
	o := newDefaultKeyPairOption()
	for _, fn := range opts {
		fn(o)
	}

	privateKey, err := rsa.GenerateKey(o.Random, o.BitSize)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't generate a private key: %w", err)
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't retrieve a public key from a private key: %w", err)
	}

	return newPrivateKeyPEM(privateKey), ssh.MarshalAuthorizedKey(publicKey), nil
}
