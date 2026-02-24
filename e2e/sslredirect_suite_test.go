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
	sslRedirectIngressName   = "ssl-redirect-test"
	sslNoRedirectIngressName = "ssl-no-redirect-test"

	sslRedirectTraefikHost   = sslRedirectIngressName + ".traefik.local"
	sslRedirectNginxHost     = sslRedirectIngressName + ".nginx.local"
	sslNoRedirectTraefikHost = sslNoRedirectIngressName + ".traefik.local"
	sslNoRedirectNginxHost   = sslNoRedirectIngressName + ".nginx.local"
)

type SSLRedirectSuite struct {
	BaseSuite
}

func TestSSLRedirectSuite(t *testing.T) {
	suite.Run(t, new(SSLRedirectSuite))
}

func (s *SSLRedirectSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress with force-ssl-redirect enabled.
	// force-ssl-redirect works without TLS configured on the ingress,
	// unlike ssl-redirect which only triggers when TLS is present.
	redirectAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/force-ssl-redirect": "true",
	}

	err := s.traefik.DeployIngress(sslRedirectIngressName, sslRedirectTraefikHost, redirectAnnotations)
	require.NoError(s.T(), err, "deploy ssl-redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(sslRedirectIngressName, sslRedirectNginxHost, redirectAnnotations)
	require.NoError(s.T(), err, "deploy ssl-redirect ingress to nginx cluster")

	// Ingress with ssl-redirect disabled.
	noRedirectAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/ssl-redirect": "false",
	}

	err = s.traefik.DeployIngress(sslNoRedirectIngressName, sslNoRedirectTraefikHost, noRedirectAnnotations)
	require.NoError(s.T(), err, "deploy ssl-no-redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(sslNoRedirectIngressName, sslNoRedirectNginxHost, noRedirectAnnotations)
	require.NoError(s.T(), err, "deploy ssl-no-redirect ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), sslRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), sslRedirectNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), sslNoRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), sslNoRedirectNginxHost, 20, 1*time.Second)
}

func (s *SSLRedirectSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(sslRedirectIngressName)
	_ = s.nginx.DeleteIngress(sslRedirectIngressName)
	_ = s.traefik.DeleteIngress(sslNoRedirectIngressName)
	_ = s.nginx.DeleteIngress(sslNoRedirectIngressName)
}

// redirectRequest makes the same HTTP request against both clusters using the ssl-redirect enabled ingress.
func (s *SSLRedirectSuite) redirectRequest(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), sslRedirectTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), sslRedirectNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// noRedirectRequest makes the same HTTP request against both clusters using the ssl-redirect disabled ingress.
func (s *SSLRedirectSuite) noRedirectRequest(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), sslNoRedirectTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), sslNoRedirectNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SSLRedirectSuite) TestSSLRedirectEnabled() {
	traefikResp, nginxResp := s.redirectRequest(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// Location headers contain different hostnames (.traefik.local vs .nginx.local)
	// so we only compare status codes here. TestSSLRedirectLocationHeader validates
	// the Location header format independently.
	assert.NotEmpty(s.T(), traefikResp.ResponseHeaders.Get("Location"), "traefik should have Location header")
	assert.NotEmpty(s.T(), nginxResp.ResponseHeaders.Get("Location"), "nginx should have Location header")
}

func (s *SSLRedirectSuite) TestSSLRedirectLocationHeader() {
	traefikResp, nginxResp := s.redirectRequest(http.MethodGet, "/", nil)

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.NotEmpty(s.T(), traefikLocation, "traefik Location header should be present")
	assert.NotEmpty(s.T(), nginxLocation, "nginx Location header should be present")

	assert.True(s.T(), strings.HasPrefix(traefikLocation, "https://"),
		"traefik Location should start with https://, got: %s", traefikLocation)
	assert.True(s.T(), strings.HasPrefix(nginxLocation, "https://"),
		"nginx Location should start with https://, got: %s", nginxLocation)
}

func (s *SSLRedirectSuite) TestSSLRedirectDisabled() {
	traefikResp, nginxResp := s.noRedirectRequest(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 OK when ssl-redirect is disabled")

	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("Location"),
		"traefik should not have Location header when ssl-redirect is disabled")
	assert.Empty(s.T(), nginxResp.ResponseHeaders.Get("Location"),
		"nginx should not have Location header when ssl-redirect is disabled")
}

func (s *SSLRedirectSuite) TestSSLRedirectPreservesPath() {
	traefikResp, nginxResp := s.redirectRequest(http.MethodGet, "/some/path", nil)

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.NotEmpty(s.T(), traefikLocation, "traefik Location header should be present")
	assert.NotEmpty(s.T(), nginxLocation, "nginx Location header should be present")

	assert.True(s.T(), strings.HasPrefix(traefikLocation, "https://"),
		"traefik Location should start with https://, got: %s", traefikLocation)
	assert.True(s.T(), strings.HasPrefix(nginxLocation, "https://"),
		"nginx Location should start with https://, got: %s", nginxLocation)

	assert.True(s.T(), strings.HasSuffix(traefikLocation, "/some/path"),
		"traefik Location should preserve path, got: %s", traefikLocation)
	assert.True(s.T(), strings.HasSuffix(nginxLocation, "/some/path"),
		"nginx Location should preserve path, got: %s", nginxLocation)
}
