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
	// A single Ingress excludes an internal subpath, with nothing else declared on the host.
	lookaheadInternalHost    = "lookahead-internal"
	lookaheadInternalIngress = "lookahead-internal"

	// A general Ingress excludes several subpaths, and a separate Ingress on the same host declares one of them.
	lookaheadSharedHost           = "lookahead-shared"
	lookaheadSharedGeneralIngress = "lookahead-shared-general"
	lookaheadSharedSessionIngress = "lookahead-shared-session"
)

// NegativeLookaheadSuite checks that a regex path excluding subpaths with a negative lookahead,
// such as /api/licensing/((?!_internal).*), routes and rewrites like ingress-nginx.
type NegativeLookaheadSuite struct {
	BaseSuite
}

func TestNegativeLookaheadSuite(t *testing.T) {
	suite.Run(t, new(NegativeLookaheadSuite))
}

func (s *NegativeLookaheadSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	rewriteAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/use-regex":      "true",
		"nginx.ingress.kubernetes.io/rewrite-target": "/$1",
	}

	s.deployIngress(lookaheadInternalIngress, lookaheadInternalHost, "/api/licensing/((?!_internal).*)", "ImplementationSpecific", rewriteAnnotations)

	s.deployIngress(lookaheadSharedGeneralIngress, lookaheadSharedHost, "/api/example/v1/((?!invitations|session).*)", "ImplementationSpecific", rewriteAnnotations)
	s.deployIngress(lookaheadSharedSessionIngress, lookaheadSharedHost, "/api/example/v1/session", "Prefix", nil)

	// Every Ingress of a host must be loaded before asserting: without the session one,
	// the excluded session path would answer 404 on both controllers and prove nothing.
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		s.waitForPath(c, lookaheadInternalHost, "/api/licensing/status")
		s.waitForPath(c, lookaheadSharedHost, "/api/example/v1/customers")
		s.waitForPath(c, lookaheadSharedHost, "/api/example/v1/session/current")
	}
}

func (s *NegativeLookaheadSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		_ = c.DeleteIngress(lookaheadInternalIngress)
		_ = c.DeleteIngress(lookaheadSharedGeneralIngress)
		_ = c.DeleteIngress(lookaheadSharedSessionIngress)
	}
}

// deployIngress deploys an Ingress declaring a single path on <host>.<cluster>.local to both clusters.
func (s *NegativeLookaheadSuite) deployIngress(name, host, path, pathType string, annotations map[string]string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngressWith(ingressTemplateData{
			Name:        name,
			Host:        host + "." + c.Name + ".local",
			Path:        path,
			PathType:    pathType,
			Annotations: annotations,
		})
		require.NoError(s.T(), err, "deploy %s ingress to %s cluster", name, c.Name)
	}
}

// waitForPath waits until the controller routes the given path.
// Like WaitForIngressReady, it fails the suite when the path never starts routing, as the assertions would then prove nothing.
func (s *NegativeLookaheadSuite) waitForPath(c *Cluster, host, path string) {
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

// request sends the same HTTP request to both controllers.
func (s *NegativeLookaheadSuite) request(host, path string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), host+".traefik.local", http.MethodGet, path, nil, 3, time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), host+".nginx.local", http.MethodGet, path, nil, 3, time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// assertBackendURI asserts that both controllers served the request and that the backend received the given URI.
func (s *NegativeLookaheadSuite) assertBackendURI(traefikResp, nginxResp *Response, path, wantURI string) {
	s.T().Helper()

	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: expected 200 on %s", path)
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: expected 200 on %s", path)

	assert.Contains(s.T(), nginxResp.Body, "GET "+wantURI+" HTTP/1.1", "nginx: backend should see URI %s for %s", wantURI, path)
	assert.Contains(s.T(), traefikResp.Body, "GET "+wantURI+" HTTP/1.1", "traefik: backend should see URI %s for %s", wantURI, path)
}

// assertNotRouted asserts that both controllers answered 404 without reaching a backend.
func (s *NegativeLookaheadSuite) assertNotRouted(traefikResp, nginxResp *Response, path string) {
	s.T().Helper()

	assert.Equal(s.T(), http.StatusNotFound, nginxResp.StatusCode, "nginx: expected 404 on %s", path)
	assert.Equal(s.T(), http.StatusNotFound, traefikResp.StatusCode, "traefik: expected 404 on %s, got body: %s", path, traefikResp.Body)
}

// TestAllowedPathIsRewritten checks that a path outside the excluded subpath is routed,
// and that rewrite-target still uses the capture group opened by the negative lookahead.
func (s *NegativeLookaheadSuite) TestAllowedPathIsRewritten() {
	for path, wantURI := range map[string]string{
		"/api/licensing/":             "/",
		"/api/licensing/status":       "/status",
		"/api/licensing/v2/keys/123":  "/v2/keys/123",
		"/api/licensing/internal/abc": "/internal/abc",
	} {
		traefikResp, nginxResp := s.request(lookaheadInternalHost, path)
		s.assertBackendURI(traefikResp, nginxResp, path, wantURI)
	}
}

// TestExcludedPathIsNotServed checks that the excluded subpath does not fall through to the general route,
// whatever its case, as ingress-nginx matches regex locations case-insensitively.
func (s *NegativeLookaheadSuite) TestExcludedPathIsNotServed() {
	for _, path := range []string{
		"/api/licensing/_internal",
		"/api/licensing/_internal/keys",
		"/api/licensing/_internalfoo",
		"/api/licensing/_INTERNAL/keys",
	} {
		traefikResp, nginxResp := s.request(lookaheadInternalHost, path)
		s.assertNotRouted(traefikResp, nginxResp, path)
	}
}

// TestExcludedPathIsServedByItsOwnIngress checks that, with several excluded subpaths,
// an excluded subpath declared by another Ingress on the host reaches that Ingress without being rewritten,
// while an excluded subpath no Ingress declares is not served.
func (s *NegativeLookaheadSuite) TestExcludedPathIsServedByItsOwnIngress() {
	traefikResp, nginxResp := s.request(lookaheadSharedHost, "/api/example/v1/customers")
	s.assertBackendURI(traefikResp, nginxResp, "/api/example/v1/customers", "/customers")

	traefikResp, nginxResp = s.request(lookaheadSharedHost, "/api/example/v1/session/current")
	s.assertBackendURI(traefikResp, nginxResp, "/api/example/v1/session/current", "/api/example/v1/session/current")

	traefikResp, nginxResp = s.request(lookaheadSharedHost, "/api/example/v1/invitations/accept")
	s.assertNotRouted(traefikResp, nginxResp, "/api/example/v1/invitations/accept")
}
