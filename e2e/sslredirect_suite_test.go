package e2e

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	sslRedirectIngressName         = "ssl-redirect-test"
	sslNoRedirectIngressName       = "ssl-no-redirect-test"
	sslRedirectExplicitIngressName = "ssl-redirect-explicit-test"

	sslRedirectTraefikHost         = sslRedirectIngressName + ".traefik.local"
	sslRedirectNginxHost           = sslRedirectIngressName + ".nginx.local"
	sslRedirectGatewayHost         = sslRedirectIngressName + ".gateway.local"
	sslNoRedirectTraefikHost       = sslNoRedirectIngressName + ".traefik.local"
	sslNoRedirectNginxHost         = sslNoRedirectIngressName + ".nginx.local"
	sslNoRedirectGatewayHost       = sslNoRedirectIngressName + ".gateway.local"
	sslRedirectExplicitTraefikHost = sslRedirectExplicitIngressName + ".traefik.local"
	sslRedirectExplicitNginxHost   = sslRedirectExplicitIngressName + ".nginx.local"
	sslRedirectExplicitGatewayHost = sslRedirectExplicitIngressName + ".gateway.local"
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

	// Ingress with ssl-redirect explicitly enabled.
	explicitAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/ssl-redirect": "true",
	}

	err = s.traefik.DeployIngress(sslRedirectExplicitIngressName, sslRedirectExplicitTraefikHost, explicitAnnotations)
	require.NoError(s.T(), err, "deploy ssl-redirect-explicit ingress to traefik cluster")

	err = s.nginx.DeployIngress(sslRedirectExplicitIngressName, sslRedirectExplicitNginxHost, explicitAnnotations)
	require.NoError(s.T(), err, "deploy ssl-redirect-explicit ingress to nginx cluster")

	// Deploy Gateway API equivalents.
	gwDir := filepath.Join(fixturesDir, "gateway", "sslredirect")
	for _, f := range []string{"redirect.yaml", "no-redirect.yaml", "explicit.yaml"} {
		err = s.gateway.DeployGatewayFixture(filepath.Join(gwDir, f))
		require.NoError(s.T(), err, "deploy gateway fixture %s", f)
	}

	s.traefik.WaitForIngressReady(s.T(), sslRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), sslRedirectNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), sslNoRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), sslNoRedirectNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), sslRedirectExplicitTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), sslRedirectExplicitNginxHost, 20, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), sslRedirectGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), sslNoRedirectGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), sslRedirectExplicitGatewayHost, 60, 1*time.Second)
}

func (s *SSLRedirectSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(sslRedirectIngressName)
	_ = s.nginx.DeleteIngress(sslRedirectIngressName)
	_ = s.traefik.DeleteIngress(sslNoRedirectIngressName)
	_ = s.nginx.DeleteIngress(sslNoRedirectIngressName)
	_ = s.traefik.DeleteIngress(sslRedirectExplicitIngressName)
	_ = s.nginx.DeleteIngress(sslRedirectExplicitIngressName)

	gwDir := filepath.Join(fixturesDir, "gateway", "sslredirect")
	for _, f := range []string{"redirect.yaml", "no-redirect.yaml", "explicit.yaml"} {
		_ = s.gateway.DeleteGatewayFixture(filepath.Join(gwDir, f))
	}
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

	gatewayResp := s.gateway.MakeRequest(s.T(), sslRedirectGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	// MIGRATION GAP: Traefik ingress uses 308 (Permanent Redirect) for force-ssl-redirect.
	// Gateway API RequestRedirect only supports 301 and 302 per spec, so 308 is not available.
	assert.Equal(s.T(), http.StatusMovedPermanently, gatewayResp.StatusCode, "gateway migration: force-ssl-redirect maps to 301 (Gateway API does not support 308)")
	assert.NotEmpty(s.T(), gatewayResp.ResponseHeaders.Get("Location"), "gateway should have Location header")
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

	gatewayResp := s.gateway.MakeRequest(s.T(), sslRedirectGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayLocation := gatewayResp.ResponseHeaders.Get("Location")
	assert.NotEmpty(s.T(), gatewayLocation, "gateway Location header should be present")
	assert.True(s.T(), strings.HasPrefix(gatewayLocation, "https://"),
		"gateway Location should start with https://, got: %s", gatewayLocation)
}

func (s *SSLRedirectSuite) TestSSLRedirectDisabled() {
	traefikResp, nginxResp := s.noRedirectRequest(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 OK when ssl-redirect is disabled")

	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("Location"),
		"traefik should not have Location header when ssl-redirect is disabled")
	assert.Empty(s.T(), nginxResp.ResponseHeaders.Get("Location"),
		"nginx should not have Location header when ssl-redirect is disabled")

	gatewayResp := s.gateway.MakeRequest(s.T(), sslNoRedirectGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	assert.Empty(s.T(), gatewayResp.ResponseHeaders.Get("Location"),
		"gateway should not have Location header when ssl-redirect is disabled")
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

	gatewayResp := s.gateway.MakeRequest(s.T(), sslRedirectGatewayHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayLocation := gatewayResp.ResponseHeaders.Get("Location")
	assert.NotEmpty(s.T(), gatewayLocation, "gateway Location header should be present")
	assert.True(s.T(), strings.HasPrefix(gatewayLocation, "https://"),
		"gateway Location should start with https://, got: %s", gatewayLocation)
	assert.True(s.T(), strings.HasSuffix(gatewayLocation, "/some/path"),
		"gateway Location should preserve path, got: %s", gatewayLocation)
}

// explicitRedirectRequest makes the same HTTP request against both clusters using the ssl-redirect explicit ingress.
func (s *SSLRedirectSuite) explicitRedirectRequest(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), sslRedirectExplicitTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), sslRedirectExplicitNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SSLRedirectSuite) TestSSLRedirectExplicitWithoutTLS() {
	// ssl-redirect: "true" only triggers when TLS is configured on the Ingress.
	// Without a TLS section, both controllers should serve normally (no redirect).
	traefikResp, nginxResp := s.explicitRedirectRequest(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 when ssl-redirect is true but no TLS configured")

	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("Location"),
		"traefik should not redirect without TLS on the ingress")
	assert.Empty(s.T(), nginxResp.ResponseHeaders.Get("Location"),
		"nginx should not redirect without TLS on the ingress")

	// Migration gap: Gateway API RequestRedirect always redirects regardless of TLS config.
	// The Ingress ssl-redirect annotation only triggers when TLS is configured on the Ingress,
	// but the migrated Gateway API HTTPRoute has an unconditional RequestRedirect filter.
	// So the gateway will return 301 redirect while the Ingress returns 200.
	gatewayResp := s.gateway.MakeRequest(s.T(), sslRedirectExplicitGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusMovedPermanently, gatewayResp.StatusCode,
		"gateway migration gap: RequestRedirect always redirects, unlike ssl-redirect which requires TLS on the Ingress")
}

func (s *SSLRedirectSuite) TestSSLRedirectPreservesQueryString() {
	traefikResp, nginxResp := s.redirectRequest(http.MethodGet, "/path?key=value", nil)

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.NotEmpty(s.T(), traefikLocation, "traefik Location header should be present")
	assert.NotEmpty(s.T(), nginxLocation, "nginx Location header should be present")

	assert.True(s.T(), strings.HasSuffix(traefikLocation, "/path?key=value"),
		"traefik Location should preserve query string, got: %s", traefikLocation)
	assert.True(s.T(), strings.HasSuffix(nginxLocation, "/path?key=value"),
		"nginx Location should preserve query string, got: %s", nginxLocation)

	gatewayResp := s.gateway.MakeRequest(s.T(), sslRedirectGatewayHost, http.MethodGet, "/path?key=value", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayLocation := gatewayResp.ResponseHeaders.Get("Location")
	assert.NotEmpty(s.T(), gatewayLocation, "gateway Location header should be present")
	assert.True(s.T(), strings.HasSuffix(gatewayLocation, "/path?key=value"),
		"gateway Location should preserve query string, got: %s", gatewayLocation)
}
