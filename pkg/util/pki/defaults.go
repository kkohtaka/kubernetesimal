package pki

import (
	"crypto/rand"
	"crypto/x509"
	"time"
)

const (
	defaultBits   = 2048
	defaultExpire = 30 * 24 * time.Hour

	defaultCAKeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
	defaultKeyUsage   = x509.KeyUsageDigitalSignature
)

var (
	defaultRandom = rand.Reader

	defaultExtKeyUsage = []x509.ExtKeyUsage{
		x509.ExtKeyUsageServerAuth,
		x509.ExtKeyUsageClientAuth,
	}
)
