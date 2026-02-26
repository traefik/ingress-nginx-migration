package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// Non-www host: requests to www.wwwredir-nonwww.*.local should redirect to wwwredir-nonwww.*.local.
	wwwRedirNonWWWIngressName = "wwwredir-nonwww-test"
	wwwRedirNonWWWTraefikHost = wwwRedirNonWWWIngressName + ".traefik.local"
	wwwRedirNonWWWNginxHost   = wwwRedirNonWWWIngressName + ".nginx.local"

	// www host: requests to wwwredir-www.*.local (without www) should redirect to www.wwwredir-www.*.local.
	wwwRedirWWWIngressName = "wwwredir-www-test"
	wwwRedirWWWTraefikHost = "www." + wwwRedirWWWIngressName + ".traefik.local"
	wwwRedirWWWNginxHost   = "www." + wwwRedirWWWIngressName + ".nginx.local"
)

type WWWRedirectSuite struct {
	BaseSuite
}

func TestWWWRedirectSuite(t *testing.T) {
	suite.Run(t, new(WWWRedirectSuite))
}

func (s *WWWRedirectSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	redirectAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/from-to-www-redirect": "true",
	}

	// Ingress with non-www host: should redirect www → non-www.
	err := s.traefik.DeployIngress(wwwRedirNonWWWIngressName, wwwRedirNonWWWTraefikHost, redirectAnnotations)
	require.NoError(s.T(), err, "deploy non-www redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(wwwRedirNonWWWIngressName, wwwRedirNonWWWNginxHost, redirectAnnotations)
	require.NoError(s.T(), err, "deploy non-www redirect ingress to nginx cluster")

	// Ingress with www host: should redirect non-www → www.
	err = s.traefik.DeployIngress(wwwRedirWWWIngressName, wwwRedirWWWTraefikHost, redirectAnnotations)
	require.NoError(s.T(), err, "deploy www redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(wwwRedirWWWIngressName, wwwRedirWWWNginxHost, redirectAnnotations)
	require.NoError(s.T(), err, "deploy www redirect ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), wwwRedirNonWWWTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), wwwRedirNonWWWNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), wwwRedirWWWTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), wwwRedirWWWNginxHost, 20, 1*time.Second)

	// Also wait for the redirect-source hosts (the auto-generated redirect rules).
	// A 301 redirect counts as "ready" since WaitForIngressReady accepts any non-404/non-502 response.
	s.traefik.WaitForIngressReady(s.T(), "www."+wwwRedirNonWWWTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), "www."+wwwRedirNonWWWNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), strings.TrimPrefix(wwwRedirWWWTraefikHost, "www."), 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), strings.TrimPrefix(wwwRedirWWWNginxHost, "www."), 20, 1*time.Second)
}

func (s *WWWRedirectSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(wwwRedirNonWWWIngressName)
	_ = s.nginx.DeleteIngress(wwwRedirNonWWWIngressName)
	_ = s.traefik.DeleteIngress(wwwRedirWWWIngressName)
	_ = s.nginx.DeleteIngress(wwwRedirWWWIngressName)
}

func (s *WWWRedirectSuite) TestNonWWWHostServesNormally() {
	// Direct requests to the non-www host should be served normally.
	traefikResp := s.traefik.MakeRequest(s.T(), wwwRedirNonWWWTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), wwwRedirNonWWWNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for direct non-www host request")
}

func (s *WWWRedirectSuite) TestWWWToNonWWWRedirect() {
	// Requests to www.<host> should redirect to <host> (non-www).
	wwwTraefikHost := "www." + wwwRedirNonWWWTraefikHost
	wwwNginxHost := "www." + wwwRedirNonWWWNginxHost

	traefikResp := s.traefik.MakeRequest(s.T(), wwwTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), wwwNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Both should redirect (308).
	assert.Equal(s.T(), http.StatusPermanentRedirect, traefikResp.StatusCode,
		"traefik should return 308 for www → non-www redirect")
	assert.Equal(s.T(), http.StatusPermanentRedirect, nginxResp.StatusCode,
		"nginx should return 308 for www → non-www redirect")

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.NotEmpty(s.T(), traefikLocation, "traefik should have Location header")
	assert.NotEmpty(s.T(), nginxLocation, "nginx should have Location header")

	// Location should point to the non-www host.
	assert.True(s.T(), strings.Contains(traefikLocation, wwwRedirNonWWWTraefikHost),
		"traefik Location should contain non-www host, got: %s", traefikLocation)
	assert.True(s.T(), strings.Contains(nginxLocation, wwwRedirNonWWWNginxHost),
		"nginx Location should contain non-www host, got: %s", nginxLocation)
}

func (s *WWWRedirectSuite) TestWWWHostServesNormally() {
	// Direct requests to the www host should be served normally.
	traefikResp := s.traefik.MakeRequest(s.T(), wwwRedirWWWTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), wwwRedirWWWNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for direct www host request")
}

func (s *WWWRedirectSuite) TestNonWWWToWWWRedirect() {
	// Requests to <host> (without www) should redirect to www.<host>.
	nonWWWTraefikHost := strings.TrimPrefix(wwwRedirWWWTraefikHost, "www.")
	nonWWWNginxHost := strings.TrimPrefix(wwwRedirWWWNginxHost, "www.")

	traefikResp := s.traefik.MakeRequest(s.T(), nonWWWTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), nonWWWNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Both should redirect (308).
	assert.Equal(s.T(), http.StatusPermanentRedirect, traefikResp.StatusCode,
		"traefik should return 308 for non-www → www redirect")
	assert.Equal(s.T(), http.StatusPermanentRedirect, nginxResp.StatusCode,
		"nginx should return 308 for non-www → www redirect")

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.NotEmpty(s.T(), traefikLocation, "traefik should have Location header")
	assert.NotEmpty(s.T(), nginxLocation, "nginx should have Location header")

	// Location should point to the www host.
	assert.True(s.T(), strings.Contains(traefikLocation, wwwRedirWWWTraefikHost),
		"traefik Location should contain www host, got: %s", traefikLocation)
	assert.True(s.T(), strings.Contains(nginxLocation, wwwRedirWWWNginxHost),
		"nginx Location should contain www host, got: %s", nginxLocation)
}

func (s *WWWRedirectSuite) TestWWWRedirectPreservesPath() {
	wwwTraefikHost := "www." + wwwRedirNonWWWTraefikHost
	wwwNginxHost := "www." + wwwRedirNonWWWNginxHost

	traefikResp := s.traefik.MakeRequest(s.T(), wwwTraefikHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), wwwNginxHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	// Path should be preserved in the redirect.
	assert.True(s.T(), strings.HasSuffix(traefikLocation, "/some/path"),
		"traefik Location should preserve path, got: %s", traefikLocation)
	assert.True(s.T(), strings.HasSuffix(nginxLocation, "/some/path"),
		"nginx Location should preserve path, got: %s", nginxLocation)
}
