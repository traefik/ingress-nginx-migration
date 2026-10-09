package e2e

import (
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// One Ingress requires a client certificate, another one declares a path on the same host without any auth-tls annotation.
	authTLSMixedHost                 = "mtls-mixed-host"
	authTLSMixedHostProtectedIngress = "mtls-mixed-host-protected"
	authTLSMixedHostPublicIngress    = "mtls-mixed-host-public"

	// Two Ingresses on the same host declare different auth-tls-verify-client values.
	// The "required" one is processed first by both controllers: ingresses are sorted by creation timestamp,
	// ties being broken by descending namespace/name, and it is both created first and named after "optional".
	authTLSMixedModesHost            = "mtls-mixed-modes"
	authTLSMixedModesRequiredIngress = "mtls-mixed-modes-required"
	authTLSMixedModesOptionalIngress = "mtls-mixed-modes-optional"

	authTLSSharedHostCASecretName  = "mtls-shared-host-ca"
	authTLSSharedHostTLSSecretName = "mtls-shared-host-tls"
)

// AuthTLSSharedHostSuite checks that client certificate authentication applies to every path of a host,
// as ingress-nginx configures it on the server block from the first Ingress declaring it for the host.
type AuthTLSSharedHostSuite struct {
	BaseSuite

	certs authTLSCerts
}

func TestAuthTLSSharedHostSuite(t *testing.T) {
	suite.Run(t, new(AuthTLSSharedHostSuite))
}

func (s *AuthTLSSharedHostSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	certs, err := generateAuthTLSCerts()
	require.NoError(s.T(), err, "generate mTLS certificates")
	s.certs = certs

	serverCert, serverKey, err := generateServerCert(s.certs.caCert, s.certs.caKey,
		authTLSMixedHost+".traefik.local", authTLSMixedHost+".nginx.local",
		authTLSMixedModesHost+".traefik.local", authTLSMixedModesHost+".nginx.local",
	)
	require.NoError(s.T(), err, "generate server certificate")

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err = c.DeploySecret(secretTemplateData{
			Name: authTLSSharedHostCASecretName,
			Type: "Opaque",
			Data: map[string]string{
				"ca.crt": base64.StdEncoding.EncodeToString(s.certs.caCertPEM),
			},
		})
		require.NoError(s.T(), err, "deploy CA secret to %s cluster", c.Name)

		err = c.DeploySecret(secretTemplateData{
			Name: authTLSSharedHostTLSSecretName,
			Type: "kubernetes.io/tls",
			Data: map[string]string{
				"tls.crt": base64.StdEncoding.EncodeToString(serverCert),
				"tls.key": base64.StdEncoding.EncodeToString(serverKey),
			},
		})
		require.NoError(s.T(), err, "deploy server TLS secret to %s cluster", c.Name)
	}

	s.deployIngress(authTLSMixedHostProtectedIngress, authTLSMixedHost, "/protected", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSMixedHostPublicIngress, authTLSMixedHost, "/public", "", nil)

	s.deployIngress(authTLSMixedModesRequiredIngress, authTLSMixedModesHost, "/required", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSMixedModesOptionalIngress, authTLSMixedModesHost, "/optional", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "optional",
	})

	// Every Ingress of a host must be loaded before asserting: with only the protected one,
	// the host would require a client certificate even on an affected Traefik version.
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		s.waitForPath(c, authTLSMixedHost, "/protected")
		s.waitForPath(c, authTLSMixedHost, "/public")
		s.waitForPath(c, authTLSMixedModesHost, "/required")
		s.waitForPath(c, authTLSMixedModesHost, "/optional")
	}
}

func (s *AuthTLSSharedHostSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		_ = c.DeleteIngress(authTLSMixedHostProtectedIngress)
		_ = c.DeleteIngress(authTLSMixedHostPublicIngress)
		_ = c.DeleteIngress(authTLSMixedModesRequiredIngress)
		_ = c.DeleteIngress(authTLSMixedModesOptionalIngress)
		_ = c.DeleteSecret(authTLSSharedHostCASecretName)
		_ = c.DeleteSecret(authTLSSharedHostTLSSecretName)
	}
}

// deployIngress deploys an Ingress declaring a single path on <host>.<cluster>.local to both clusters.
func (s *AuthTLSSharedHostSuite) deployIngress(name, host, path, tlsSecret string, annotations map[string]string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngressWith(ingressTemplateData{
			Name:        name,
			Host:        host + "." + c.Name + ".local",
			Path:        path,
			Annotations: annotations,
			TLSSecret:   tlsSecret,
		})
		require.NoError(s.T(), err, "deploy %s ingress to %s cluster", name, c.Name)
	}
}

// waitForPath waits until the controller routes the given path, over HTTP so that it does not depend on client certificates.
// Like WaitForIngressReady, it fails the suite when the path never starts routing, as the assertions would then prove nothing.
func (s *AuthTLSSharedHostSuite) waitForPath(c *Cluster, host, path string) {
	s.T().Helper()

	for range 30 {
		resp := c.MakeRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, path, nil, 1, time.Second)
		if resp != nil && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadGateway {
			return
		}
		time.Sleep(time.Second)
	}

	s.T().Fatalf("[%s] path %s on host %s not ready after 30 retries", c.Name, path, host)
}

// request sends the same HTTPS request to both controllers.
func (s *AuthTLSSharedHostSuite) request(host, path string, clientCert *tls.Certificate) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeTLSRequest(s.T(), host+".traefik.local", http.MethodGet, path, nil, clientCert, 3, time.Second)
	nginxResp = s.nginx.MakeTLSRequest(s.T(), host+".nginx.local", http.MethodGet, path, nil, clientCert, 3, time.Second)

	return traefikResp, nginxResp
}

// assertClientCertRejected asserts that both controllers refused the request.
// Traefik rejects the TLS handshake (nil response), while ingress-nginx answers with a 400.
func (s *AuthTLSSharedHostSuite) assertClientCertRejected(traefikResp, nginxResp *Response, path string) {
	s.T().Helper()

	if nginxResp != nil {
		assert.Contains(s.T(), []int{http.StatusBadRequest, http.StatusForbidden}, nginxResp.StatusCode,
			"nginx: expected %s to be rejected without a valid client certificate", path)
	}

	if traefikResp != nil {
		assert.Contains(s.T(), []int{http.StatusBadRequest, http.StatusForbidden}, traefikResp.StatusCode,
			"traefik: expected %s to be rejected without a valid client certificate, got %d (body: %s)", path, traefikResp.StatusCode, traefikResp.Body)
	}
}

// assertServed asserts that both controllers served the request.
func (s *AuthTLSSharedHostSuite) assertServed(traefikResp, nginxResp *Response, path string) {
	s.T().Helper()

	require.NotNil(s.T(), traefikResp, "traefik: %s should be served with a valid client certificate", path)
	require.NotNil(s.T(), nginxResp, "nginx: %s should be served with a valid client certificate", path)

	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: expected 200 on %s with a valid client certificate", path)
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: expected 200 on %s with a valid client certificate", path)
}

// TestAnnotatedPathStaysProtected checks that adding an Ingress without auth-tls annotations on a protected host
// does not disable client certificate authentication on the protected path.
func (s *AuthTLSSharedHostSuite) TestAnnotatedPathStaysProtected() {
	traefikResp, nginxResp := s.request(authTLSMixedHost, "/protected", nil)
	s.assertClientCertRejected(traefikResp, nginxResp, "/protected")

	traefikResp, nginxResp = s.request(authTLSMixedHost, "/protected", &s.certs.invalidClientCert)
	s.assertClientCertRejected(traefikResp, nginxResp, "/protected")

	traefikResp, nginxResp = s.request(authTLSMixedHost, "/protected", &s.certs.clientCert)
	s.assertServed(traefikResp, nginxResp, "/protected")
}

// TestUnannotatedPathRequiresClientCert checks that a path declared by an Ingress without auth-tls annotations
// requires a client certificate when another Ingress configures client certificate authentication for the host.
func (s *AuthTLSSharedHostSuite) TestUnannotatedPathRequiresClientCert() {
	traefikResp, nginxResp := s.request(authTLSMixedHost, "/public", nil)
	s.assertClientCertRejected(traefikResp, nginxResp, "/public")

	traefikResp, nginxResp = s.request(authTLSMixedHost, "/public", &s.certs.clientCert)
	s.assertServed(traefikResp, nginxResp, "/public")
}

// TestFirstIngressClientAuthWins checks that, when Ingresses on the same host declare different auth-tls-verify-client values,
// the configuration of the first Ingress applies to the whole host.
func (s *AuthTLSSharedHostSuite) TestFirstIngressClientAuthWins() {
	for _, path := range []string{"/required", "/optional"} {
		traefikResp, nginxResp := s.request(authTLSMixedModesHost, path, nil)
		s.assertClientCertRejected(traefikResp, nginxResp, path)

		traefikResp, nginxResp = s.request(authTLSMixedModesHost, path, &s.certs.clientCert)
		s.assertServed(traefikResp, nginxResp, path)
	}
}
