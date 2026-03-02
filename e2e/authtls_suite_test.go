package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	authTLSRequiredIngressName = "mtls-required-test"
	authTLSRequiredTraefikHost = authTLSRequiredIngressName + ".traefik.local"
	authTLSRequiredNginxHost   = authTLSRequiredIngressName + ".nginx.local"

	authTLSOptionalIngressName = "mtls-optional-test"
	authTLSOptionalTraefikHost = authTLSOptionalIngressName + ".traefik.local"
	authTLSOptionalNginxHost   = authTLSOptionalIngressName + ".nginx.local"

	authTLSOffIngressName = "mtls-off-test"
	authTLSOffTraefikHost = authTLSOffIngressName + ".traefik.local"
	authTLSOffNginxHost   = authTLSOffIngressName + ".nginx.local"

	authTLSPassCertIngressName = "mtls-pass-cert-test"
	authTLSPassCertTraefikHost = authTLSPassCertIngressName + ".traefik.local"
	authTLSPassCertNginxHost   = authTLSPassCertIngressName + ".nginx.local"

	authTLSCACertSecretName    = "mtls-ca-cert"
	authTLSServerTLSSecretName = "mtls-server-tls"
)

// authTLSCerts holds all generated certificates for the auth-tls test suite.
type authTLSCerts struct {
	caCertPEM  []byte
	caKey      *ecdsa.PrivateKey
	caCert     *x509.Certificate
	clientCert tls.Certificate
	// invalidClientCert is signed by a different CA.
	invalidClientCert tls.Certificate
}

type AuthTLSSuite struct {
	BaseSuite

	certs        authTLSCerts
	traefikHTTPS string // host:port for HTTPS on traefik cluster
	nginxHTTPS   string // host:port for HTTPS on nginx cluster
}

func TestAuthTLSSuite(t *testing.T) {
	suite.Run(t, new(AuthTLSSuite))
}

func (s *AuthTLSSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Generate all TLS certificates.
	certs, err := generateAuthTLSCerts()
	require.NoError(s.T(), err, "generate mTLS certificates")
	s.certs = certs

	// Resolve HTTPS ports from the cluster structs.
	s.traefikHTTPS = fmt.Sprintf("%s:%s", s.traefik.Host, s.traefik.PortHTTPS)
	s.nginxHTTPS = fmt.Sprintf("%s:%s", s.nginx.Host, s.nginx.PortHTTPS)

	// Deploy the CA certificate secret to both clusters.
	// The provider reads ca.crt from the secret data.
	caSecretData := secretTemplateData{
		Name: authTLSCACertSecretName,
		Type: "Opaque",
		Data: map[string]string{
			"ca.crt": base64.StdEncoding.EncodeToString(s.certs.caCertPEM),
		},
	}

	err = s.traefik.DeploySecret(caSecretData)
	require.NoError(s.T(), err, "deploy CA secret to traefik cluster")

	err = s.nginx.DeploySecret(caSecretData)
	require.NoError(s.T(), err, "deploy CA secret to nginx cluster")

	// Deploy a server TLS secret for the ingress TLS termination.
	serverCert, serverKey, err := generateServerCert(s.certs.caCert, s.certs.caKey,
		authTLSRequiredTraefikHost, authTLSRequiredNginxHost,
		authTLSOptionalTraefikHost, authTLSOptionalNginxHost,
		authTLSOffTraefikHost, authTLSOffNginxHost,
		authTLSPassCertTraefikHost, authTLSPassCertNginxHost,
	)
	require.NoError(s.T(), err, "generate server certificate")

	serverTLSSecret := secretTemplateData{
		Name: authTLSServerTLSSecretName,
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(serverCert),
			"tls.key": base64.StdEncoding.EncodeToString(serverKey),
		},
	}

	err = s.traefik.DeploySecret(serverTLSSecret)
	require.NoError(s.T(), err, "deploy server TLS secret to traefik cluster")

	err = s.nginx.DeploySecret(serverTLSSecret)
	require.NoError(s.T(), err, "deploy server TLS secret to nginx cluster")

	// Deploy ingresses with various auth-tls configurations.

	// 1. auth-tls-verify-client="on" (required).
	s.deployAuthTLSIngress(authTLSRequiredIngressName, authTLSRequiredTraefikHost, authTLSRequiredNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})

	// 2. auth-tls-verify-client="optional".
	s.deployAuthTLSIngress(authTLSOptionalIngressName, authTLSOptionalTraefikHost, authTLSOptionalNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "optional",
	})

	// 3. auth-tls-verify-client="off".
	s.deployAuthTLSIngress(authTLSOffIngressName, authTLSOffTraefikHost, authTLSOffNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "off",
	})

	// 4. auth-tls-pass-certificate-to-upstream="true" with verify-client="on".
	s.deployAuthTLSIngress(authTLSPassCertIngressName, authTLSPassCertTraefikHost, authTLSPassCertNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "on",
		"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
	})

	// Wait for all ingresses to be ready.
	s.traefik.WaitForIngressReady(s.T(), authTLSRequiredTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSRequiredNginxHost, 30, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authTLSOptionalTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSOptionalNginxHost, 30, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authTLSOffTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSOffNginxHost, 30, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authTLSPassCertTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSPassCertNginxHost, 30, 1*time.Second)
}

func (s *AuthTLSSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(authTLSRequiredIngressName)
	_ = s.nginx.DeleteIngress(authTLSRequiredIngressName)
	_ = s.traefik.DeleteIngress(authTLSOptionalIngressName)
	_ = s.nginx.DeleteIngress(authTLSOptionalIngressName)
	_ = s.traefik.DeleteIngress(authTLSOffIngressName)
	_ = s.nginx.DeleteIngress(authTLSOffIngressName)
	_ = s.traefik.DeleteIngress(authTLSPassCertIngressName)
	_ = s.nginx.DeleteIngress(authTLSPassCertIngressName)
	_ = s.traefik.DeleteSecret(authTLSCACertSecretName)
	_ = s.nginx.DeleteSecret(authTLSCACertSecretName)
	_ = s.traefik.DeleteSecret(authTLSServerTLSSecretName)
	_ = s.nginx.DeleteSecret(authTLSServerTLSSecretName)
}

// deployAuthTLSIngress deploys an ingress with TLS termination and the given annotations to both clusters.
func (s *AuthTLSSuite) deployAuthTLSIngress(name, traefikHost, nginxHost string, annotations map[string]string) {
	s.T().Helper()

	traefikManifest := renderTLSIngressManifest(s.traefik.IngressName(name), traefikHost, authTLSServerTLSSecretName, annotations)
	err := s.traefik.ApplyManifest(traefikManifest)
	require.NoError(s.T(), err, "deploy %s ingress to traefik cluster", name)

	nginxManifest := renderTLSIngressManifest(s.nginx.IngressName(name), nginxHost, authTLSServerTLSSecretName, annotations)
	err = s.nginx.ApplyManifest(nginxManifest)
	require.NoError(s.T(), err, "deploy %s ingress to nginx cluster", name)
}

// renderTLSIngressManifest renders an ingress YAML manifest with a TLS section.
func renderTLSIngressManifest(name, host, tlsSecretName string, annotations map[string]string) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: networking.k8s.io/v1\n")
	sb.WriteString("kind: Ingress\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  name: " + name + "\n")
	sb.WriteString("  annotations:\n")
	for k, v := range annotations {
		sb.WriteString(fmt.Sprintf("    %s: %q\n", k, v))
	}
	sb.WriteString("spec:\n")
	sb.WriteString("  ingressClassName: nginx\n")
	sb.WriteString("  tls:\n")
	sb.WriteString("  - hosts:\n")
	sb.WriteString("    - " + host + "\n")
	sb.WriteString("    secretName: " + tlsSecretName + "\n")
	sb.WriteString("  rules:\n")
	sb.WriteString("  - host: " + host + "\n")
	sb.WriteString("    http:\n")
	sb.WriteString("      paths:\n")
	sb.WriteString("      - path: /\n")
	sb.WriteString("        pathType: Prefix\n")
	sb.WriteString("        backend:\n")
	sb.WriteString("          service:\n")
	sb.WriteString("            name: snippet-test-backend\n")
	sb.WriteString("            port:\n")
	sb.WriteString("              number: 80\n")
	return sb.String()
}

// makeTLSRequest makes an HTTPS request to the given cluster endpoint with optional client certificate.
// It returns the parsed Response or nil on failure.
func makeTLSRequest(t *testing.T, hostPort, host string, clientCert *tls.Certificate, maxRetries int, delay time.Duration) *Response {
	t.Helper()

	url := fmt.Sprintf("https://%s/", hostPort)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}
	if clientCert != nil {
		tlsConfig.Certificates = []tls.Certificate{*clientCert}
	}

	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	var lastErr error
	for range maxRetries {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		return &Response{
			StatusCode:      resp.StatusCode,
			Body:            string(body),
			ResponseHeaders: resp.Header,
			RequestHeaders:  parseWhoamiHeaders(string(body)),
		}
	}

	t.Logf("TLS request to %s (host=%s) failed after %d retries: %v", hostPort, host, maxRetries, lastErr)
	return nil
}

// --- Certificate generation helpers ---

// generateAuthTLSCerts generates a CA, a valid client certificate signed by that CA,
// and an invalid client certificate signed by a different CA.
func generateAuthTLSCerts() (authTLSCerts, error) {
	// Generate the CA key and self-signed certificate.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test mTLS CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("creating CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("parsing CA certificate: %w", err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	// Generate a valid client certificate signed by the CA.
	clientCert, err := generateClientCert(caCert, caKey, "Test Client")
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating client certificate: %w", err)
	}

	// Generate an invalid client certificate signed by a different (rogue) CA.
	rogueCAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating rogue CA key: %w", err)
	}

	rogueCATemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(100),
		Subject:               pkix.Name{CommonName: "Rogue CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	rogueCACertDER, err := x509.CreateCertificate(rand.Reader, rogueCATemplate, rogueCATemplate, &rogueCAKey.PublicKey, rogueCAKey)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("creating rogue CA certificate: %w", err)
	}

	rogueCACert, err := x509.ParseCertificate(rogueCACertDER)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("parsing rogue CA certificate: %w", err)
	}

	invalidClientCert, err := generateClientCert(rogueCACert, rogueCAKey, "Invalid Client")
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating invalid client certificate: %w", err)
	}

	return authTLSCerts{
		caCertPEM:         caCertPEM,
		caKey:             caKey,
		caCert:            caCert,
		clientCert:        clientCert,
		invalidClientCert: invalidClientCert,
	}, nil
}

// generateClientCert creates a client TLS certificate signed by the given CA.
func generateClientCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string) (tls.Certificate, error) {
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating client key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating serial number: %w", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("creating client certificate: %w", err)
	}

	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshaling client key: %w", err)
	}
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	return tls.X509KeyPair(clientCertPEM, clientKeyPEM)
}

// generateServerCert creates a server TLS certificate for ingress TLS termination.
// It returns PEM-encoded certificate and key bytes.
func generateServerCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, hosts ...string) (certPEM, keyPEM []byte, err error) {
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating server key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generating serial number: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     hosts,
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("creating server certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling server key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	return certPEM, keyPEM, nil
}

// --- Tests ---

func (s *AuthTLSSuite) TestClientCertRequired() {
	// Without client cert: should be rejected.
	traefikResp := makeTLSRequest(s.T(), s.traefikHTTPS, authTLSRequiredTraefikHost, nil, 5, 1*time.Second)
	nginxResp := makeTLSRequest(s.T(), s.nginxHTTPS, authTLSRequiredNginxHost, nil, 5, 1*time.Second)

	// When verify-client is "on" and no client cert is presented, the TLS handshake
	// may fail entirely (nil response) or the server returns 400/403.
	if traefikResp != nil && nginxResp != nil {
		assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	}
	if traefikResp != nil {
		assert.True(s.T(), traefikResp.StatusCode == http.StatusBadRequest || traefikResp.StatusCode == http.StatusForbidden,
			"traefik: expected 400 or 403 without client cert, got %d", traefikResp.StatusCode)
	}
	if nginxResp != nil {
		assert.True(s.T(), nginxResp.StatusCode == http.StatusBadRequest || nginxResp.StatusCode == http.StatusForbidden,
			"nginx: expected 400 or 403 without client cert, got %d", nginxResp.StatusCode)
	}

	// With valid client cert: should succeed.
	traefikResp = makeTLSRequest(s.T(), s.traefikHTTPS, authTLSRequiredTraefikHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil with valid client cert")

	nginxResp = makeTLSRequest(s.T(), s.nginxHTTPS, authTLSRequiredNginxHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil with valid client cert")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with valid client cert")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with valid client cert")
}

func (s *AuthTLSSuite) TestClientCertOptional() {
	// Without client cert: should succeed (optional does not require a cert).
	traefikResp := makeTLSRequest(s.T(), s.traefikHTTPS, authTLSOptionalTraefikHost, nil, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil without client cert (optional)")

	nginxResp := makeTLSRequest(s.T(), s.nginxHTTPS, authTLSOptionalNginxHost, nil, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil without client cert (optional)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch without cert (optional)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 without client cert when verify-client is optional")

	// With valid client cert: should also succeed.
	traefikResp = makeTLSRequest(s.T(), s.traefikHTTPS, authTLSOptionalTraefikHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil with valid client cert (optional)")

	nginxResp = makeTLSRequest(s.T(), s.nginxHTTPS, authTLSOptionalNginxHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil with valid client cert (optional)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with valid cert (optional)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with valid client cert when verify-client is optional")
}

func (s *AuthTLSSuite) TestClientCertOff() {
	// Without client cert: should succeed.
	traefikResp := makeTLSRequest(s.T(), s.traefikHTTPS, authTLSOffTraefikHost, nil, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil without client cert (off)")

	nginxResp := makeTLSRequest(s.T(), s.nginxHTTPS, authTLSOffNginxHost, nil, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil without client cert (off)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch without cert (off)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 without client cert when verify-client is off")

	// With client cert: should also succeed.
	traefikResp = makeTLSRequest(s.T(), s.traefikHTTPS, authTLSOffTraefikHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil with client cert (off)")

	nginxResp = makeTLSRequest(s.T(), s.nginxHTTPS, authTLSOffNginxHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil with client cert (off)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with cert (off)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with client cert when verify-client is off")
}

func (s *AuthTLSSuite) TestInvalidClientCert() {
	// With a client cert signed by a different CA: should be rejected when verify-client is "on".
	traefikResp := makeTLSRequest(s.T(), s.traefikHTTPS, authTLSRequiredTraefikHost, &s.certs.invalidClientCert, 5, 1*time.Second)
	nginxResp := makeTLSRequest(s.T(), s.nginxHTTPS, authTLSRequiredNginxHost, &s.certs.invalidClientCert, 5, 1*time.Second)

	// The TLS handshake may fail entirely (nil response) or the server returns 400/403.
	if traefikResp != nil && nginxResp != nil {
		assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with invalid client cert")
	}
	if traefikResp != nil {
		assert.True(s.T(), traefikResp.StatusCode == http.StatusBadRequest || traefikResp.StatusCode == http.StatusForbidden,
			"traefik: expected 400 or 403 with invalid client cert, got %d", traefikResp.StatusCode)
	}
	if nginxResp != nil {
		assert.True(s.T(), nginxResp.StatusCode == http.StatusBadRequest || nginxResp.StatusCode == http.StatusForbidden,
			"nginx: expected 400 or 403 with invalid client cert, got %d", nginxResp.StatusCode)
	}
}

func (s *AuthTLSSuite) TestPassCertToUpstream() {
	// With pass-certificate-to-upstream enabled, the upstream (whoami) should receive
	// a header containing the client certificate information.
	traefikResp := makeTLSRequest(s.T(), s.traefikHTTPS, authTLSPassCertTraefikHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := makeTLSRequest(s.T(), s.nginxHTTPS, authTLSPassCertNginxHost, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: expected 200 with valid client cert and pass-cert")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: expected 200 with valid client cert and pass-cert")

	// Both nginx and traefik should pass client certificate info to the upstream.
	// nginx uses Ssl-Client-Cert, traefik uses X-Forwarded-Tls-Client-Cert.
	// Check that at least one of the common certificate-related headers is present.
	certHeaders := []string{
		"X-Forwarded-Tls-Client-Cert",
		"Ssl-Client-Cert",
		"X-Ssl-Client-Cert",
		"X-Client-Cert",
	}

	traefikHasCert := false
	for _, h := range certHeaders {
		if v, ok := traefikResp.RequestHeaders[h]; ok && v != "" {
			traefikHasCert = true
			break
		}
	}
	assert.True(s.T(), traefikHasCert,
		"traefik: expected client certificate header in upstream request, body: %s", traefikResp.Body)

	nginxHasCert := false
	for _, h := range certHeaders {
		if v, ok := nginxResp.RequestHeaders[h]; ok && v != "" {
			nginxHasCert = true
			break
		}
	}
	assert.True(s.T(), nginxHasCert,
		"nginx: expected client certificate header in upstream request, body: %s", nginxResp.Body)
}
