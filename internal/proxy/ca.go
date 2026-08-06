package proxy

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
	"path/filepath"
	"time"
)

// CACert holds the CA certificate and private key for MITM signing.
type CACert struct {
	Cert tls.Certificate
	X509 *x509.Certificate
}

// CACertDir returns the directory for storing the CA certificate and key.
func CACertDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "sentinel")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadOrGenerateCA loads an existing CA from disk or generates a new one.
func LoadOrGenerateCA() (*CACert, error) {
	dir, err := CACertDir()
	if err != nil {
		return nil, err
	}

	certPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca-key.pem")

	// Try loading existing.
	if ca, err := loadCAFromDisk(certPath, keyPath); err == nil {
		return ca, nil
	}

	// Generate new CA.
	ca, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// Save to disk.
	if err := saveCAToDisk(ca, certPath, keyPath); err != nil {
		return nil, fmt.Errorf("save CA: %w", err)
	}

	return ca, nil
}

func loadCAFromDisk(certPath, keyPath string) (*CACert, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode CA key PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode CA cert PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certBlock.Bytes},
		PrivateKey:  key,
		Leaf:        cert,
	}

	return &CACert{Cert: tlsCert, X509: cert}, nil
}

func generateCA() (*CACert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Sentinel MITM CA",
			Organization: []string{"Sentinel"},
		},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  key,
		Leaf:        cert,
	}

	return &CACert{Cert: tlsCert, X509: cert}, nil
}

func saveCAToDisk(ca *CACert, certPath, keyPath string) error {
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert.Certificate[0]})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return err
	}

	keyBytes, err := x509.MarshalECPrivateKey(ca.Cert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		return fmt.Errorf("marshal CA key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return err
	}

	return nil
}

// PrintCACert prints the CA certificate PEM to stdout for browser installation.
func PrintCACert() error {
	ca, err := LoadOrGenerateCA()
	if err != nil {
		return err
	}
	os.Stdout.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert.Certificate[0]}))
	return nil
}
