package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"
)

// certCache caches generated leaf certificates keyed by hostname.
var certCache sync.Map

// SignHost generates a TLS certificate for the given hostname signed by the CA.
func SignHost(ca *CACert, hostname string) (*tls.Certificate, error) {
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}

	// Check cache first.
	if cached, ok := certCache.Load(hostname); ok {
		return cached.(*tls.Certificate), nil
	}

	// Generate leaf key.
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate leaf key: %w", err)
	}

	// Serial number.
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Ouroboros MITM"},
		},
		DNSNames:    []string{hostname},
		NotBefore:   now.Add(-24 * time.Hour),
		NotAfter:    now.Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	// Handle IP addresses.
	if ip := net.ParseIP(hostname); ip != nil {
		template.IPAddresses = []net.IP{ip}
		template.DNSNames = nil
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, ca.X509, &leafKey.PublicKey, ca.Cert.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}

	leafCert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		return nil, fmt.Errorf("parse leaf cert: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{derBytes, ca.Cert.Certificate[0]},
		PrivateKey:  leafKey,
		Leaf:        leafCert,
	}

	certCache.Store(hostname, tlsCert)
	return tlsCert, nil
}
