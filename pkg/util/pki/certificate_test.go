package pki

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"math/big"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestBuildCertificate(t *testing.T) {
	var (
		caSerialNumber int64 = 42
		serialNumber   int64 = 43

		country            = []string{"COUNTRY_CODE"}
		organization       = []string{"ORGANIZATION"}
		organizationalUnit = []string{"ORGANIZATIONAL_UNIT"}
		province           = []string{"PROVINCE"}
		locality           = []string{"LOCALITY"}
		streetAddress      = []string{"STREET_ADDRESS"}
		postalCode         = []string{"POSTAL_CODE"}

		notBefore = time.Now().Truncate(time.Second).UTC()
		notAfter  = time.Now().Truncate(time.Second).UTC().Add(30 * time.Second)
	)

	ca, err := NewCABuilder(caSerialNumber).Build()
	if err != nil {
		t.Fatalf("Could not build a CA: %v", err)
	}

	c, err := NewCertificateBuilder(serialNumber).
		Random(rand.Reader).
		Country(country).
		Organization(organization).
		OrganizationalUnit(organizationalUnit).
		Province(province).
		Locality(locality).
		StreetAddress(streetAddress).
		PostalCode(postalCode).
		NotBefore(notBefore).
		NotAfter(notAfter).
		Build(ca)
	if err != nil {
		t.Fatalf("Could not build a certificate: %v", err)
	}

	cert := c.GetCertificate()
	certFile, err := ioutil.TempFile("", "server.*.crt")
	if err != nil {
		t.Fatalf("Could not create a temporary file for CA certificate: %v", err)
	}
	defer os.Remove(certFile.Name())
	if _, err := certFile.Write(cert); err != nil {
		t.Fatalf("Could not write CA certificate to a file: %v", err)
	}
	if err := certFile.Close(); err != nil {
		t.Fatalf("Could not close a CA certificate file: %v", err)
	}

	key := c.GetPrivateKey()
	keyFile, err := ioutil.TempFile("", "server.*.key")
	if err != nil {
		t.Fatalf("Could not create a temporary file for CA private key: %v", err)
	}
	defer os.Remove(keyFile.Name())
	if _, err := keyFile.Write(key); err != nil {
		t.Fatalf("Could not write CA private key to a file: %v", err)
	}
	if err := keyFile.Close(); err != nil {
		t.Fatalf("Could not close a CA private key file: %v", err)
	}

	tls, err := tls.LoadX509KeyPair(certFile.Name(), keyFile.Name())
	if err != nil {
		t.Fatalf("Could not load a CA key pair: %v", err)
	}

	newCert, err := x509.ParseCertificate(tls.Certificate[0])
	if err != nil {
		t.Fatalf("Could not parse a CA certificate: %v", err)
	}

	if want, got := big.NewInt(serialNumber), newCert.SerialNumber; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := country, newCert.Subject.Country; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := organization, newCert.Subject.Organization; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := organizationalUnit, newCert.Subject.OrganizationalUnit; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := province, newCert.Subject.Province; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := locality, newCert.Subject.Locality; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := streetAddress, newCert.Subject.StreetAddress; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := postalCode, newCert.Subject.PostalCode; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := notBefore, newCert.NotBefore; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
	if want, got := notAfter, newCert.NotAfter; !reflect.DeepEqual(want, got) {
		t.Errorf("want: %v, got: %v", want, got)
	}
}
