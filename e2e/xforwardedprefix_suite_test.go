package e2e

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	xfpIngressName = "x-forwarded-prefix-test"
	xfpTraefikHost = xfpIngressName + ".traefik.local"
	xfpNginxHost   = xfpIngressName + ".nginx.local"

	xfpNoAnnotIngressName = "x-forwarded-prefix-no-annot-test"
	xfpNoAnnotTraefikHost = xfpNoAnnotIngressName + ".traefik.local"
	xfpNoAnnotNginxHost   = xfpNoAnnotIngressName + ".nginx.local"

	xfpNestedIngressName = "x-forwarded-prefix-nested-test"
	xfpNestedTraefikHost = xfpNestedIngressName + ".traefik.local"
	xfpNestedNginxHost   = xfpNestedIngressName + ".nginx.local"

	xfpGatewayHost        = "x-forwarded-prefix-test.gateway.local"
	xfpNoAnnotGatewayHost = "x-forwarded-prefix-no-annot-test.gateway.local"
	xfpNestedGatewayHost  = "x-forwarded-prefix-nested-test.gateway.local"
)

type XForwardedPrefixSuite struct {
	BaseSuite
}

func TestXForwardedPrefixSuite(t *testing.T) {
	suite.Run(t, new(XForwardedPrefixSuite))
}

func (s *XForwardedPrefixSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress with x-forwarded-prefix set to "/api".
	// Traefik requires rewrite-target to activate x-forwarded-prefix processing.
	xfpAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/x-forwarded-prefix": "/api",
		"nginx.ingress.kubernetes.io/rewrite-target":     "/$1",
		"nginx.ingress.kubernetes.io/use-regex":          "true",
	}

	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        xfpIngressName,
		Host:        xfpTraefikHost,
		Path:        "/(.*)",
		PathType:    "ImplementationSpecific",
		Annotations: xfpAnnotations,
	})
	require.NoError(s.T(), err, "deploy x-forwarded-prefix ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        xfpIngressName,
		Host:        xfpNginxHost,
		Path:        "/(.*)",
		PathType:    "ImplementationSpecific",
		Annotations: xfpAnnotations,
	})
	require.NoError(s.T(), err, "deploy x-forwarded-prefix ingress to nginx cluster")

	// Ingress without x-forwarded-prefix annotation.
	err = s.traefik.DeployIngress(xfpNoAnnotIngressName, xfpNoAnnotTraefikHost, nil)
	require.NoError(s.T(), err, "deploy no-annotation ingress to traefik cluster")

	err = s.nginx.DeployIngress(xfpNoAnnotIngressName, xfpNoAnnotNginxHost, nil)
	require.NoError(s.T(), err, "deploy no-annotation ingress to nginx cluster")

	// Ingress with nested x-forwarded-prefix set to "/api/v1".
	xfpNestedAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/x-forwarded-prefix": "/api/v1",
		"nginx.ingress.kubernetes.io/rewrite-target":     "/$1",
		"nginx.ingress.kubernetes.io/use-regex":          "true",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        xfpNestedIngressName,
		Host:        xfpNestedTraefikHost,
		Path:        "/(.*)",
		PathType:    "ImplementationSpecific",
		Annotations: xfpNestedAnnotations,
	})
	require.NoError(s.T(), err, "deploy nested x-forwarded-prefix ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        xfpNestedIngressName,
		Host:        xfpNestedNginxHost,
		Path:        "/(.*)",
		PathType:    "ImplementationSpecific",
		Annotations: xfpNestedAnnotations,
	})
	require.NoError(s.T(), err, "deploy nested x-forwarded-prefix ingress to nginx cluster")

	// Deploy Gateway API equivalents.
	gwDir := filepath.Join(fixturesDir, "gateway", "xforwardedprefix")
	for _, f := range []string{"prefix.yaml", "no-annot.yaml", "nested.yaml"} {
		err = s.gateway.DeployGatewayFixture(filepath.Join(gwDir, f))
		require.NoError(s.T(), err, "deploy gateway fixture %s", f)
	}

	s.traefik.WaitForIngressReady(s.T(), xfpTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), xfpNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), xfpNoAnnotTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), xfpNoAnnotNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), xfpNestedTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), xfpNestedNginxHost, 20, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), xfpGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), xfpNoAnnotGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), xfpNestedGatewayHost, 60, 1*time.Second)
}

func (s *XForwardedPrefixSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(xfpIngressName)
	_ = s.nginx.DeleteIngress(xfpIngressName)
	_ = s.traefik.DeleteIngress(xfpNoAnnotIngressName)
	_ = s.nginx.DeleteIngress(xfpNoAnnotIngressName)
	_ = s.traefik.DeleteIngress(xfpNestedIngressName)
	_ = s.nginx.DeleteIngress(xfpNestedIngressName)

	gwDir := filepath.Join(fixturesDir, "gateway", "xforwardedprefix")
	for _, f := range []string{"prefix.yaml", "no-annot.yaml", "nested.yaml"} {
		_ = s.gateway.DeleteGatewayFixture(filepath.Join(gwDir, f))
	}
}

// requestXFP makes the same HTTP request against both clusters using the x-forwarded-prefix ingress.
func (s *XForwardedPrefixSuite) requestXFP(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), xfpTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), xfpNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestNoAnnot makes the same HTTP request against both clusters using the no-annotation ingress.
func (s *XForwardedPrefixSuite) requestNoAnnot(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), xfpNoAnnotTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), xfpNoAnnotNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestNested makes the same HTTP request against both clusters using the nested prefix ingress.
func (s *XForwardedPrefixSuite) requestNested(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), xfpNestedTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), xfpNestedNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *XForwardedPrefixSuite) TestXForwardedPrefixSet() {
	traefikResp, nginxResp := s.requestXFP(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"X-Forwarded-Prefix header should match between controllers",
	)
	assert.Equal(s.T(), "/api", traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"traefik backend should see X-Forwarded-Prefix: /api")
	assert.Equal(s.T(), "/api", nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		"nginx backend should see X-Forwarded-Prefix: /api")

	gatewayResp := s.gateway.MakeRequest(s.T(), xfpGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	assert.Equal(s.T(), "/api", gatewayResp.RequestHeaders["X-Forwarded-Prefix"],
		"gateway backend should see X-Forwarded-Prefix: /api")
}

func (s *XForwardedPrefixSuite) TestNoXForwardedPrefix() {
	traefikResp, nginxResp := s.requestNoAnnot(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"X-Forwarded-Prefix header should match between controllers",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"traefik backend should not see X-Forwarded-Prefix without annotation")
	assert.Empty(s.T(), nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		"nginx backend should not see X-Forwarded-Prefix without annotation")

	gatewayResp := s.gateway.MakeRequest(s.T(), xfpNoAnnotGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	assert.Empty(s.T(), gatewayResp.RequestHeaders["X-Forwarded-Prefix"],
		"gateway backend should not see X-Forwarded-Prefix without annotation")
}

func (s *XForwardedPrefixSuite) TestXForwardedPrefixOnSubpath() {
	traefikResp, nginxResp := s.requestXFP(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"X-Forwarded-Prefix on subpath should match between controllers",
	)
	assert.Equal(s.T(), "/api", traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"traefik backend should see X-Forwarded-Prefix: /api on subpath")
	assert.Equal(s.T(), "/api", nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		"nginx backend should see X-Forwarded-Prefix: /api on subpath")

	gatewayResp := s.gateway.MakeRequest(s.T(), xfpGatewayHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	assert.Equal(s.T(), "/api", gatewayResp.RequestHeaders["X-Forwarded-Prefix"],
		"gateway backend should see X-Forwarded-Prefix: /api on subpath")
}

func (s *XForwardedPrefixSuite) TestXForwardedPrefixPreservesHeaders() {
	traefikResp, nginxResp := s.requestXFP(http.MethodGet, "/", map[string]string{
		"X-Custom-Header": "test-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Verify X-Forwarded-Prefix is present.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"X-Forwarded-Prefix should match between controllers",
	)
	assert.Equal(s.T(), "/api", traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"traefik backend should see X-Forwarded-Prefix: /api")

	// Verify custom header is also forwarded.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Custom-Header"],
		traefikResp.RequestHeaders["X-Custom-Header"],
		"custom header should be forwarded to backend",
	)
	assert.Equal(s.T(), "test-value", traefikResp.RequestHeaders["X-Custom-Header"],
		"custom header value should be preserved")

	gatewayResp := s.gateway.MakeRequest(s.T(), xfpGatewayHost, http.MethodGet, "/", map[string]string{"X-Custom-Header": "test-value"}, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	assert.Equal(s.T(), "/api", gatewayResp.RequestHeaders["X-Forwarded-Prefix"],
		"gateway backend should see X-Forwarded-Prefix: /api")
}

func (s *XForwardedPrefixSuite) TestXForwardedPrefixNested() {
	traefikResp, nginxResp := s.requestNested(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"X-Forwarded-Prefix with nested prefix should match between controllers",
	)
	assert.Equal(s.T(), "/api/v1", traefikResp.RequestHeaders["X-Forwarded-Prefix"],
		"traefik backend should see X-Forwarded-Prefix: /api/v1")
	assert.Equal(s.T(), "/api/v1", nginxResp.RequestHeaders["X-Forwarded-Prefix"],
		"nginx backend should see X-Forwarded-Prefix: /api/v1")

	gatewayResp := s.gateway.MakeRequest(s.T(), xfpNestedGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	assert.Equal(s.T(), "/api/v1", gatewayResp.RequestHeaders["X-Forwarded-Prefix"],
		"gateway backend should see X-Forwarded-Prefix: /api/v1")
}
