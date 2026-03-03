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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	sslPassthroughIngressName = "ssl-passthrough-test"
	sslPassthroughTraefikHost = sslPassthroughIngressName + ".traefik.local"
	sslPassthroughNginxHost   = sslPassthroughIngressName + ".nginx.local"

	passthroughBackendName          = "passthrough-backend"
	passthroughBackendConfigMapName = "passthrough-backend-config"
	passthroughBackendTLSSecretName = "passthrough-backend-tls"

	// The CN baked into the passthrough backend's certificate.
	// Tests verify they see this CN, proving TLS was NOT terminated by the controller.
	passthroughBackendCN = "passthrough-backend-cert"
)

// sslPassthroughCerts holds all generated certificates for the ssl-passthrough test suite.
type sslPassthroughCerts struct {
	serverCertPEM []byte
	serverKeyPEM  []byte
}

type SSLPassthroughSuite struct {
	BaseSuite

	certs        sslPassthroughCerts
	traefikHTTPS string
	nginxHTTPS   string
}

func TestSSLPassthroughSuite(t *testing.T) {
	suite.Run(t, new(SSLPassthroughSuite))
}

func (s *SSLPassthroughSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.traefikHTTPS = fmt.Sprintf("%s:%s", s.traefik.Host, s.traefik.PortHTTPS)
	s.nginxHTTPS = fmt.Sprintf("%s:%s", s.nginx.Host, s.nginx.PortHTTPS)

	// Generate backend server cert with known CN and SANs.
	certs, err := generatePassthroughCerts()
	require.NoError(s.T(), err, "generate passthrough certificates")
	s.certs = certs

	// Deploy the TLS backend.
	s.deployPassthroughBackend()

	// Deploy ssl-passthrough ingresses.
	// The ingress just needs the annotation and a host; no TLS section (the controller doesn't terminate TLS).
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/ssl-passthrough": "true",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        sslPassthroughIngressName,
		Host:        sslPassthroughTraefikHost,
		Annotations: annotations,
		ServiceName: passthroughBackendName,
		ServicePort: 443,
	})
	require.NoError(s.T(), err, "deploy ssl-passthrough ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        sslPassthroughIngressName,
		Host:        sslPassthroughNginxHost,
		Annotations: annotations,
		ServiceName: passthroughBackendName,
		ServicePort: 443,
	})
	require.NoError(s.T(), err, "deploy ssl-passthrough ingress to nginx cluster")

	// Give the controllers time to pick up the passthrough config.
	// ssl-passthrough is handled differently (TCP-level), so WaitForIngressReady (HTTP probe) won't work.
	time.Sleep(10 * time.Second)
}

func (s *SSLPassthroughSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(sslPassthroughIngressName)
	_ = s.nginx.DeleteIngress(sslPassthroughIngressName)

	_ = s.traefik.DeleteSecret(passthroughBackendTLSSecretName)
	_ = s.traefik.DeleteConfigMap(passthroughBackendConfigMapName)

	_ = s.traefik.Kubectl("delete", "deployment", passthroughBackendName, "-n", testNamespace, "--ignore-not-found")
	_ = s.traefik.Kubectl("delete", "service", passthroughBackendName, "-n", testNamespace, "--ignore-not-found")
}

// deployPassthroughBackend deploys an nginx-based TLS backend for ssl-passthrough tests.
func (s *SSLPassthroughSuite) deployPassthroughBackend() {
	s.T().Helper()

	// Deploy the server TLS secret.
	serverTLSSecret := secretTemplateData{
		Name: passthroughBackendTLSSecretName,
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(s.certs.serverCertPEM),
			"tls.key": base64.StdEncoding.EncodeToString(s.certs.serverKeyPEM),
		},
	}
	err := s.traefik.DeploySecret(serverTLSSecret)
	require.NoError(s.T(), err, "deploy passthrough backend TLS secret")

	// Deploy the nginx config.
	configMap := configMapTemplateData{
		Name: passthroughBackendConfigMapName,
		Data: map[string]string{
			"default.conf": `server {
    listen 443 ssl;
    ssl_certificate /etc/nginx/certs/tls.crt;
    ssl_certificate_key /etc/nginx/certs/tls.key;

    location / {
        return 200 "passthrough-backend-ok\n";
        add_header Content-Type text/plain;
    }
}`,
		},
	}
	err = s.traefik.DeployConfigMap(configMap)
	require.NoError(s.T(), err, "deploy passthrough backend config")

	// Deploy the backend (deployment + service).
	err = s.traefik.DeployNginxBackend(nginxBackendTemplateData{
		Name:          passthroughBackendName,
		ConfigMapName: passthroughBackendConfigMapName,
		TLSSecretName: passthroughBackendTLSSecretName,
	})
	require.NoError(s.T(), err, "deploy passthrough backend")

	err = waitForDeployment(s.traefik, testNamespace, passthroughBackendName)
	require.NoError(s.T(), err, "passthrough backend not ready")
}

// makePassthroughTLSRequest connects via TLS to the given endpoint and returns
// the response along with the peer certificate CN.
func makePassthroughTLSRequest(t *testing.T, hostPort, sniHost string, maxRetries int, delay time.Duration) (resp *Response, peerCN string) {
	t.Helper()

	url := fmt.Sprintf("https://%s/", hostPort)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         sniHost,
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
		req.Host = sniHost

		httpResp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		body, err := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		r := &Response{
			StatusCode:      httpResp.StatusCode,
			Body:            string(body),
			ResponseHeaders: httpResp.Header,
		}

		// Extract the peer certificate CN.
		cn := ""
		if httpResp.TLS != nil && len(httpResp.TLS.PeerCertificates) > 0 {
			cn = httpResp.TLS.PeerCertificates[0].Subject.CommonName
		}

		return r, cn
	}

	t.Logf("passthrough TLS request to %s (sni=%s) failed after %d retries: %v", hostPort, sniHost, maxRetries, lastErr)
	return nil, ""
}

// --- Certificate generation ---

func generatePassthroughCerts() (sslPassthroughCerts, error) {
	// Self-signed server certificate with a known CN.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return sslPassthroughCerts{}, fmt.Errorf("generating server key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return sslPassthroughCerts{}, fmt.Errorf("generating serial number: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{CommonName: passthroughBackendCN},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{
			sslPassthroughTraefikHost,
			sslPassthroughNginxHost,
			"passthrough-backend",
			"passthrough-backend.default.svc.cluster.local",
		},
	}

	// Self-signed.
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, serverTemplate, &serverKey.PublicKey, serverKey)
	if err != nil {
		return sslPassthroughCerts{}, fmt.Errorf("creating server cert: %w", err)
	}

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return sslPassthroughCerts{}, fmt.Errorf("marshaling server key: %w", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	return sslPassthroughCerts{
		serverCertPEM: serverCertPEM,
		serverKeyPEM:  serverKeyPEM,
	}, nil
}

// --- Tests ---

func (s *SSLPassthroughSuite) TestSSLPassthroughCertificateIsFromBackend() {
	// The key assertion: the TLS certificate CN should be from the backend,
	// not the ingress controller. This proves the TLS was NOT terminated by the controller.
	traefikResp, traefikCN := makePassthroughTLSRequest(s.T(), s.traefikHTTPS, sslPassthroughTraefikHost, 10, 2*time.Second)
	nginxResp, nginxCN := makePassthroughTLSRequest(s.T(), s.nginxHTTPS, sslPassthroughNginxHost, 10, 2*time.Second)

	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), passthroughBackendCN, traefikCN,
		"traefik: TLS certificate CN should be from the backend (passthrough)")
	assert.Equal(s.T(), passthroughBackendCN, nginxCN,
		"nginx: TLS certificate CN should be from the backend (passthrough)")
}

func (s *SSLPassthroughSuite) TestSSLPassthroughReturnsOK() {
	traefikResp, _ := makePassthroughTLSRequest(s.T(), s.traefikHTTPS, sslPassthroughTraefikHost, 10, 2*time.Second)
	nginxResp, _ := makePassthroughTLSRequest(s.T(), s.nginxHTTPS, sslPassthroughNginxHost, 10, 2*time.Second)

	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 with ssl-passthrough")
}

func (s *SSLPassthroughSuite) TestSSLPassthroughResponseBody() {
	traefikResp, _ := makePassthroughTLSRequest(s.T(), s.traefikHTTPS, sslPassthroughTraefikHost, 10, 2*time.Second)
	nginxResp, _ := makePassthroughTLSRequest(s.T(), s.nginxHTTPS, sslPassthroughNginxHost, 10, 2*time.Second)

	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	// The response body comes directly from the backend.
	assert.Contains(s.T(), traefikResp.Body, "passthrough-backend-ok",
		"traefik: response body should come from the passthrough backend")
	assert.Contains(s.T(), nginxResp.Body, "passthrough-backend-ok",
		"nginx: response body should come from the passthrough backend")
}
