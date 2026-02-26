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
	serviceUpstreamIngressName = "service-upstream-test"
	serviceUpstreamTraefikHost = serviceUpstreamIngressName + ".traefik.local"
	serviceUpstreamNginxHost   = serviceUpstreamIngressName + ".nginx.local"

	noServiceUpstreamIngressName = "no-service-upstream-test"
	noServiceUpstreamTraefikHost = noServiceUpstreamIngressName + ".traefik.local"
	noServiceUpstreamNginxHost   = noServiceUpstreamIngressName + ".nginx.local"

	serviceUpstreamFalseIngressName = "service-upstream-false-test"
	serviceUpstreamFalseTraefikHost = serviceUpstreamFalseIngressName + ".traefik.local"
	serviceUpstreamFalseNginxHost   = serviceUpstreamFalseIngressName + ".nginx.local"

	backendProtocolHTTPIngressName = "backend-protocol-http-test"
	backendProtocolHTTPTraefikHost = backendProtocolHTTPIngressName + ".traefik.local"
	backendProtocolHTTPNginxHost   = backendProtocolHTTPIngressName + ".nginx.local"

	noBackendProtocolIngressName = "no-backend-protocol-test"
	noBackendProtocolTraefikHost = noBackendProtocolIngressName + ".traefik.local"
	noBackendProtocolNginxHost   = noBackendProtocolIngressName + ".nginx.local"

	backendProtocolHTTPSIngressName = "backend-protocol-https-test"
	backendProtocolHTTPSTraefikHost = backendProtocolHTTPSIngressName + ".traefik.local"
	backendProtocolHTTPSNginxHost   = backendProtocolHTTPSIngressName + ".nginx.local"
)

type BackendConfigSuite struct {
	BaseSuite
}

func TestBackendConfigSuite(t *testing.T) {
	suite.Run(t, new(BackendConfigSuite))
}

func (s *BackendConfigSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// 1. service-upstream: "true"
	serviceUpstreamAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/service-upstream": "true",
	}

	err := s.traefik.DeployIngress(serviceUpstreamIngressName, serviceUpstreamTraefikHost, serviceUpstreamAnnotations)
	require.NoError(s.T(), err, "deploy service-upstream ingress to traefik cluster")

	err = s.nginx.DeployIngress(serviceUpstreamIngressName, serviceUpstreamNginxHost, serviceUpstreamAnnotations)
	require.NoError(s.T(), err, "deploy service-upstream ingress to nginx cluster")

	// 2. No service-upstream annotation (default: use pod endpoints).
	noAnnotations := map[string]string{}

	err = s.traefik.DeployIngress(noServiceUpstreamIngressName, noServiceUpstreamTraefikHost, noAnnotations)
	require.NoError(s.T(), err, "deploy no-service-upstream ingress to traefik cluster")

	err = s.nginx.DeployIngress(noServiceUpstreamIngressName, noServiceUpstreamNginxHost, noAnnotations)
	require.NoError(s.T(), err, "deploy no-service-upstream ingress to nginx cluster")

	// 3. service-upstream: "false" (explicit default).
	serviceUpstreamFalseAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/service-upstream": "false",
	}

	err = s.traefik.DeployIngress(serviceUpstreamFalseIngressName, serviceUpstreamFalseTraefikHost, serviceUpstreamFalseAnnotations)
	require.NoError(s.T(), err, "deploy service-upstream-false ingress to traefik cluster")

	err = s.nginx.DeployIngress(serviceUpstreamFalseIngressName, serviceUpstreamFalseNginxHost, serviceUpstreamFalseAnnotations)
	require.NoError(s.T(), err, "deploy service-upstream-false ingress to nginx cluster")

	// 4. backend-protocol: "HTTP" (explicit).
	backendProtocolHTTPAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol": "HTTP",
	}

	err = s.traefik.DeployIngress(backendProtocolHTTPIngressName, backendProtocolHTTPTraefikHost, backendProtocolHTTPAnnotations)
	require.NoError(s.T(), err, "deploy backend-protocol-http ingress to traefik cluster")

	err = s.nginx.DeployIngress(backendProtocolHTTPIngressName, backendProtocolHTTPNginxHost, backendProtocolHTTPAnnotations)
	require.NoError(s.T(), err, "deploy backend-protocol-http ingress to nginx cluster")

	// 5. No backend-protocol annotation (default: HTTP).
	err = s.traefik.DeployIngress(noBackendProtocolIngressName, noBackendProtocolTraefikHost, map[string]string{})
	require.NoError(s.T(), err, "deploy no-backend-protocol ingress to traefik cluster")

	err = s.nginx.DeployIngress(noBackendProtocolIngressName, noBackendProtocolNginxHost, map[string]string{})
	require.NoError(s.T(), err, "deploy no-backend-protocol ingress to nginx cluster")

	// 6. backend-protocol: "HTTPS" (will fail with HTTP-only backend, but both controllers should agree).
	backendProtocolHTTPSAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/backend-protocol": "HTTPS",
	}

	err = s.traefik.DeployIngress(backendProtocolHTTPSIngressName, backendProtocolHTTPSTraefikHost, backendProtocolHTTPSAnnotations)
	require.NoError(s.T(), err, "deploy backend-protocol-https ingress to traefik cluster")

	err = s.nginx.DeployIngress(backendProtocolHTTPSIngressName, backendProtocolHTTPSNginxHost, backendProtocolHTTPSAnnotations)
	require.NoError(s.T(), err, "deploy backend-protocol-https ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), serviceUpstreamTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), serviceUpstreamNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), noServiceUpstreamTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), noServiceUpstreamNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), serviceUpstreamFalseTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), serviceUpstreamFalseNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), backendProtocolHTTPTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), backendProtocolHTTPNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), noBackendProtocolTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), noBackendProtocolNginxHost, 20, 1*time.Second)
	// Note: HTTPS backend ingress may not become "ready" since backend doesn't speak TLS.
}

func (s *BackendConfigSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(serviceUpstreamIngressName)
	_ = s.nginx.DeleteIngress(serviceUpstreamIngressName)
	_ = s.traefik.DeleteIngress(noServiceUpstreamIngressName)
	_ = s.nginx.DeleteIngress(noServiceUpstreamIngressName)
	_ = s.traefik.DeleteIngress(serviceUpstreamFalseIngressName)
	_ = s.nginx.DeleteIngress(serviceUpstreamFalseIngressName)
	_ = s.traefik.DeleteIngress(backendProtocolHTTPIngressName)
	_ = s.nginx.DeleteIngress(backendProtocolHTTPIngressName)
	_ = s.traefik.DeleteIngress(noBackendProtocolIngressName)
	_ = s.nginx.DeleteIngress(noBackendProtocolIngressName)
	_ = s.traefik.DeleteIngress(backendProtocolHTTPSIngressName)
	_ = s.nginx.DeleteIngress(backendProtocolHTTPSIngressName)
}

func (s *BackendConfigSuite) requestServiceUpstream(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), serviceUpstreamTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), serviceUpstreamNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *BackendConfigSuite) requestNoServiceUpstream(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), noServiceUpstreamTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), noServiceUpstreamNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *BackendConfigSuite) requestServiceUpstreamFalse(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), serviceUpstreamFalseTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), serviceUpstreamFalseNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *BackendConfigSuite) requestBackendProtocolHTTP(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), backendProtocolHTTPTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), backendProtocolHTTPNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *BackendConfigSuite) requestNoBackendProtocol(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), noBackendProtocolTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), noBackendProtocolNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- service-upstream tests ---

func (s *BackendConfigSuite) TestServiceUpstreamReturnsOK() {
	traefikResp, nginxResp := s.requestServiceUpstream(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with service-upstream=true")
}

func (s *BackendConfigSuite) TestNoServiceUpstreamReturnsOK() {
	traefikResp, nginxResp := s.requestNoServiceUpstream(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 without service-upstream")
}

func (s *BackendConfigSuite) TestServiceUpstreamFalseReturnsOK() {
	traefikResp, nginxResp := s.requestServiceUpstreamFalse(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with service-upstream=false")
}

func (s *BackendConfigSuite) TestServiceUpstreamOnSubpath() {
	traefikResp, nginxResp := s.requestServiceUpstream(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 on subpath with service-upstream")
}

func (s *BackendConfigSuite) TestServiceUpstreamPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "service-upstream"}
	traefikResp, nginxResp := s.requestServiceUpstream(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), "service-upstream", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header with service-upstream")
	assert.Equal(s.T(), "service-upstream", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header with service-upstream")
}

func (s *BackendConfigSuite) TestServiceUpstreamAndNoUpstreamBothServe() {
	traefikUpstream, _ := s.requestServiceUpstream(http.MethodGet, "/", nil)
	traefikNoUpstream, _ := s.requestNoServiceUpstream(http.MethodGet, "/", nil)

	assert.Equal(s.T(), http.StatusOK, traefikUpstream.StatusCode,
		"service-upstream should return 200")
	assert.Equal(s.T(), http.StatusOK, traefikNoUpstream.StatusCode,
		"no service-upstream should return 200")
	assert.NotEmpty(s.T(), traefikUpstream.Body, "service-upstream should return a body")
	assert.NotEmpty(s.T(), traefikNoUpstream.Body, "no service-upstream should return a body")
}

func (s *BackendConfigSuite) TestServiceUpstreamFalseMatchesNoAnnotation() {
	traefikFalse, nginxFalse := s.requestServiceUpstreamFalse(http.MethodGet, "/", nil)
	traefikNone, nginxNone := s.requestNoServiceUpstream(http.MethodGet, "/", nil)

	assert.Equal(s.T(), traefikNone.StatusCode, traefikFalse.StatusCode,
		"traefik: service-upstream=false should match no annotation")
	assert.Equal(s.T(), nginxNone.StatusCode, nginxFalse.StatusCode,
		"nginx: service-upstream=false should match no annotation")
}

// --- backend-protocol tests ---

func (s *BackendConfigSuite) TestBackendProtocolHTTPReturnsOK() {
	traefikResp, nginxResp := s.requestBackendProtocolHTTP(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with backend-protocol=HTTP")
}

func (s *BackendConfigSuite) TestNoBackendProtocolReturnsOK() {
	traefikResp, nginxResp := s.requestNoBackendProtocol(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 without backend-protocol")
}

func (s *BackendConfigSuite) TestBackendProtocolHTTPMatchesDefault() {
	traefikHTTP, nginxHTTP := s.requestBackendProtocolHTTP(http.MethodGet, "/", nil)
	traefikNone, nginxNone := s.requestNoBackendProtocol(http.MethodGet, "/", nil)

	assert.Equal(s.T(), traefikNone.StatusCode, traefikHTTP.StatusCode,
		"traefik: explicit HTTP should match default")
	assert.Equal(s.T(), nginxNone.StatusCode, nginxHTTP.StatusCode,
		"nginx: explicit HTTP should match default")
}

func (s *BackendConfigSuite) TestBackendProtocolHTTPOnSubpath() {
	traefikResp, nginxResp := s.requestBackendProtocolHTTP(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 on subpath with backend-protocol=HTTP")
}

func (s *BackendConfigSuite) TestBackendProtocolHTTPPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "backend-protocol-http"}
	traefikResp, nginxResp := s.requestBackendProtocolHTTP(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), "backend-protocol-http", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header")
	assert.Equal(s.T(), "backend-protocol-http", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header")
}

func (s *BackendConfigSuite) TestBackendProtocolHTTPSFailsConsistently() {
	// HTTPS against an HTTP-only backend should fail on both controllers.
	traefikResp := s.traefik.MakeRequest(s.T(), backendProtocolHTTPSTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	nginxResp := s.nginx.MakeRequest(s.T(), backendProtocolHTTPSNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)

	if traefikResp != nil && nginxResp != nil {
		// Both should return an error status (5xx class).
		assert.GreaterOrEqual(s.T(), traefikResp.StatusCode, 400,
			"traefik should return error when using HTTPS against HTTP backend")
		assert.GreaterOrEqual(s.T(), nginxResp.StatusCode, 400,
			"nginx should return error when using HTTPS against HTTP backend")
	}
}
