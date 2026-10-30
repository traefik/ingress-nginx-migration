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

	// Pass the certificate to the upstream, with the HTTPS redirection disabled, so that the
	// plaintext route serves the backend instead of redirecting to it.
	authTLSPassCertNoRedirectIngressName = "mtls-pass-cert-no-redirect-test"
	authTLSPassCertNoRedirectTraefikHost = authTLSPassCertNoRedirectIngressName + ".traefik.local"
	authTLSPassCertNoRedirectNginxHost   = authTLSPassCertNoRedirectIngressName + ".nginx.local"

	// Pass the certificate to the upstream, with an optional client certificate, so that a TLS
	// request carrying none of it still reaches the backend.
	authTLSPassCertOptionalIngressName = "mtls-pass-cert-optional-test"
	authTLSPassCertOptionalTraefikHost = authTLSPassCertOptionalIngressName + ".traefik.local"
	authTLSPassCertOptionalNginxHost   = authTLSPassCertOptionalIngressName + ".nginx.local"

	// Pass the certificate to the upstream, with a client certificate requested but not verified during the handshake,
	// so that the backend learns from Ssl-Client-Verify whether the CA trusts it.
	authTLSNoCAPassCertIngressName = "mtls-no-ca-pass-cert-test"
	authTLSNoCAPassCertTraefikHost = authTLSNoCAPassCertIngressName + ".traefik.local"
	authTLSNoCAPassCertNginxHost   = authTLSNoCAPassCertIngressName + ".nginx.local"

	authTLSCACertSecretName    = "mtls-ca-cert"
	authTLSServerTLSSecretName = "mtls-server-tls"
)

// authTLSCerts holds all generated certificates for the auth-tls test suite.
type authTLSCerts struct {
	caCertPEM  []byte
	caKey      *ecdsa.PrivateKey
	caCert     *x509.Certificate
	clientCert tls.Certificate
	// chainedClientCert is signed by an intermediate CA of the CA, and presented along with it.
	chainedClientCert tls.Certificate
	// invalidClientCert is signed by a different CA.
	invalidClientCert tls.Certificate
}

type AuthTLSSuite struct {
	BaseSuite

	certs authTLSCerts
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
		authTLSPassCertNoRedirectTraefikHost, authTLSPassCertNoRedirectNginxHost,
		authTLSPassCertOptionalTraefikHost, authTLSPassCertOptionalNginxHost,
		authTLSNoCAPassCertTraefikHost, authTLSNoCAPassCertNginxHost,
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

	// 5. auth-tls-pass-certificate-to-upstream="true" with ssl-redirect="false",
	// where the plaintext route reaches the backend.
	s.deployAuthTLSIngress(authTLSPassCertNoRedirectIngressName, authTLSPassCertNoRedirectTraefikHost, authTLSPassCertNoRedirectNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "on",
		"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
		"nginx.ingress.kubernetes.io/ssl-redirect":                          "false",
	})

	// 6. auth-tls-pass-certificate-to-upstream="true" with verify-client="optional".
	s.deployAuthTLSIngress(authTLSPassCertOptionalIngressName, authTLSPassCertOptionalTraefikHost, authTLSPassCertOptionalNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "optional",
		"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
	})

	// 7. auth-tls-pass-certificate-to-upstream="true" with verify-client="optional_no_ca".
	s.deployAuthTLSIngress(authTLSNoCAPassCertIngressName, authTLSNoCAPassCertTraefikHost, authTLSNoCAPassCertNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSCACertSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "optional_no_ca",
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
	s.traefik.WaitForIngressReady(s.T(), authTLSPassCertNoRedirectTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSPassCertNoRedirectNginxHost, 30, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authTLSPassCertOptionalTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSPassCertOptionalNginxHost, 30, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authTLSNoCAPassCertTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authTLSNoCAPassCertNginxHost, 30, 1*time.Second)
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
	_ = s.traefik.DeleteIngress(authTLSPassCertNoRedirectIngressName)
	_ = s.nginx.DeleteIngress(authTLSPassCertNoRedirectIngressName)
	_ = s.traefik.DeleteIngress(authTLSPassCertOptionalIngressName)
	_ = s.nginx.DeleteIngress(authTLSPassCertOptionalIngressName)
	_ = s.traefik.DeleteIngress(authTLSNoCAPassCertIngressName)
	_ = s.nginx.DeleteIngress(authTLSNoCAPassCertIngressName)
	_ = s.traefik.DeleteSecret(authTLSCACertSecretName)
	_ = s.nginx.DeleteSecret(authTLSCACertSecretName)
	_ = s.traefik.DeleteSecret(authTLSServerTLSSecretName)
	_ = s.nginx.DeleteSecret(authTLSServerTLSSecretName)
}

// deployAuthTLSIngress deploys an ingress with TLS termination and the given annotations to both clusters.
func (s *AuthTLSSuite) deployAuthTLSIngress(name, traefikHost, nginxHost string, annotations map[string]string) {
	s.T().Helper()

	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        traefikHost,
		Annotations: annotations,
		TLSSecret:   authTLSServerTLSSecretName,
	})
	require.NoError(s.T(), err, "deploy %s ingress to traefik cluster", name)

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        nginxHost,
		Annotations: annotations,
		TLSSecret:   authTLSServerTLSSecretName,
	})
	require.NoError(s.T(), err, "deploy %s ingress to nginx cluster", name)
}

// --- Certificate generation helpers ---

// generateAuthTLSCerts generates a CA, a valid client certificate signed by that CA,
// and an invalid client certificate signed by a different CA.
func generateAuthTLSCerts() (authTLSCerts, error) {
	return generateAuthTLSCertsWithCAName("Test mTLS CA")
}

// generateAuthTLSCertsWithCAName is generateAuthTLSCerts with a CA named caName.
func generateAuthTLSCertsWithCAName(caName string) (authTLSCerts, error) {
	// Generate the CA key and self-signed certificate.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: caName},
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

	// Generate a client certificate signed by an intermediate CA, presented along with the intermediate.
	intermediateCert, intermediateKey, err := generateIntermediateCA(caCert, caKey, "Test mTLS Intermediate CA")
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating intermediate CA: %w", err)
	}

	chainedClientCert, err := generateClientCert(intermediateCert, intermediateKey, "Chained Client")
	if err != nil {
		return authTLSCerts{}, fmt.Errorf("generating chained client certificate: %w", err)
	}
	chainedClientCert.Certificate = append(chainedClientCert.Certificate, intermediateCert.Raw)

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
		chainedClientCert: chainedClientCert,
		invalidClientCert: invalidClientCert,
	}, nil
}

// generateIntermediateCA creates an intermediate CA certificate signed by the given CA.
func generateIntermediateCA(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generating intermediate CA key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("creating intermediate CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing intermediate CA certificate: %w", err)
	}

	return cert, key, nil
}

// generateSelfSignedClientCert creates a client certificate signed by its own key.
func generateSelfSignedClientCert(commonName string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generating self-signed client key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(200),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("creating self-signed client certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshaling self-signed client key: %w", err)
	}

	return tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
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
	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSRequiredTraefikHost, http.MethodGet, "/", nil, nil, 5, 1*time.Second)
	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSRequiredNginxHost, http.MethodGet, "/", nil, nil, 5, 1*time.Second)

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
	traefikResp = s.traefik.MakeTLSRequest(s.T(), authTLSRequiredTraefikHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil with valid client cert")

	nginxResp = s.nginx.MakeTLSRequest(s.T(), authTLSRequiredNginxHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil with valid client cert")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with valid client cert")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with valid client cert")
}

func (s *AuthTLSSuite) TestClientCertOptional() {
	// Without client cert: should succeed (optional does not require a cert).
	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSOptionalTraefikHost, http.MethodGet, "/", nil, nil, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil without client cert (optional)")

	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSOptionalNginxHost, http.MethodGet, "/", nil, nil, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil without client cert (optional)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch without cert (optional)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 without client cert when verify-client is optional")

	// With valid client cert: should also succeed.
	traefikResp = s.traefik.MakeTLSRequest(s.T(), authTLSOptionalTraefikHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil with valid client cert (optional)")

	nginxResp = s.nginx.MakeTLSRequest(s.T(), authTLSOptionalNginxHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil with valid client cert (optional)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with valid cert (optional)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with valid client cert when verify-client is optional")
}

func (s *AuthTLSSuite) TestClientCertOff() {
	// Without client cert: should succeed.
	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSOffTraefikHost, http.MethodGet, "/", nil, nil, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil without client cert (off)")

	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSOffNginxHost, http.MethodGet, "/", nil, nil, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil without client cert (off)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch without cert (off)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 without client cert when verify-client is off")

	// With client cert: should also succeed.
	traefikResp = s.traefik.MakeTLSRequest(s.T(), authTLSOffTraefikHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil with client cert (off)")

	nginxResp = s.nginx.MakeTLSRequest(s.T(), authTLSOffNginxHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil with client cert (off)")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with cert (off)")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with client cert when verify-client is off")
}

func (s *AuthTLSSuite) TestInvalidClientCert() {
	// With a client cert signed by a different CA: should be rejected when verify-client is "on".
	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSRequiredTraefikHost, http.MethodGet, "/", nil, &s.certs.invalidClientCert, 5, 1*time.Second)
	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSRequiredNginxHost, http.MethodGet, "/", nil, &s.certs.invalidClientCert, 5, 1*time.Second)

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
	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSPassCertTraefikHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSPassCertNginxHost, http.MethodGet, "/", nil, &s.certs.clientCert, 5, 1*time.Second)
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

// sslClientHeaders holds the request header names the pass-certificate-to-upstream feature owns.
// Nginx builds them from the $ssl_client_* variables, and Traefik reproduces them.
var sslClientHeaders = []string{
	"Ssl-Client-Verify",
	"Ssl-Client-Cert",
	"Ssl-Client-Subject-Dn",
	"Ssl-Client-Issuer-Dn",
}

// forgedIdentity is the client certificate identity a request claims when it sends the
// Ssl-Client-* headers itself.
const forgedIdentity = "CN=forged-admin,O=forged"

// forgedSSLClientHeaders returns the four headers spelled canonically, claiming a verified identity.
func forgedSSLClientHeaders() map[string]string {
	return map[string]string{
		"Ssl-Client-Verify":     "SUCCESS",
		"Ssl-Client-Subject-Dn": forgedIdentity,
		"Ssl-Client-Issuer-Dn":  "CN=forged-ca",
		"Ssl-Client-Cert":       "FORGED-CERTIFICATE",
	}
}

// forgedAliasingSSLClientHeaders returns the same claim, spelled with underscores and dots instead
// of dashes. A backend deriving a variable name from the header name reads those spellings as the
// canonical names, so they carry the same claim.
func forgedAliasingSSLClientHeaders() map[string]string {
	return map[string]string{
		"Ssl_Client_Verify":     "SUCCESS",
		"Ssl_Client_Subject_Dn": forgedIdentity,
		"Ssl.Client.Issuer.Dn":  "CN=forged-ca",
		"Ssl.Client.Cert":       "FORGED-CERTIFICATE",
	}
}

// normalizeSSLHeaderName upper-cases the letters of name and replaces every other byte with a dash,
// which is the form a backend deriving variable names from the header names collapses a name into.
// It is written here rather than taken from the proxy, so that these assertions do not rely on the
// implementation they check.
func normalizeSSLHeaderName(name string) string {
	var b strings.Builder
	for i := range len(name) {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - ('a' - 'A'))
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		default:
			b.WriteByte('-')
		}
	}

	return b.String()
}

// sslClientHeadersReaching returns the upstream header names that a backend reads as one of the
// Ssl-Client-* fields, whichever spelling they arrived with, with their values.
func sslClientHeadersReaching(upstream map[string]string) map[string]string {
	managed := make(map[string]struct{}, len(sslClientHeaders))
	for _, name := range sslClientHeaders {
		managed[normalizeSSLHeaderName(name)] = struct{}{}
	}

	reaching := make(map[string]string)
	for name, value := range upstream {
		if _, ok := managed[normalizeSSLHeaderName(name)]; ok {
			reaching[name] = value
		}
	}

	return reaching
}

// clientSentVerificationHeaders returns Ssl-Client-* headers sent by the client, in both their canonical and their aliasing
// spellings, except the canonical Ssl-Client-Cert: nginx only sets that one when it passes the certificate to the upstream,
// see TestKnownDifference_CertHeaderWithoutPassCertDiffers.
func clientSentVerificationHeaders() map[string]string {
	headers := forgedSSLClientHeaders()
	for name, value := range forgedAliasingSSLClientHeaders() {
		headers[name] = value
	}
	delete(headers, "Ssl-Client-Cert")

	return headers
}

// requestSSLClientHeaders sends the same HTTPS request to both controllers, requires both of them to serve it,
// and returns the Ssl-Client-* headers each backend received.
func requestSSLClientHeaders(t *testing.T, traefik, nginx *Cluster, traefikHost, nginxHost, path string, headers map[string]string, clientCert *tls.Certificate) (traefikHeaders, nginxHeaders map[string]string) {
	t.Helper()

	traefikResp := traefik.MakeTLSRequest(t, traefikHost, http.MethodGet, path, headers, clientCert, 5, 1*time.Second)
	require.NotNil(t, traefikResp, "traefik: %s", path)
	nginxResp := nginx.MakeTLSRequest(t, nginxHost, http.MethodGet, path, headers, clientCert, 5, 1*time.Second)
	require.NotNil(t, nginxResp, "nginx: %s", path)

	require.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik: %s: body: %s", path, traefikResp.Body)
	require.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx: %s: body: %s", path, nginxResp.Body)

	return sslClientHeadersReaching(traefikResp.RequestHeaders), sslClientHeadersReaching(nginxResp.RequestHeaders)
}

// verifiedSSLClientHeaders returns the headers a backend receives for a verified client certificate that is not passed to it.
func verifiedSSLClientHeaders(cert tls.Certificate) map[string]string {
	return map[string]string{
		"Ssl-Client-Verify":     "SUCCESS",
		"Ssl-Client-Subject-Dn": cert.Leaf.Subject.String(),
		"Ssl-Client-Issuer-Dn":  cert.Leaf.Issuer.String(),
	}
}

// TestPassCertToUpstreamPlaintextSetsNoCertHeaders checks the plaintext route of an Ingress that
// disables the HTTPS redirection. Nginx builds these headers from variables that are empty outside
// of a TLS connection, and omits a header whose value is empty, so the backend receives none of
// them: not even Ssl-Client-Verify, which would otherwise report that the client presented no
// certificate over a connection that was never TLS.
func (s *AuthTLSSuite) TestPassCertToUpstreamPlaintextSetsNoCertHeaders() {
	traefikResp := s.traefik.MakeRequest(s.T(), authTLSPassCertNoRedirectTraefikHost, http.MethodGet, "/", nil, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), authTLSPassCertNoRedirectNginxHost, http.MethodGet, "/", nil, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	require.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: body: %s", traefikResp.Body)
	require.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: body: %s", nginxResp.Body)

	assert.Empty(s.T(), sslClientHeadersReaching(nginxResp.RequestHeaders), "nginx")
	assert.Empty(s.T(), sslClientHeadersReaching(traefikResp.RequestHeaders), "traefik")
}

// TestPassCertToUpstreamPlaintextDropsForgedCertHeaders checks that a client cannot supply the
// Ssl-Client-* headers itself on that plaintext route, which would have the backend authorize a
// certificate identity that was never presented.
func (s *AuthTLSSuite) TestPassCertToUpstreamPlaintextDropsForgedCertHeaders() {
	forged := forgedSSLClientHeaders()

	traefikResp := s.traefik.MakeRequest(s.T(), authTLSPassCertNoRedirectTraefikHost, http.MethodGet, "/", forged, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), authTLSPassCertNoRedirectNginxHost, http.MethodGet, "/", forged, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	require.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: body: %s", traefikResp.Body)
	require.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: body: %s", nginxResp.Body)

	assert.Empty(s.T(), sslClientHeadersReaching(nginxResp.RequestHeaders), "nginx")
	assert.Empty(s.T(), sslClientHeadersReaching(traefikResp.RequestHeaders), "traefik")
	assert.NotContains(s.T(), traefikResp.Body, forgedIdentity, "traefik: the forged identity reached the backend")
}

// TestPassCertToUpstreamPlaintextDropsForgedAliasingCertHeaders sends the same claim spelled so that
// it aliases the canonical names. Nginx drops a request header whose name contains an underscore or
// a dot before it reaches a backend, so the two proxies end up agreeing here through different
// means, and the comparison is on what the backend receives rather than on how it was achieved.
func (s *AuthTLSSuite) TestPassCertToUpstreamPlaintextDropsForgedAliasingCertHeaders() {
	forged := forgedAliasingSSLClientHeaders()
	// A name that merely carries an aliasing character is not one of ours: Traefik forwards it,
	// which also shows that these spellings do travel to the proxy rather than being dropped by the
	// client, and that the assertions above are not vacuous.
	forged["X_Not_Managed"] = "kept"

	traefikResp := s.traefik.MakeRequest(s.T(), authTLSPassCertNoRedirectTraefikHost, http.MethodGet, "/", forged, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), authTLSPassCertNoRedirectNginxHost, http.MethodGet, "/", forged, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	require.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: body: %s", traefikResp.Body)
	require.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: body: %s", nginxResp.Body)

	assert.Empty(s.T(), sslClientHeadersReaching(nginxResp.RequestHeaders), "nginx")
	assert.Empty(s.T(), sslClientHeadersReaching(traefikResp.RequestHeaders), "traefik")
	assert.NotContains(s.T(), traefikResp.Body, forgedIdentity, "traefik: the forged identity reached the backend")
	assert.Contains(s.T(), traefikResp.Body, "kept", "traefik: an unmanaged aliasing name should be forwarded")
}

// TestPassCertToUpstreamOverTLSDropsForgedCertHeaders checks the HTTPS route, where the proxy sets
// these headers from the certificate the client presented: a spelling the client supplied must not
// survive beside the value the proxy wrote.
func (s *AuthTLSSuite) TestPassCertToUpstreamOverTLSDropsForgedCertHeaders() {
	forged := forgedSSLClientHeaders()
	for name, value := range forgedAliasingSSLClientHeaders() {
		forged[name] = value
	}

	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSPassCertTraefikHost, http.MethodGet, "/", forged, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSPassCertNginxHost, http.MethodGet, "/", forged, &s.certs.clientCert, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	require.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: body: %s", traefikResp.Body)
	require.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: body: %s", nginxResp.Body)

	// Nginx and Traefik do not serialize a distinguished name identically, so the assertion is on the
	// common name of the certificate that was actually presented.
	require.NotNil(s.T(), s.certs.clientCert.Leaf)
	expectedCN := "CN=" + s.certs.clientCert.Leaf.Subject.CommonName

	for _, resp := range map[string]*Response{"traefik": traefikResp, "nginx": nginxResp} {
		reaching := sslClientHeadersReaching(resp.RequestHeaders)
		assert.Equal(s.T(), "SUCCESS", reaching["Ssl-Client-Verify"])
		assert.Contains(s.T(), reaching["Ssl-Client-Subject-Dn"], expectedCN)

		// Only the four canonical names, each holding the proxy's own value.
		assert.Len(s.T(), reaching, len(sslClientHeaders))
		for name, value := range reaching {
			assert.NotEqual(s.T(), forgedIdentity, value, "%s carries the forged identity", name)
			assert.NotEqual(s.T(), "FORGED-CERTIFICATE", value, "%s carries the forged certificate", name)
		}
	}
}

// TestPassCertToUpstreamTLSWithoutClientCertSetsNone checks a TLS request presenting no certificate,
// which an optional verify-client lets through. Nginx reports NONE there, unlike on a plaintext
// request where it sends nothing at all.
func (s *AuthTLSSuite) TestPassCertToUpstreamTLSWithoutClientCertSetsNone() {
	forged := forgedSSLClientHeaders()

	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSPassCertOptionalTraefikHost, http.MethodGet, "/", forged, nil, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSPassCertOptionalNginxHost, http.MethodGet, "/", forged, nil, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	require.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: body: %s", traefikResp.Body)
	require.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: body: %s", nginxResp.Body)

	for _, resp := range map[string]*Response{"traefik": traefikResp, "nginx": nginxResp} {
		reaching := sslClientHeadersReaching(resp.RequestHeaders)
		assert.Equal(s.T(), map[string]string{"Ssl-Client-Verify": "NONE"}, reaching)
	}
}

// TestWithoutPassCertSetsCertHeaders checks an Ingress requiring a client certificate without passing it to the
// upstream. Nginx sets Ssl-Client-Verify, Ssl-Client-Subject-Dn and Ssl-Client-Issuer-Dn on every location of a server
// configured for client certificate authentication, overwriting the values the client sent, and leaves out Ssl-Client-Cert,
// which auth-tls-pass-certificate-to-upstream controls.
func (s *AuthTLSSuite) TestWithoutPassCertSetsCertHeaders() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSRequiredTraefikHost, authTLSRequiredNginxHost, "/",
		clientSentVerificationHeaders(), &s.certs.clientCert)

	assert.Equal(s.T(), verifiedSSLClientHeaders(s.certs.clientCert), nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")
}

// TestKnownDifference_CertHeaderWithoutPassCertDiffers documents a known difference on an Ingress requiring a client
// certificate without passing it to the upstream. Nginx only sets Ssl-Client-Cert when it passes the certificate, so it
// forwards a value the client sent for it. Traefik owns the header whenever client certificate authentication is configured,
// and drops that value.
func (s *AuthTLSSuite) TestKnownDifference_CertHeaderWithoutPassCertDiffers() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSRequiredTraefikHost, authTLSRequiredNginxHost, "/",
		forgedSSLClientHeaders(), &s.certs.clientCert)

	assert.Equal(s.T(), "FORGED-CERTIFICATE", nginxHeaders["Ssl-Client-Cert"], "nginx should forward the Ssl-Client-Cert header the client sent")
	assert.NotContains(s.T(), traefikHeaders, "Ssl-Client-Cert", "traefik should drop the Ssl-Client-Cert header the client sent")
}

// TestClientCertOffSetsNone checks an Ingress declaring a CA with verify-client "off": no client certificate is requested,
// even from a client having one to present, and the backend is told so.
func (s *AuthTLSSuite) TestClientCertOffSetsNone() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSOffTraefikHost, authTLSOffNginxHost, "/",
		clientSentVerificationHeaders(), &s.certs.clientCert)

	assert.Equal(s.T(), map[string]string{"Ssl-Client-Verify": "NONE"}, nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")
}

// TestOptionalNoCAWithoutClientCertSetsNone checks a TLS request presenting no certificate to an Ingress with
// verify-client "optional_no_ca".
func (s *AuthTLSSuite) TestOptionalNoCAWithoutClientCertSetsNone() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSNoCAPassCertTraefikHost, authTLSNoCAPassCertNginxHost, "/",
		forgedSSLClientHeaders(), nil)

	assert.Equal(s.T(), map[string]string{"Ssl-Client-Verify": "NONE"}, nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")
}

// TestOptionalNoCAUntrustedClientCertSetsFailed checks that verify-client "optional_no_ca" lets a certificate the CA did
// not sign through the handshake, and that the backend is told that it failed verification.
// Nginx and Traefik word the failure reason differently (OpenSSL and Go errors), so only the outcome is compared.
func (s *AuthTLSSuite) TestOptionalNoCAUntrustedClientCertSetsFailed() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSNoCAPassCertTraefikHost, authTLSNoCAPassCertNginxHost, "/",
		nil, &s.certs.invalidClientCert)

	for name, headers := range map[string]map[string]string{"traefik": traefikHeaders, "nginx": nginxHeaders} {
		assert.True(s.T(), strings.HasPrefix(headers["Ssl-Client-Verify"], "FAILED:"), "%s: Ssl-Client-Verify is %q", name, headers["Ssl-Client-Verify"])
		assert.Equal(s.T(), s.certs.invalidClientCert.Leaf.Subject.String(), headers["Ssl-Client-Subject-Dn"], name)
		assert.NotEmpty(s.T(), headers["Ssl-Client-Cert"], name)
	}
}

// TestOptionalNoCATrustedClientCertSetsSuccess checks that a certificate signed by the CA is reported as verified with
// verify-client "optional_no_ca". Like most client certificates, it is valid for client authentication only (extended key
// usage), which the verification must accept as nginx does.
func (s *AuthTLSSuite) TestOptionalNoCATrustedClientCertSetsSuccess() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSNoCAPassCertTraefikHost, authTLSNoCAPassCertNginxHost, "/",
		nil, &s.certs.clientCert)

	for name, headers := range map[string]map[string]string{"traefik": traefikHeaders, "nginx": nginxHeaders} {
		assert.Equal(s.T(), "SUCCESS", headers["Ssl-Client-Verify"], name)
		assert.Equal(s.T(), s.certs.clientCert.Leaf.Subject.String(), headers["Ssl-Client-Subject-Dn"], name)
		assert.NotEmpty(s.T(), headers["Ssl-Client-Cert"], name)
	}
}

// TestOptionalNoCAClientCertThroughIntermediateSetsSuccess checks that a certificate signed by an intermediate CA, which
// the client presents along with it, is reported as verified with verify-client "optional_no_ca".
func (s *AuthTLSSuite) TestOptionalNoCAClientCertThroughIntermediateSetsSuccess() {
	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx, authTLSNoCAPassCertTraefikHost, authTLSNoCAPassCertNginxHost, "/",
		nil, &s.certs.chainedClientCert)

	for name, headers := range map[string]map[string]string{"traefik": traefikHeaders, "nginx": nginxHeaders} {
		assert.Equal(s.T(), "SUCCESS", headers["Ssl-Client-Verify"], name)
		assert.Equal(s.T(), s.certs.chainedClientCert.Leaf.Issuer.String(), headers["Ssl-Client-Issuer-Dn"], name)
	}
}
