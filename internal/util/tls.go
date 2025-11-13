package util

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/kirychukyurii/webitel-dc-switcher/internal/config"
)

// LoadTLSConfig loads TLS configuration from the provided config
func LoadTLSConfig(cfg *config.TLSConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil
	}

	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(cfg.CA)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	// Create CA certificate pool
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, nil
}
