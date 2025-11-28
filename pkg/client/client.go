package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/traefik/ingress-nginx-analyzer/pkg/analyzer"
)

// Token is the authentication token used for sending reports injected at build time.
var Token = "dev"

// mTLS certificates injected at build time (base64 encoded).
var (
	// ClientCertB64 is the base64-encoded client certificate (PEM format).
	ClientCertB64 = ""
	// ClientKeyB64 is the base64-encoded client private key (PEM format).
	ClientKeyB64 = ""
	// CACertB64 is the base64-encoded CA certificate (PEM format) for verifying the server.
	CACertB64 = ""
)

// Client is a client for sending reports to a remote endpoint.
type Client struct {
	endpointURL string
	httpClient  *http.Client
}

// New creates a new Client.
func New(endpointURL string) (*Client, error) {
	// When no endpointURL is provided, use the default one.
	if endpointURL == "" {
		endpointURL = "https://collect.ingressnginxmigration.org/a2181946f5561e7e7405000e5c94de97"
	}

	tlsConfig, err := buildTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("building TLS config: %w", err)
	}

	return &Client{
		endpointURL: endpointURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSClientConfig: tlsConfig,
			},
		},
	}, nil
}

// reportPayload is a lightweight version of analyzer.Report for API transmission.
type reportPayload struct {
	IngressCount            int `json:"ingressCount"`
	CompatibleIngressCount  int `json:"compatibleIngressCount"`
	VanillaIngressCount     int `json:"vanillaIngressCount"`
	SupportedIngressCount   int `json:"supportedIngressCount"`
	UnsupportedIngressCount int `json:"unsupportedIngressCount"`

	UnsupportedIngressAnnotations map[string]int `json:"unsupportedIngressAnnotations"`
}

func (c *Client) SendReport(report analyzer.Report) error {
	payload := reportPayload{
		IngressCount:                  report.IngressCount,
		CompatibleIngressCount:        report.CompatibleIngressCount,
		VanillaIngressCount:           report.VanillaIngressCount,
		SupportedIngressCount:         report.SupportedIngressCount,
		UnsupportedIngressCount:       report.UnsupportedIngressCount,
		UnsupportedIngressAnnotations: report.UnsupportedIngressAnnotations,
	}

	reportBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling report to JSON: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.endpointURL, bytes.NewBuffer(reportBytes))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+Token)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting report to %s: %w", c.endpointURL, err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode/100 != 2 {
		return fmt.Errorf("invalid response status code: %d", res.StatusCode)
	}

	return nil
}

func buildTLSConfig() (*tls.Config, error) {
	// If no certificates are provided, return nil (use default TLS config).
	if ClientCertB64 == "" || ClientKeyB64 == "" {
		return nil, nil
	}

	// Decode client certificate and key from base64.
	clientCertPEM, err := base64.StdEncoding.DecodeString(ClientCertB64)
	if err != nil {
		return nil, fmt.Errorf("decoding client certificate: %w", err)
	}

	clientKeyPEM, err := base64.StdEncoding.DecodeString(ClientKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decoding client key: %w", err)
	}

	// Load client certificate and key.
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}

	// If CA certificate is provided, use it to verify the server.
	if CACertB64 != "" {
		caCertPEM, err := base64.StdEncoding.DecodeString(CACertB64)
		if err != nil {
			return nil, fmt.Errorf("decoding CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertPEM) {
			return nil, errors.New("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}
