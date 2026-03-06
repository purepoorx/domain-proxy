package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"
)

type Store struct {
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	cache  sync.Map // map[string]*tls.Certificate
}

func NewStore(certPath, keyPath string) (*Store, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode CA cert PEM")
	}

	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode CA key PEM")
	}

	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}

	return &Store{
		caCert: caCert,
		caKey:  caKey,
	}, nil
}

func (s *Store) GetOrCreateCert(hostname string) (*tls.Certificate, error) {
	if cached, ok := s.cache.Load(hostname); ok {
		return cached.(*tls.Certificate), nil
	}

	cert, err := s.issueCert(hostname)
	if err != nil {
		return nil, err
	}

	s.cache.Store(hostname, cert)
	return cert, nil
}

func (s *Store) issueCert(hostname string) (*tls.Certificate, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key for %s: %w", hostname, err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   hostname,
			Organization: []string{"Domain Proxy"},
		},
		DNSNames:  []string{hostname},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, s.caCert, &privKey.PublicKey, s.caKey)
	if err != nil {
		return nil, fmt.Errorf("sign cert for %s: %w", hostname, err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privKey,
	}

	return tlsCert, nil
}

func (s *Store) CACert() *x509.Certificate {
	return s.caCert
}
