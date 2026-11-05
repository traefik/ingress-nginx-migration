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
	// A single Ingress with a host-less rule declares client certificate authentication.
	// Both controllers watch every Ingress of the shared cluster, so it is deployed once and applies to both.
	authTLSDefaultServerIngress = "mtls-default-server"

	// An Ingress without auth-tls annotations declares a known host, whose server block does not require a client certificate.
	authTLSDefaultServerKnownIngress = "mtls-default-server-known"
	authTLSDefaultServerKnownHost    = "mtls-default-server-known"

	// An Ingress declares a known host with its own, optional, client certificate authentication.
	authTLSDefaultServerOptionalIngress = "mtls-default-server-optional"
	authTLSDefaultServerOptionalHost    = "mtls-default-server-optional"

	// No Ingress declares this host, so its requests are served by the default server.
	authTLSDefaultServerUnknownHost = "mtls-default-server-unknown"

	authTLSDefaultServerCASecretName = "mtls-default-server-ca"
)

// AuthTLSDefaultServerSuite checks that client certificate authentication declared on a host-less rule applies to the default server,
// which serves every request whose host matches no other server, and leaves the known hosts without client authentication.
// While the host-less Ingress exists, every request to an unknown host is routed to it and requires a client certificate,
// on both controllers, which would break the other suites: it is removed, and its effect waited for to clear, when the suite ends.
type AuthTLSDefaultServerSuite struct {
	BaseSuite

	certs authTLSCerts
}

func TestAuthTLSDefaultServerSuite(t *testing.T) {
	suite.Run(t, new(AuthTLSDefaultServerSuite))
}

func (s *AuthTLSDefaultServerSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	certs, err := generateAuthTLSCerts()
	require.NoError(s.T(), err, "generate mTLS certificates")
	s.certs = certs

	// The cleanup is registered before deploying anything, so that a failure in the middle of the setup still removes the host-less Ingress.
	s.T().Cleanup(s.cleanup)

	// The secret and the host-less Ingress live in the namespace shared by both controllers, so they are deployed once.
	err = s.traefik.DeploySecret(secretTemplateData{
		Name: authTLSDefaultServerCASecretName,
		Type: "Opaque",
		Data: map[string]string{
			"ca.crt": base64.StdEncoding.EncodeToString(s.certs.caCertPEM),
		},
	})
	require.NoError(s.T(), err, "deploy CA secret")

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err = c.DeployIngressWith(ingressTemplateData{
			Name: authTLSDefaultServerKnownIngress,
			Host: s.host(c, authTLSDefaultServerKnownHost),
		})
		require.NoError(s.T(), err, "deploy known host ingress to %s cluster", c.Name)

		err = c.DeployIngressWith(ingressTemplateData{
			Name: authTLSDefaultServerOptionalIngress,
			Host: s.host(c, authTLSDefaultServerOptionalHost),
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSDefaultServerCASecretName,
				"nginx.ingress.kubernetes.io/auth-tls-verify-client": "optional",
			},
		})
		require.NoError(s.T(), err, "deploy optional client auth ingress to %s cluster", c.Name)
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name: authTLSDefaultServerIngress,
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/auth-tls-secret": testNamespace + "/" + authTLSDefaultServerCASecretName,
		},
	})
	require.NoError(s.T(), err, "deploy host-less ingress")

	// The known host routes once its Ingress is loaded, and the unknown host once the host-less Ingress is loaded,
	// both checked over HTTP so that they do not depend on client certificates.
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		c.WaitForIngressReady(s.T(), s.host(c, authTLSDefaultServerKnownHost), 30, time.Second)
		c.WaitForIngressReady(s.T(), s.host(c, authTLSDefaultServerOptionalHost), 30, time.Second)
		c.WaitForIngressReady(s.T(), s.host(c, authTLSDefaultServerUnknownHost), 30, time.Second)
	}
}

func (s *AuthTLSDefaultServerSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}
}

// cleanup removes the suite resources, and waits until no controller requires a client certificate for unknown hosts anymore.
func (s *AuthTLSDefaultServerSuite) cleanup() {
	_ = s.traefik.DeleteIngress(authTLSDefaultServerIngress)
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		_ = c.DeleteIngress(authTLSDefaultServerKnownIngress)
		_ = c.DeleteIngress(authTLSDefaultServerOptionalIngress)
	}
	_ = s.traefik.DeleteSecret(authTLSDefaultServerCASecretName)

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		if !waitForDefaultServerReset(s.T(), c, s.host(c, authTLSDefaultServerUnknownHost)) {
			s.T().Errorf("[%s] unknown hosts still served by the host-less Ingress after its deletion", c.Name)
		}
	}
}

// waitForDefaultServerReset reports whether an HTTPS request to an unknown host without client certificate gets a 404 again,
// meaning the Ingress feeding the default server, and its client certificate authentication if any, are gone.
func waitForDefaultServerReset(t *testing.T, c *Cluster, unknownHost string) bool {
	t.Helper()

	for range 60 {
		resp := c.MakeTLSRequest(t, unknownHost, http.MethodGet, "/", nil, nil, 1, time.Second)
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return true
		}
		time.Sleep(time.Second)
	}

	return false
}

func (s *AuthTLSDefaultServerSuite) host(c *Cluster, name string) string {
	return name + "." + c.Name + ".local"
}

// request sends the same HTTPS request to both controllers.
func (s *AuthTLSDefaultServerSuite) request(host string, clientCert *tls.Certificate) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeTLSRequest(s.T(), s.host(s.traefik, host), http.MethodGet, "/", nil, clientCert, 3, time.Second)
	nginxResp = s.nginx.MakeTLSRequest(s.T(), s.host(s.nginx, host), http.MethodGet, "/", nil, clientCert, 3, time.Second)

	return traefikResp, nginxResp
}

// assertClientCertRejected asserts that both controllers refused the request.
// Traefik rejects the TLS handshake (nil response), while ingress-nginx answers with a 400.
func (s *AuthTLSDefaultServerSuite) assertClientCertRejected(traefikResp, nginxResp *Response, host string) {
	s.T().Helper()

	if nginxResp != nil {
		assert.Contains(s.T(), []int{http.StatusBadRequest, http.StatusForbidden}, nginxResp.StatusCode,
			"nginx: expected %s to be rejected without a valid client certificate", host)
	}

	if traefikResp != nil {
		assert.Contains(s.T(), []int{http.StatusBadRequest, http.StatusForbidden}, traefikResp.StatusCode,
			"traefik: expected %s to be rejected without a valid client certificate, got %d (body: %s)", host, traefikResp.StatusCode, traefikResp.Body)
	}
}

// assertServed asserts that both controllers served the request.
func (s *AuthTLSDefaultServerSuite) assertServed(traefikResp, nginxResp *Response, host string) {
	s.T().Helper()

	require.NotNil(s.T(), traefikResp, "traefik: %s should be served", host)
	require.NotNil(s.T(), nginxResp, "nginx: %s should be served", host)

	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: expected 200 on %s", host)
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: expected 200 on %s (body: %s)", host, traefikResp.Body)
}

// TestUnknownHostRequiresClientCert checks that a request to a host matching no Ingress, served by the default server,
// requires the client certificate declared by the host-less Ingress.
func (s *AuthTLSDefaultServerSuite) TestUnknownHostRequiresClientCert() {
	traefikResp, nginxResp := s.request(authTLSDefaultServerUnknownHost, nil)
	s.assertClientCertRejected(traefikResp, nginxResp, authTLSDefaultServerUnknownHost)

	traefikResp, nginxResp = s.request(authTLSDefaultServerUnknownHost, &s.certs.invalidClientCert)
	s.assertClientCertRejected(traefikResp, nginxResp, authTLSDefaultServerUnknownHost)

	traefikResp, nginxResp = s.request(authTLSDefaultServerUnknownHost, &s.certs.clientCert)
	s.assertServed(traefikResp, nginxResp, authTLSDefaultServerUnknownHost)
}

// TestKnownHostServesWithoutClientCert checks that a host declared by an Ingress without auth-tls annotations
// keeps serving without a client certificate, as its server block does not configure client certificate authentication.
func (s *AuthTLSDefaultServerSuite) TestKnownHostServesWithoutClientCert() {
	traefikResp, nginxResp := s.request(authTLSDefaultServerKnownHost, nil)
	s.assertServed(traefikResp, nginxResp, authTLSDefaultServerKnownHost)
}

// TestKnownHostKeepsItsClientAuth checks that a host declaring its own, optional, client certificate authentication
// keeps it, and does not require the client certificate of the default server.
func (s *AuthTLSDefaultServerSuite) TestKnownHostKeepsItsClientAuth() {
	traefikResp, nginxResp := s.request(authTLSDefaultServerOptionalHost, nil)
	s.assertServed(traefikResp, nginxResp, authTLSDefaultServerOptionalHost)

	traefikResp, nginxResp = s.request(authTLSDefaultServerOptionalHost, &s.certs.clientCert)
	s.assertServed(traefikResp, nginxResp, authTLSDefaultServerOptionalHost)
}

const (
	// A single Ingress without rules declares a default backend and client certificate authentication.
	// Both controllers watch every Ingress of the shared cluster, so it is deployed once and applies to both.
	authTLSNoRulesIngress = "mtls-no-rules"

	// No Ingress declares this host, so its requests are served by the default backend of the Ingress without rules.
	authTLSNoRulesUnknownHost = "mtls-no-rules-unknown"

	authTLSNoRulesCASecretName = "mtls-no-rules-ca"
)

// AuthTLSNoRulesSuite checks that client certificate authentication declared on an Ingress without rules is ignored:
// its default backend serves every request whose host matches no other server, without requiring a client certificate.
// While this Ingress exists, every request to an unknown host is routed to it, on both controllers,
// which would break the other suites: it is removed, and its effect waited for to clear, when the suite ends.
type AuthTLSNoRulesSuite struct {
	BaseSuite
}

func TestAuthTLSNoRulesSuite(t *testing.T) {
	suite.Run(t, new(AuthTLSNoRulesSuite))
}

func (s *AuthTLSNoRulesSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	certs, err := generateAuthTLSCerts()
	require.NoError(s.T(), err, "generate mTLS certificates")

	// The cleanup is registered before deploying anything, so that a failure in the middle of the setup still removes the Ingress.
	s.T().Cleanup(s.cleanup)

	err = s.traefik.DeploySecret(secretTemplateData{
		Name: authTLSNoRulesCASecretName,
		Type: "Opaque",
		Data: map[string]string{
			"ca.crt": base64.StdEncoding.EncodeToString(certs.caCertPEM),
		},
	})
	require.NoError(s.T(), err, "deploy CA secret")

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:           authTLSNoRulesIngress,
		NoRules:        true,
		DefaultBackend: &ingressDefaultBackend{ServiceName: "backend", ServicePort: 80},
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/auth-tls-secret": testNamespace + "/" + authTLSNoRulesCASecretName,
		},
	})
	require.NoError(s.T(), err, "deploy ingress without rules")

	// The unknown host routes once the Ingress is loaded, checked over HTTP so that it does not depend on client certificates.
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		c.WaitForIngressReady(s.T(), s.host(c), 30, time.Second)
	}
}

func (s *AuthTLSNoRulesSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}
}

// cleanup removes the suite resources, and waits until no controller serves the unknown hosts anymore.
func (s *AuthTLSNoRulesSuite) cleanup() {
	_ = s.traefik.DeleteIngress(authTLSNoRulesIngress)
	_ = s.traefik.DeleteSecret(authTLSNoRulesCASecretName)

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		if !waitForDefaultServerReset(s.T(), c, s.host(c)) {
			s.T().Errorf("[%s] unknown hosts still served by the Ingress without rules after its deletion", c.Name)
		}
	}
}

func (s *AuthTLSNoRulesSuite) host(c *Cluster) string {
	return authTLSNoRulesUnknownHost + "." + c.Name + ".local"
}

// TestUnknownHostServesWithoutClientCert checks that a request to a host matching no Ingress,
// served by the default backend of the Ingress without rules, does not require a client certificate.
func (s *AuthTLSNoRulesSuite) TestUnknownHostServesWithoutClientCert() {
	traefikResp := s.traefik.MakeTLSRequest(s.T(), s.host(s.traefik), http.MethodGet, "/", nil, nil, 3, time.Second)
	nginxResp := s.nginx.MakeTLSRequest(s.T(), s.host(s.nginx), http.MethodGet, "/", nil, nil, 3, time.Second)

	require.NotNil(s.T(), traefikResp, "traefik: unknown host should be served without client certificate")
	require.NotNil(s.T(), nginxResp, "nginx: unknown host should be served without client certificate")

	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: expected 200 on unknown host")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: expected 200 on unknown host (body: %s)", traefikResp.Body)
}
