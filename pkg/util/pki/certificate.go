package pki

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"time"
)

type CertificateBuilder struct {
	random io.Reader
	bits   int

	serialNumber *big.Int

	country            []string
	organization       []string
	organizationalUnit []string
	province           []string
	locality           []string
	streetAddress      []string
	postalCode         []string

	notBefore time.Time
	notAfter  time.Time
}

func NewCertificateBuilder(serialNumber int64) *CertificateBuilder {
	return &CertificateBuilder{
		serialNumber: big.NewInt(serialNumber),
		random:       defaultRandom,
		bits:         defaultBits,
		notBefore:    time.Now(),
		notAfter:     time.Now().Add(defaultExpire),
	}
}

func (b *CertificateBuilder) Random(random io.Reader) *CertificateBuilder {
	b.random = random
	return b
}

func (b *CertificateBuilder) Country(country []string) *CertificateBuilder {
	b.country = country
	return b
}

func (b *CertificateBuilder) Organization(organization []string) *CertificateBuilder {
	b.organization = organization
	return b
}

func (b *CertificateBuilder) OrganizationalUnit(organizationalUnit []string) *CertificateBuilder {
	b.organizationalUnit = organizationalUnit
	return b
}

func (b *CertificateBuilder) Province(province []string) *CertificateBuilder {
	b.province = province
	return b
}

func (b *CertificateBuilder) Locality(locality []string) *CertificateBuilder {
	b.locality = locality
	return b
}

func (b *CertificateBuilder) StreetAddress(streetAddress []string) *CertificateBuilder {
	b.streetAddress = streetAddress
	return b
}

func (b *CertificateBuilder) PostalCode(postalCode []string) *CertificateBuilder {
	b.postalCode = postalCode
	return b
}

func (b *CertificateBuilder) NotBefore(notBefore time.Time) *CertificateBuilder {
	b.notBefore = notBefore
	return b
}

func (b *CertificateBuilder) NotAfter(notAfter time.Time) *CertificateBuilder {
	b.notAfter = notAfter
	return b
}

func (b *CertificateBuilder) Build(ca *CA) (*Certificate, error) {
	cert := &x509.Certificate{
		SerialNumber: b.serialNumber,
		NotBefore:    b.notBefore,
		NotAfter:     b.notAfter,

		Subject: pkix.Name{
			Country:            b.country,
			Organization:       b.organization,
			OrganizationalUnit: b.organizationalUnit,
			Province:           b.province,
			Locality:           b.locality,
			StreetAddress:      b.streetAddress,
			PostalCode:         b.postalCode,
		},

		KeyUsage:    defaultKeyUsage,
		ExtKeyUsage: defaultExtKeyUsage,
	}
	pk, err := rsa.GenerateKey(b.random, b.bits)
	if err != nil {
		return nil, fmt.Errorf("Could not generate a key: %v", err)
	}
	data, err := x509.CreateCertificate(
		b.random,
		cert,
		ca.cert,
		&pk.PublicKey,
		pk,
	)
	if err != nil {
		return nil, fmt.Errorf("Could not generate a certificate: %v", err)
	}
	return &Certificate{
		cert: cert,
		pk:   pk,
		data: data,
	}, nil
}

type Certificate struct {
	cert *x509.Certificate
	pk   *rsa.PrivateKey
	data []byte
}

func (c *Certificate) GetCertificate() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: c.data,
	})
}

func (c *Certificate) GetPrivateKey() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(c.pk),
	})
}
