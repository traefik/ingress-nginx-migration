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
	rewriteIngressName        = "rewrite-test"
	rewriteCaptureIngressName = "rewrite-capture-test"
	rewriteTraefikHost        = rewriteIngressName + ".traefik.local"
	rewriteNginxHost          = rewriteIngressName + ".nginx.local"
	rewriteCaptureTraefikHost = rewriteCaptureIngressName + ".traefik.local"
	rewriteCaptureNginxHost   = rewriteCaptureIngressName + ".nginx.local"

	noRegexIngressName = "no-regex-test"
	noRegexTraefikHost = noRegexIngressName + ".traefik.local"
	noRegexNginxHost   = noRegexIngressName + ".nginx.local"

	exactPathIngressName = "exact-path-test"
	exactPathTraefikHost = exactPathIngressName + ".traefik.local"
	exactPathNginxHost   = exactPathIngressName + ".nginx.local"

	fullPathNoRegexIngressName = "full-path-test"
	fullPathNoRegexTraefikHost = fullPathNoRegexIngressName + ".traefik.local"
	fullPathNoRegexNginxHost   = fullPathNoRegexIngressName + ".nginx.local"

	fullPathWithPathRegexIngressName = "full-path-regex-test"
	fullPathWithPathRegexTraefikHost = fullPathWithPathRegexIngressName + ".traefik.local"
	fullPathWithPathRegexNginxHost   = fullPathWithPathRegexIngressName + ".nginx.local"
)

type RewriteTargetSuite struct {
	BaseSuite
}

func TestRewriteTargetSuite(t *testing.T) {
	suite.Run(t, new(RewriteTargetSuite))
}

func (s *RewriteTargetSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Simple rewrite ingress: /app(.*) -> /$1
	// use-regex + ImplementationSpecific is required for rewrite-target on nginx-ingress.
	simpleRewriteAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/rewrite-target": "/$2",
		"nginx.ingress.kubernetes.io/use-regex":      "true",
	}

	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        rewriteIngressName,
		Host:        rewriteTraefikHost,
		Annotations: simpleRewriteAnnotations,
		Path:        "/app(/|$)(.*)",
		PathType:    "ImplementationSpecific",
	})
	require.NoError(s.T(), err, "deploy simple rewrite ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        rewriteIngressName,
		Host:        rewriteNginxHost,
		Annotations: simpleRewriteAnnotations,
		Path:        "/app(/|$)(.*)",
		PathType:    "ImplementationSpecific",
	})
	require.NoError(s.T(), err, "deploy simple rewrite ingress to nginx cluster")

	// Capture group rewrite ingress: /api(/|$)(.*) -> /$2
	captureRewriteAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/rewrite-target": "/$2",
		"nginx.ingress.kubernetes.io/use-regex":      "true",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        rewriteCaptureIngressName,
		Host:        rewriteCaptureTraefikHost,
		Annotations: captureRewriteAnnotations,
		Path:        "/api(/|$)(.*)",
		PathType:    "ImplementationSpecific",
	})
	require.NoError(s.T(), err, "deploy capture group rewrite ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        rewriteCaptureIngressName,
		Host:        rewriteCaptureNginxHost,
		Annotations: captureRewriteAnnotations,
		Path:        "/api(/|$)(.*)",
		PathType:    "ImplementationSpecific",
	})
	require.NoError(s.T(), err, "deploy capture group rewrite ingress to nginx cluster")

	// No-regex ingress: rewrite-target is set but use-regex is "false",
	// so the rewrite should NOT take effect.
	noRegexAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/rewrite-target": "/rewritten",
		"nginx.ingress.kubernetes.io/use-regex":      "false",
	}

	err = s.traefik.DeployIngress(noRegexIngressName, noRegexTraefikHost, noRegexAnnotations)
	require.NoError(s.T(), err, "deploy no-regex ingress to traefik cluster")

	err = s.nginx.DeployIngress(noRegexIngressName, noRegexNginxHost, noRegexAnnotations)
	require.NoError(s.T(), err, "deploy no-regex ingress to nginx cluster")

	// Exact path rewrite ingress: /original -> /rewritten (pathType: Exact)
	exactPathAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/rewrite-target": "/rewritten",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        exactPathIngressName,
		Host:        exactPathTraefikHost,
		Annotations: exactPathAnnotations,
		Path:        "/original",
		PathType:    "Exact",
	})
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        exactPathIngressName,
		Host:        exactPathNginxHost,
		Annotations: exactPathAnnotations,
		Path:        "/original",
		PathType:    "Exact",
	})
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to nginx cluster")

	// Full path rewrite ingress: https://bar.example.org/$1 => https://bar.example.org (if no regex in path)
	fullPathAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/rewrite-target": "https://bar.example.org/$1",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        fullPathNoRegexIngressName,
		Host:        fullPathNoRegexTraefikHost,
		Annotations: fullPathAnnotations,
		Path:        "/original",
		PathType:    "Prefix",
	})
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        fullPathNoRegexIngressName,
		Host:        fullPathNoRegexNginxHost,
		Annotations: fullPathAnnotations,
		Path:        "/original",
		PathType:    "Prefix",
	})
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to nginx cluster")

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        fullPathWithPathRegexIngressName,
		Host:        fullPathWithPathRegexTraefikHost,
		Annotations: fullPathAnnotations,
		Path:        "/original/(.*)",
		PathType:    "ImplementationSpecific",
	})
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        fullPathWithPathRegexIngressName,
		Host:        fullPathWithPathRegexNginxHost,
		Annotations: fullPathAnnotations,
		Path:        "/original/(.*)",
		PathType:    "ImplementationSpecific",
	})
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to nginx cluster")

	// Wait for ingresses to be ready by polling the actual paths.
	s.waitForRewriteIngressReady(s.traefik, rewriteTraefikHost, "/app")
	s.waitForRewriteIngressReady(s.nginx, rewriteNginxHost, "/app")
	s.waitForRewriteIngressReady(s.traefik, rewriteCaptureTraefikHost, "/api/healthz")
	s.waitForRewriteIngressReady(s.nginx, rewriteCaptureNginxHost, "/api/healthz")
	s.waitForRewriteIngressReady(s.traefik, noRegexTraefikHost, "/")
	s.waitForRewriteIngressReady(s.nginx, noRegexNginxHost, "/")
	s.waitForRewriteIngressReady(s.traefik, exactPathTraefikHost, "/original")
	s.waitForRewriteIngressReady(s.nginx, exactPathNginxHost, "/original")
	s.waitForRewriteIngressReady(s.traefik, fullPathNoRegexTraefikHost, "/original")
	s.waitForRewriteIngressReady(s.nginx, fullPathNoRegexNginxHost, "/original")
	s.waitForRewriteIngressReady(s.traefik, fullPathWithPathRegexTraefikHost, "/original/health")
	s.waitForRewriteIngressReady(s.nginx, fullPathWithPathRegexNginxHost, "/original/health")
}

// waitForRewriteIngressReady polls the given path until the ingress starts routing requests.
func (s *RewriteTargetSuite) waitForRewriteIngressReady(c *Cluster, host, path string) {
	s.T().Helper()

	for i := 0; i < 20; i++ {
		resp := c.MakeRequest(s.T(), host, http.MethodGet, path, nil, 1, 0)
		if resp != nil && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadGateway {
			return
		}
		time.Sleep(1 * time.Second)
	}
	s.T().Logf("[%s] ingress for host %s path %s not ready after 20 retries", c.Name, host, path)
}

func (s *RewriteTargetSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(rewriteIngressName)
	_ = s.nginx.DeleteIngress(rewriteIngressName)
	_ = s.traefik.DeleteIngress(rewriteCaptureIngressName)
	_ = s.nginx.DeleteIngress(rewriteCaptureIngressName)
	_ = s.traefik.DeleteIngress(noRegexIngressName)
	_ = s.nginx.DeleteIngress(noRegexIngressName)
	_ = s.traefik.DeleteIngress(exactPathIngressName)
	_ = s.nginx.DeleteIngress(exactPathIngressName)
	_ = s.traefik.DeleteIngress(fullPathNoRegexIngressName)
	_ = s.nginx.DeleteIngress(fullPathNoRegexIngressName)
	_ = s.traefik.DeleteIngress(fullPathWithPathRegexIngressName)
	_ = s.nginx.DeleteIngress(fullPathWithPathRegexIngressName)
}

// requestSimple makes the same HTTP request against both clusters using the simple rewrite hosts.
func (s *RewriteTargetSuite) requestSimple(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rewriteTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rewriteNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestCapture makes the same HTTP request against both clusters using the capture group rewrite hosts.
func (s *RewriteTargetSuite) requestCapture(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rewriteCaptureTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rewriteCaptureNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RewriteTargetSuite) TestSimpleRewrite() {
	traefikResp, nginxResp := s.requestSimple(http.MethodGet, "/app", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /app")

	assert.Contains(s.T(), "GET / HTTP/1.1", nginxResp.Body, "nginx backend should see URI /")
	assert.Contains(s.T(), "GET / HTTP/1.1", traefikResp.Body, "traefik backend should see URI /")
}

func (s *RewriteTargetSuite) TestSimpleRewriteSubpath() {
	traefikResp, nginxResp := s.requestSimple(http.MethodGet, "/app/hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /app/hello")

	assert.Contains(s.T(), "GET /hello HTTP/1.1", nginxResp.Body, "nginx backend should see URI /hello")
	assert.Contains(s.T(), "GET /hello HTTP/1.1", traefikResp.Body, "traefik backend should see URI /hello")
}

func (s *RewriteTargetSuite) TestCaptureGroupRewrite() {
	traefikResp, nginxResp := s.requestCapture(http.MethodGet, "/api/users", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /api/users")

	assert.Contains(s.T(), "GET /users HTTP/1.1", nginxResp.Body, "nginx backend should see URI /users")
	assert.Contains(s.T(), "GET /users HTTP/1.1", traefikResp.Body, "traefik backend should see URI /users")
}

func (s *RewriteTargetSuite) TestCaptureGroupRewriteRoot() {
	traefikResp, nginxResp := s.requestCapture(http.MethodGet, "/api/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /api/")

	assert.Contains(s.T(), "GET / HTTP/1.1", nginxResp.Body, "nginx backend should see URI /")
	assert.Contains(s.T(), "GET / HTTP/1.1", traefikResp.Body, "traefik backend should see URI /")
}

func (s *RewriteTargetSuite) TestCaptureGroupRewriteDeepPath() {
	traefikResp, nginxResp := s.requestCapture(http.MethodGet, "/api/v1/users/123", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /api/v1/users/123")

	assert.Contains(s.T(), "GET /v1/users/123 HTTP/1.1", nginxResp.Body, "nginx backend should see URI /v1/users/123")
	assert.Contains(s.T(), "GET /v1/users/123 HTTP/1.1", traefikResp.Body, "traefik backend should see URI /v1/users/123")
}

// requestNoRegex makes the same HTTP request against both clusters using the no-regex ingress hosts.
func (s *RewriteTargetSuite) requestNoRegex(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), noRegexTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), noRegexNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RewriteTargetSuite) TestNoRegexRewriteBehavior() {
	// With use-regex: "false", nginx-ingress still applies rewrite-target
	// (use-regex only controls path matching mode, not whether rewrite fires).
	// Verify both controllers behave the same way.
	traefikResp, nginxResp := s.requestNoRegex(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /")

	// Both should rewrite to /rewritten since rewrite-target applies regardless of use-regex.
	assert.Contains(s.T(), "GET /rewritten HTTP/1.1", nginxResp.Body, "nginx backend should see rewritten URI")
	assert.Contains(s.T(), "GET /rewritten HTTP/1.1", traefikResp.Body, "traefik backend should see rewritten URI")
}

// requestExactPath makes the same HTTP request against both clusters using the exact-path rewrite hosts.
func (s *RewriteTargetSuite) requestExactPath(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), exactPathTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), exactPathNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RewriteTargetSuite) TestExactPathRewrite() {
	traefikResp, nginxResp := s.requestExactPath(http.MethodGet, "/original", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /original")

	assert.Contains(s.T(), "GET /rewritten HTTP/1.1", nginxResp.Body, "nginx backend should see URI /rewritten")
	assert.Contains(s.T(), "GET /rewritten HTTP/1.1", traefikResp.Body, "traefik backend should see URI /rewritten")
}

func (s *RewriteTargetSuite) TestFullPathRewriteNoRegexPath() {
	traefikResp := s.traefik.MakeRequest(s.T(), fullPathNoRegexTraefikHost, http.MethodGet, "/original", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), fullPathNoRegexNginxHost, http.MethodGet, "/original", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302")

	assert.Equal(s.T(), "https://bar.example.org/", nginxResp.ResponseHeaders.Get("Location"), "nginx backend should redirect to rewrite target full URL")
	assert.Equal(s.T(), "https://bar.example.org/", traefikResp.ResponseHeaders.Get("Location"), "traefik backend should redirect to rewrite target full URL")

	traefikResp = s.traefik.MakeRequest(s.T(), fullPathNoRegexTraefikHost, http.MethodGet, "/original/other", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), fullPathNoRegexNginxHost, http.MethodGet, "/original/other", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302")

	assert.Equal(s.T(), "https://bar.example.org/", nginxResp.ResponseHeaders.Get("Location"), "nginx backend should redirect to rewrite target full URL")
	assert.Equal(s.T(), "https://bar.example.org/", traefikResp.ResponseHeaders.Get("Location"), "traefik backend should redirect to rewrite target full URL")

	traefikResp = s.traefik.MakeRequest(s.T(), fullPathNoRegexTraefikHost, http.MethodGet, "/original/a/b/c", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), fullPathNoRegexNginxHost, http.MethodGet, "/original/a/b/c", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302")

	assert.Equal(s.T(), "https://bar.example.org/", nginxResp.ResponseHeaders.Get("Location"), "nginx backend should redirect to rewrite target full URL")
	assert.Equal(s.T(), "https://bar.example.org/", traefikResp.ResponseHeaders.Get("Location"), "traefik backend should redirect to rewrite target full URL")
}

func (s *RewriteTargetSuite) TestFullPathRewriteWithRegexPath() {
	traefikResp := s.traefik.MakeRequest(s.T(), fullPathWithPathRegexTraefikHost, http.MethodGet, "/original", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), fullPathWithPathRegexNginxHost, http.MethodGet, "/original", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302")

	assert.Equal(s.T(), "https://bar.example.org/", nginxResp.ResponseHeaders.Get("Location"), "nginx backend should redirect to rewrite target full URL")
	assert.Equal(s.T(), "https://bar.example.org/", traefikResp.ResponseHeaders.Get("Location"), "traefik backend should redirect to rewrite target full URL")

	traefikResp = s.traefik.MakeRequest(s.T(), fullPathWithPathRegexTraefikHost, http.MethodGet, "/original/other", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), fullPathWithPathRegexNginxHost, http.MethodGet, "/original/other", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302")

	assert.Equal(s.T(), nginxResp.ResponseHeaders.Get("Location"), "https://bar.example.org/other", "nginx backend should redirect to rewrite target full URL")
	assert.Equal(s.T(), traefikResp.ResponseHeaders.Get("Location"), "https://bar.example.org/other", "traefik backend should redirect to rewrite target full URL")

	traefikResp = s.traefik.MakeRequest(s.T(), fullPathWithPathRegexTraefikHost, http.MethodGet, "/original/a/b/c", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), fullPathWithPathRegexNginxHost, http.MethodGet, "/original/a/b/c", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302")

	assert.Equal(s.T(), nginxResp.ResponseHeaders.Get("Location"), "https://bar.example.org/a/b/c", "nginx backend should redirect to rewrite target full URL")
	assert.Equal(s.T(), traefikResp.ResponseHeaders.Get("Location"), "https://bar.example.org/a/b/c", "traefik backend should redirect to rewrite target full URL")
}
