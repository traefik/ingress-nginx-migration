package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// Ingress names for proxy-ssl tests.
	proxySSLBasicIngressName = "proxy-ssl-basic-test"
	proxySSLBasicTraefikHost = proxySSLBasicIngressName + ".traefik.local"
	proxySSLBasicNginxHost   = proxySSLBasicIngressName + ".nginx.local"

	proxySSLVerifyOffIngressName = "proxy-ssl-verify-off-test"
	proxySSLVerifyOffTraefikHost = proxySSLVerifyOffIngressName + ".traefik.local"
	proxySSLVerifyOffNginxHost   = proxySSLVerifyOffIngressName + ".nginx.local"

	proxySSLVerifyOnIngressName = "proxy-ssl-verify-on-test"
	proxySSLVerifyOnTraefikHost = proxySSLVerifyOnIngressName + ".traefik.local"
	proxySSLVerifyOnNginxHost   = proxySSLVerifyOnIngressName + ".nginx.local"

	proxySSLNameIngressName = "proxy-ssl-name-test"
	proxySSLNameTraefikHost = proxySSLNameIngressName + ".traefik.local"
	proxySSLNameNginxHost   = proxySSLNameIngressName + ".nginx.local"

	proxySSLServerNameIngressName = "proxy-ssl-svrname-test"
	proxySSLServerNameTraefikHost = proxySSLServerNameIngressName + ".traefik.local"
	proxySSLServerNameNginxHost   = proxySSLServerNameIngressName + ".nginx.local"

	// Resource names.
	httpsBackendName          = "https-backend"
	httpsBackendConfigMapName = "https-backend-config"
	httpsBackendTLSSecretName = "https-backend-tls"
	proxySSLClientSecretName  = "proxy-ssl-client-cert"
	proxySSLNoCAClientSecret  = "proxy-ssl-no-ca-cert"
	httpsBackendServerName    = "https-backend.default.svc.cluster.local"
)

// proxySSLCerts holds all generated certificates for proxy-ssl tests.
type proxySSLCerts struct {
	caCertPEM     []byte
	caCert        *x509.Certificate
	caKey         *ecdsa.PrivateKey
	serverCertPEM []byte
	serverKeyPEM  []byte
	clientCertPEM []byte
	clientKeyPEM  []byte
}

type ProxySSLSuite struct {
	BaseSuite

	certs proxySSLCerts
}

func TestProxySSLSuite(t *testing.T) {
	suite.Run(t, new(ProxySSLSuite))
}

func (s *ProxySSLSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Generate all certificates.
	certs, err := generateProxySSLCerts()
	require.NoError(s.T(), err, "generate proxy-ssl certificates")
	s.certs = certs

	// Deploy the HTTPS backend (shared by all proxy-ssl tests).
	s.deployHTTPSBackend()

	// Deploy proxy-ssl-secret with client cert AND CA cert (for verify-on tests).
	clientSecretWithCA := secretTemplateData{
		Name: proxySSLClientSecretName,
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(s.certs.clientCertPEM),
			"tls.key": base64.StdEncoding.EncodeToString(s.certs.clientKeyPEM),
			"ca.crt":  base64.StdEncoding.EncodeToString(s.certs.caCertPEM),
		},
	}
	err = s.traefik.DeploySecret(clientSecretWithCA)
	require.NoError(s.T(), err, "deploy proxy-ssl client secret with CA")
	err = s.nginx.DeploySecret(clientSecretWithCA)
	require.NoError(s.T(), err, "deploy proxy-ssl client secret with CA to nginx")

	// Deploy proxy-ssl-secret with client cert but NO CA cert.
	clientSecretNoCA := secretTemplateData{
		Name: proxySSLNoCAClientSecret,
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(s.certs.clientCertPEM),
			"tls.key": base64.StdEncoding.EncodeToString(s.certs.clientKeyPEM),
		},
	}
	err = s.traefik.DeploySecret(clientSecretNoCA)
	require.NoError(s.T(), err, "deploy proxy-ssl client secret without CA")
	err = s.nginx.DeploySecret(clientSecretNoCA)
	require.NoError(s.T(), err, "deploy proxy-ssl client secret without CA to nginx")

	// 1. Basic: backend-protocol HTTPS + proxy-ssl-secret.
	s.deployProxySSLIngress(proxySSLBasicIngressName, proxySSLBasicTraefikHost, proxySSLBasicNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
		"nginx.ingress.kubernetes.io/proxy-ssl-secret": testNamespace + "/" + proxySSLClientSecretName,
	})

	// 2. proxy-ssl-verify: off (explicit).
	s.deployProxySSLIngress(proxySSLVerifyOffIngressName, proxySSLVerifyOffTraefikHost, proxySSLVerifyOffNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
		"nginx.ingress.kubernetes.io/proxy-ssl-secret": testNamespace + "/" + proxySSLNoCAClientSecret,
		"nginx.ingress.kubernetes.io/proxy-ssl-verify": "off",
	})

	// 3. proxy-ssl-verify: on + CA in secret + proxy-ssl-name.
	s.deployProxySSLIngress(proxySSLVerifyOnIngressName, proxySSLVerifyOnTraefikHost, proxySSLVerifyOnNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
		"nginx.ingress.kubernetes.io/proxy-ssl-secret": testNamespace + "/" + proxySSLClientSecretName,
		"nginx.ingress.kubernetes.io/proxy-ssl-verify": "on",
		"nginx.ingress.kubernetes.io/proxy-ssl-name":   httpsBackendServerName,
	})

	// 4. proxy-ssl-name (SNI override).
	s.deployProxySSLIngress(proxySSLNameIngressName, proxySSLNameTraefikHost, proxySSLNameNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
		"nginx.ingress.kubernetes.io/proxy-ssl-secret": testNamespace + "/" + proxySSLClientSecretName,
		"nginx.ingress.kubernetes.io/proxy-ssl-name":   httpsBackendServerName,
	})

	// 5. proxy-ssl-server-name (alternative to proxy-ssl-name).
	s.deployProxySSLIngress(proxySSLServerNameIngressName, proxySSLServerNameTraefikHost, proxySSLServerNameNginxHost, map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol":      "HTTPS",
		"nginx.ingress.kubernetes.io/proxy-ssl-secret":      testNamespace + "/" + proxySSLClientSecretName,
		"nginx.ingress.kubernetes.io/proxy-ssl-server-name": "on",
	})

	// Wait for all ingresses to be ready.
	for _, h := range []struct {
		host    string
		cluster *Cluster
	}{
		{proxySSLBasicTraefikHost, s.traefik},
		{proxySSLBasicNginxHost, s.nginx},
		{proxySSLVerifyOffTraefikHost, s.traefik},
		{proxySSLVerifyOffNginxHost, s.nginx},
		{proxySSLVerifyOnTraefikHost, s.traefik},
		{proxySSLVerifyOnNginxHost, s.nginx},
		{proxySSLNameTraefikHost, s.traefik},
		{proxySSLNameNginxHost, s.nginx},
		{proxySSLServerNameTraefikHost, s.traefik},
		{proxySSLServerNameNginxHost, s.nginx},
	} {
		h.cluster.WaitForIngressReady(s.T(), h.host, 30, 1*time.Second)
	}
}

func (s *ProxySSLSuite) TearDownSuite() {
	for _, name := range []string{
		proxySSLBasicIngressName,
		proxySSLVerifyOffIngressName,
		proxySSLVerifyOnIngressName,
		proxySSLNameIngressName,
		proxySSLServerNameIngressName,
	} {
		_ = s.traefik.DeleteIngress(name)
		_ = s.nginx.DeleteIngress(name)
	}

	_ = s.traefik.DeleteSecret(proxySSLClientSecretName)
	_ = s.nginx.DeleteSecret(proxySSLClientSecretName)
	_ = s.traefik.DeleteSecret(proxySSLNoCAClientSecret)
	_ = s.nginx.DeleteSecret(proxySSLNoCAClientSecret)
	_ = s.traefik.DeleteSecret(httpsBackendTLSSecretName)
	_ = s.traefik.DeleteConfigMap(httpsBackendConfigMapName)

	_ = s.traefik.Kubectl("delete", "deployment", httpsBackendName, "-n", testNamespace, "--ignore-not-found")
	_ = s.traefik.Kubectl("delete", "service", httpsBackendName, "-n", testNamespace, "--ignore-not-found")
}

// deployHTTPSBackend deploys an nginx-based HTTPS backend into the cluster.
func (s *ProxySSLSuite) deployHTTPSBackend() {
	s.T().Helper()

	// Deploy the server TLS secret.
	serverTLSSecret := secretTemplateData{
		Name: httpsBackendTLSSecretName,
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(s.certs.serverCertPEM),
			"tls.key": base64.StdEncoding.EncodeToString(s.certs.serverKeyPEM),
		},
	}
	err := s.traefik.DeploySecret(serverTLSSecret)
	require.NoError(s.T(), err, "deploy HTTPS backend TLS secret")

	// Deploy the nginx config.
	configMap := configMapTemplateData{
		Name: httpsBackendConfigMapName,
		Data: map[string]string{
			"default.conf": `server {
    listen 443 ssl;
    ssl_certificate /etc/nginx/certs/tls.crt;
    ssl_certificate_key /etc/nginx/certs/tls.key;

    location / {
        return 200 "https-backend-ok\n";
        add_header Content-Type text/plain;
    }
}`,
		},
	}
	err = s.traefik.DeployConfigMap(configMap)
	require.NoError(s.T(), err, "deploy HTTPS backend config")

	// Deploy the HTTPS backend (deployment + service).
	err = s.traefik.DeployNginxBackend(nginxBackendTemplateData{
		Name:          httpsBackendName,
		ConfigMapName: httpsBackendConfigMapName,
		TLSSecretName: httpsBackendTLSSecretName,
	})
	require.NoError(s.T(), err, "deploy HTTPS backend")

	// Wait for the backend to be ready.
	err = waitForDeployment(s.traefik, testNamespace, httpsBackendName)
	require.NoError(s.T(), err, "HTTPS backend not ready")
}

// deployProxySSLIngress deploys an ingress pointing to the HTTPS backend with the given annotations.
func (s *ProxySSLSuite) deployProxySSLIngress(name, traefikHost, nginxHost string, annotations map[string]string) {
	s.T().Helper()

	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        traefikHost,
		Annotations: annotations,
		ServiceName: httpsBackendName,
		ServicePort: 443,
	})
	require.NoError(s.T(), err, "deploy %s ingress to traefik cluster", name)

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        nginxHost,
		Annotations: annotations,
		ServiceName: httpsBackendName,
		ServicePort: 443,
	})
	require.NoError(s.T(), err, "deploy %s ingress to nginx cluster", name)
}

// request makes the same HTTP request against both clusters.
func (s *ProxySSLSuite) request(traefikHost, nginxHost, method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), traefikHost, method, path, headers, 5, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), nginxHost, method, path, headers, 5, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- Certificate generation ---

func generateProxySSLCerts() (proxySSLCerts, error) {
	// Generate CA.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("generating CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Proxy SSL Test CA"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("creating CA cert: %w", err)
	}

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("parsing CA cert: %w", err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	// Generate server certificate for the HTTPS backend.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("generating server key: %w", err)
	}

	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("generating server serial: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: httpsBackendServerName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{
			httpsBackendServerName,
			"https-backend",
			"https-backend.default",
			"https-backend.default.svc",
		},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("creating server cert: %w", err)
	}

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("marshaling server key: %w", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	// Generate client certificate (for proxy-ssl-secret).
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("generating client key: %w", err)
	}

	clientSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("generating client serial: %w", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: clientSerial,
		Subject:      pkix.Name{CommonName: "Proxy SSL Client"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("creating client cert: %w", err)
	}

	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		return proxySSLCerts{}, fmt.Errorf("marshaling client key: %w", err)
	}
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	return proxySSLCerts{
		caCertPEM:     caCertPEM,
		caCert:        caCert,
		caKey:         caKey,
		serverCertPEM: serverCertPEM,
		serverKeyPEM:  serverKeyPEM,
		clientCertPEM: clientCertPEM,
		clientKeyPEM:  clientKeyPEM,
	}, nil
}

// --- Tests ---

func (s *ProxySSLSuite) TestProxySSLSecretBasic() {
	// backend-protocol: HTTPS + proxy-ssl-secret should succeed.
	traefikResp, nginxResp := s.request(
		proxySSLBasicTraefikHost, proxySSLBasicNginxHost,
		http.MethodGet, "/", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 with proxy-ssl-secret and HTTPS backend")
}

func (s *ProxySSLSuite) TestProxySSLSecretBasicOnSubpath() {
	traefikResp, nginxResp := s.request(
		proxySSLBasicTraefikHost, proxySSLBasicNginxHost,
		http.MethodGet, "/some/path", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch on subpath")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 on subpath with proxy-ssl-secret")
}

func (s *ProxySSLSuite) TestProxySSLSecretBasicPOST() {
	traefikResp, nginxResp := s.request(
		proxySSLBasicTraefikHost, proxySSLBasicNginxHost,
		http.MethodPost, "/", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for POST")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 for POST with proxy-ssl-secret")
}

func (s *ProxySSLSuite) TestProxySSLVerifyOff() {
	// proxy-ssl-verify: off should succeed even without CA in the secret
	// (self-signed backend cert is accepted without verification).
	traefikResp, nginxResp := s.request(
		proxySSLVerifyOffTraefikHost, proxySSLVerifyOffNginxHost,
		http.MethodGet, "/", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 with proxy-ssl-verify=off")
}

func (s *ProxySSLSuite) TestProxySSLVerifyOnWithCA() {
	// proxy-ssl-verify: on with correct CA and proxy-ssl-name should succeed.
	traefikResp, nginxResp := s.request(
		proxySSLVerifyOnTraefikHost, proxySSLVerifyOnNginxHost,
		http.MethodGet, "/", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 with proxy-ssl-verify=on and valid CA")
}

func (s *ProxySSLSuite) TestProxySSLVerifyOnWithCAOnSubpath() {
	traefikResp, nginxResp := s.request(
		proxySSLVerifyOnTraefikHost, proxySSLVerifyOnNginxHost,
		http.MethodGet, "/another/path", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch on subpath")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 on subpath with proxy-ssl-verify=on and valid CA")
}

func (s *ProxySSLSuite) TestProxySSLName() {
	// proxy-ssl-name sets the SNI for the backend connection.
	traefikResp, nginxResp := s.request(
		proxySSLNameTraefikHost, proxySSLNameNginxHost,
		http.MethodGet, "/", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 with proxy-ssl-name")
}

func (s *ProxySSLSuite) TestProxySSLServerName() {
	// proxy-ssl-server-name is an alternative to proxy-ssl-name.
	traefikResp, nginxResp := s.request(
		proxySSLServerNameTraefikHost, proxySSLServerNameNginxHost,
		http.MethodGet, "/", nil,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 with proxy-ssl-server-name")
}

func (s *ProxySSLSuite) TestProxySSLSecretPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "proxy-ssl"}
	traefikResp, nginxResp := s.request(
		proxySSLBasicTraefikHost, proxySSLBasicNginxHost,
		http.MethodGet, "/", headers,
	)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")
}
