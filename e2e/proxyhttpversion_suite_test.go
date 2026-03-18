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
	// Ingress without proxy-http-version annotation (defaults to 1.1).
	httpVersionDefaultIngressName = "http-version-default-test"
	httpVersionDefaultTraefikHost = httpVersionDefaultIngressName + ".traefik.local"
	httpVersionDefaultNginxHost   = httpVersionDefaultIngressName + ".nginx.local"

	// Ingress with explicit proxy-http-version: "1.1".
	httpVersion11IngressName = "http-version-11-test"
	httpVersion11TraefikHost = httpVersion11IngressName + ".traefik.local"
	httpVersion11NginxHost   = httpVersion11IngressName + ".nginx.local"
)

type ProxyHTTPVersionSuite struct {
	BaseSuite
}

func TestProxyHTTPVersionSuite(t *testing.T) {
	suite.Run(t, new(ProxyHTTPVersionSuite))
}

func (s *ProxyHTTPVersionSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress without proxy-http-version annotation (uses default behavior).
	err := s.traefik.DeployIngress(httpVersionDefaultIngressName, httpVersionDefaultTraefikHost, nil)
	require.NoError(s.T(), err, "deploy default http-version ingress to traefik cluster")

	err = s.nginx.DeployIngress(httpVersionDefaultIngressName, httpVersionDefaultNginxHost, nil)
	require.NoError(s.T(), err, "deploy default http-version ingress to nginx cluster")

	// Ingress with explicit proxy-http-version: "1.1".
	http11Annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-http-version": "1.1",
	}

	err = s.traefik.DeployIngress(httpVersion11IngressName, httpVersion11TraefikHost, http11Annotations)
	require.NoError(s.T(), err, "deploy http-version-1.1 ingress to traefik cluster")

	err = s.nginx.DeployIngress(httpVersion11IngressName, httpVersion11NginxHost, http11Annotations)
	require.NoError(s.T(), err, "deploy http-version-1.1 ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), httpVersionDefaultTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), httpVersionDefaultNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), httpVersion11TraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), httpVersion11NginxHost, 20, 1*time.Second)
}

func (s *ProxyHTTPVersionSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(httpVersionDefaultIngressName)
	_ = s.nginx.DeleteIngress(httpVersionDefaultIngressName)
	_ = s.traefik.DeleteIngress(httpVersion11IngressName)
	_ = s.nginx.DeleteIngress(httpVersion11IngressName)
}

// requestDefault makes the same HTTP request against both clusters using the default ingress.
func (s *ProxyHTTPVersionSuite) requestDefault(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), httpVersionDefaultTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), httpVersionDefaultNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestHTTP11 makes the same HTTP request against both clusters using the explicit 1.1 ingress.
func (s *ProxyHTTPVersionSuite) requestHTTP11(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), httpVersion11TraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), httpVersion11NginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *ProxyHTTPVersionSuite) TestDefaultHTTPVersionReturnsOK() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for default http version")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for default http version")
}

func (s *ProxyHTTPVersionSuite) TestHTTPVersion11ReturnsOK() {
	traefikResp, nginxResp := s.requestHTTP11(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for explicit http/1.1")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for explicit http/1.1")
}

func (s *ProxyHTTPVersionSuite) TestHTTPVersion11OnSubpath() {
	traefikResp, nginxResp := s.requestHTTP11(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for subpath")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath with http/1.1")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for subpath with http/1.1")
}

func (s *ProxyHTTPVersionSuite) TestHTTPVersion11PreservesHeaders() {
	customHeaders := map[string]string{
		"X-Custom-Test":  "test-value",
		"X-Another-Test": "another-value",
	}

	traefikResp, nginxResp := s.requestHTTP11(http.MethodGet, "/", customHeaders)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with custom headers")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 with custom headers")

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Custom-Test"],
		traefikResp.RequestHeaders["X-Custom-Test"],
		"X-Custom-Test header passthrough mismatch",
	)
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Another-Test"],
		traefikResp.RequestHeaders["X-Another-Test"],
		"X-Another-Test header passthrough mismatch",
	)
}

func (s *ProxyHTTPVersionSuite) TestHTTPVersion11MatchesDefault() {
	traefikDefault, nginxDefault := s.requestDefault(http.MethodGet, "/", nil)
	traefikExplicit, nginxExplicit := s.requestHTTP11(http.MethodGet, "/", nil)

	assert.Equal(s.T(), traefikDefault.StatusCode, traefikExplicit.StatusCode,
		"traefik: default and explicit 1.1 should return the same status code")
	assert.Equal(s.T(), nginxDefault.StatusCode, nginxExplicit.StatusCode,
		"nginx: default and explicit 1.1 should return the same status code")
}

func (s *ProxyHTTPVersionSuite) TestHTTPVersionPOSTRequest() {
	traefikResp, nginxResp := s.requestHTTP11(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for POST")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with http/1.1")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for POST with http/1.1")
}
