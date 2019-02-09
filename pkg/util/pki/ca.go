package pki

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"time"
)

type CABuilder struct {
	builder *CertificateBuilder
}

func NewCABuilder(serialNumber int64) *CABuilder {
	return &CABuilder{
		builder: NewCertificateBuilder(serialNumber),
	}
}

func (b *CABuilder) Random(random io.Reader) *CABuilder {
	b.builder.random = random
	return b
}

func (b *CABuilder) Country(country []string) *CABuilder {
	b.builder.country = country
	return b
}

func (b *CABuilder) Organization(organization []string) *CABuilder {
	b.builder.organization = organization
	return b
}

func (b *CABuilder) OrganizationalUnit(organizationalUnit []string) *CABuilder {
	b.builder.organizationalUnit = organizationalUnit
	return b
}

func (b *CABuilder) Province(province []string) *CABuilder {
	b.builder.province = province
	return b
}

func (b *CABuilder) Locality(locality []string) *CABuilder {
	b.builder.locality = locality
	return b
}

func (b *CABuilder) StreetAddress(streetAddress []string) *CABuilder {
	b.builder.streetAddress = streetAddress
	return b
}

func (b *CABuilder) PostalCode(postalCode []string) *CABuilder {
	b.builder.postalCode = postalCode
	return b
}

func (b *CABuilder) NotBefore(notBefore time.Time) *CABuilder {
	b.builder.notBefore = notBefore
	return b
}

func (b *CABuilder) NotAfter(notAfter time.Time) *CABuilder {
	b.builder.notAfter = notAfter
	return b
}

func (b *CABuilder) Build() (*CA, error) {
	cert := &x509.Certificate{
		SerialNumber: b.builder.serialNumber,
		NotBefore:    b.builder.notBefore,
		NotAfter:     b.builder.notAfter,

		Subject: pkix.Name{
			Country:            b.builder.country,
			Organization:       b.builder.organization,
			OrganizationalUnit: b.builder.organizationalUnit,
			Province:           b.builder.province,
			Locality:           b.builder.locality,
			StreetAddress:      b.builder.streetAddress,
			PostalCode:         b.builder.postalCode,
		},

		IsCA:                  true,
		KeyUsage:              defaultCAKeyUsage,
		ExtKeyUsage:           defaultExtKeyUsage,
		BasicConstraintsValid: true,
	}
	pk, err := rsa.GenerateKey(b.builder.random, b.builder.bits)
	if err != nil {
		return nil, fmt.Errorf("Could not generate a key: %v", err)
	}
	data, err := x509.CreateCertificate(
		b.builder.random,
		cert,
		cert,
		&pk.PublicKey,
		pk,
	)
	if err != nil {
		return nil, fmt.Errorf("Could not generate a certificate: %v", err)
	}
	return &CA{
		cert: cert,
		pk:   pk,
		data: data,
	}, nil
}

type CA struct {
	randReader io.Reader
	cert       *x509.Certificate
	pk         *rsa.PrivateKey
	data       []byte
}

func (ca *CA) GetCertificate() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ca.data,
	})
}

func (ca *CA) GetPrivateKey() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(ca.pk),
	})
}
