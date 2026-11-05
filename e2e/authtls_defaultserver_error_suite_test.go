package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// An Ingress declares, on a rule without host, an auth-tls-secret that does not exist.
	// The rule belongs to the ingress-nginx default server, which then answers 403 to the requests for the hosts no other
	// server matches.
	authTLSDefaultServerErrorIngress    = "mtls-default-server-error"
	authTLSDefaultServerErrorSecretName = "mtls-default-server-error-secret"

	// Another Ingress declares a path on a host of its own, which the 403 of the default server must leave alone.
	authTLSDefaultServerErrorKnownHost    = "mtls-default-server-error-known"
	authTLSDefaultServerErrorKnownIngress = "mtls-default-server-error-known"

	// No Ingress declares this host.
	authTLSDefaultServerErrorUnknownHost = "mtls-default-server-error-unknown"
)

// AuthTLSDefaultServerErrorSuite checks an auth-tls-secret that cannot be used on a rule without host.
// It runs apart from the other auth-tls suites: both controllers see the Ingresses of both clusters, so the 403 of the default
// server applies to every host none of their Ingresses declares, which the other suites rely on while waiting for their paths.
type AuthTLSDefaultServerErrorSuite struct {
	BaseSuite
}

func TestAuthTLSDefaultServerErrorSuite(t *testing.T) {
	suite.Run(t, new(AuthTLSDefaultServerErrorSuite))
}

func (s *AuthTLSDefaultServerErrorSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// TearDownSuite does not run when SetupSuite fails, and a remaining Ingress would break the suites running afterward.
	s.T().Cleanup(s.cleanup)

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngressWith(ingressTemplateData{
			Name: authTLSDefaultServerErrorKnownIngress,
			Host: authTLSDefaultServerErrorKnownHost + "." + c.Name + ".local",
			Path: "/known",
		})
		require.NoError(s.T(), err, "deploy %s ingress to %s cluster", authTLSDefaultServerErrorKnownIngress, c.Name)
	}

	// A single Ingress serves both controllers: they both see it, and the ingress-nginx admission webhook refuses a second
	// Ingress declaring the same path without host.
	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name: authTLSDefaultServerErrorIngress,
		Path: "/missing",
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSDefaultServerErrorSecretName,
			"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
		},
	})
	require.NoError(s.T(), err, "deploy %s ingress", authTLSDefaultServerErrorIngress)

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		s.waitForStatus(c, authTLSDefaultServerErrorKnownHost, "/known", http.StatusOK)
		s.waitForStatus(c, authTLSDefaultServerErrorUnknownHost, "/", http.StatusForbidden)
	}
}

// cleanup deletes the Ingresses of the suite, and waits until the hosts no Ingress declares stop answering 403.
func (s *AuthTLSDefaultServerErrorSuite) cleanup() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}

	_ = s.traefik.DeleteIngress(authTLSDefaultServerErrorIngress)
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		_ = c.DeleteIngress(authTLSDefaultServerErrorKnownIngress)
	}

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		for range 30 {
			resp := c.MakeRequest(s.T(), authTLSDefaultServerErrorUnknownHost+"."+c.Name+".local", http.MethodGet, "/", nil, 1, time.Second)
			if resp != nil && resp.StatusCode != http.StatusForbidden {
				break
			}
			time.Sleep(time.Second)
		}
	}
}

// waitForStatus waits until the controller answers the given status over HTTP for <host>.<cluster>.local.
// It fails the suite when it never does, as the assertions would then prove nothing.
func (s *AuthTLSDefaultServerErrorSuite) waitForStatus(c *Cluster, host, path string, status int) {
	s.T().Helper()

	for range 30 {
		resp := c.MakeRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, path, nil, 1, time.Second)
		if resp != nil && resp.StatusCode == status {
			return
		}
		time.Sleep(time.Second)
	}

	s.T().Fatalf("[%s] path %s on host %s does not answer %d after 30 retries", c.Name, path, host, status)
}

// statuses sends the same request to both controllers, over HTTP and HTTPS, and returns the status codes they answered.
func (s *AuthTLSDefaultServerErrorSuite) statuses(host, path string) (traefikStatuses, nginxStatuses []int) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		httpResp := c.MakeRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, path, nil, 3, time.Second)
		require.NotNil(s.T(), httpResp, "%s: HTTP %s%s", c.Name, host, path)
		httpsResp := c.MakeTLSRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, path, nil, nil, 3, time.Second)
		require.NotNil(s.T(), httpsResp, "%s: HTTPS %s%s", c.Name, host, path)

		statuses := []int{httpResp.StatusCode, httpsResp.StatusCode}
		if c == s.traefik {
			traefikStatuses = statuses
		} else {
			nginxStatuses = statuses
		}
	}

	return traefikStatuses, nginxStatuses
}

// TestUnknownHostAnswersForbidden checks that the hosts no Ingress declares answer 403, on every path.
func (s *AuthTLSDefaultServerErrorSuite) TestUnknownHostAnswersForbidden() {
	for _, path := range []string{"/missing", "/"} {
		traefikStatuses, nginxStatuses := s.statuses(authTLSDefaultServerErrorUnknownHost, path)

		assert.Equal(s.T(), []int{http.StatusForbidden, http.StatusForbidden}, nginxStatuses, "nginx: %s", path)
		assert.Equal(s.T(), nginxStatuses, traefikStatuses, "traefik: %s", path)
	}
}

// TestKnownHostIsNotForbidden checks that a host declared by another Ingress keeps serving its paths, and answers the paths
// it does not declare like without the default server error, over HTTP and HTTPS.
func (s *AuthTLSDefaultServerErrorSuite) TestKnownHostIsNotForbidden() {
	traefikStatuses, nginxStatuses := s.statuses(authTLSDefaultServerErrorKnownHost, "/known")

	assert.Equal(s.T(), []int{http.StatusOK, http.StatusOK}, nginxStatuses, "nginx: /known")
	assert.Equal(s.T(), nginxStatuses, traefikStatuses, "traefik: /known")

	traefikStatuses, nginxStatuses = s.statuses(authTLSDefaultServerErrorKnownHost, "/undeclared")

	assert.Equal(s.T(), []int{http.StatusNotFound, http.StatusNotFound}, nginxStatuses, "nginx: /undeclared")
	assert.Equal(s.T(), nginxStatuses, traefikStatuses, "traefik: /undeclared")
}
