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

func TestBuildCA(t *testing.T) {
	var (
		serialNumber int64 = 42

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

	ca, err := NewCABuilder(serialNumber).
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
		Build()
	if err != nil {
		t.Fatalf("Could not build a CA: %v", err)
	}

	caCert := ca.GetCertificate()
	caCertFile, err := ioutil.TempFile("", "ca.*.crt")
	if err != nil {
		t.Fatalf("Could not create a temporary file for CA certificate: %v", err)
	}
	defer os.Remove(caCertFile.Name())
	if _, err := caCertFile.Write(caCert); err != nil {
		t.Fatalf("Could not write CA certificate to a file: %v", err)
	}
	if err := caCertFile.Close(); err != nil {
		t.Fatalf("Could not close a CA certificate file: %v", err)
	}

	caKey := ca.GetPrivateKey()
	caKeyFile, err := ioutil.TempFile("", "ca.*.key")
	if err != nil {
		t.Fatalf("Could not create a temporary file for CA private key: %v", err)
	}
	defer os.Remove(caKeyFile.Name())
	if _, err := caKeyFile.Write(caKey); err != nil {
		t.Fatalf("Could not write CA private key to a file: %v", err)
	}
	if err := caKeyFile.Close(); err != nil {
		t.Fatalf("Could not close a CA private key file: %v", err)
	}

	caTLS, err := tls.LoadX509KeyPair(caCertFile.Name(), caKeyFile.Name())
	if err != nil {
		t.Fatalf("Could not load a CA key pair: %v", err)
	}

	newCert, err := x509.ParseCertificate(caTLS.Certificate[0])
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
