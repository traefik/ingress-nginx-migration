package e2e

import (
	"fmt"
	"net/http"
	"strings"
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

	appRootIngressName = "app-root-test"
	appRootTraefikHost = appRootIngressName + ".traefik.local"
	appRootNginxHost   = appRootIngressName + ".nginx.local"

	noRegexIngressName = "no-regex-test"
	noRegexTraefikHost = noRegexIngressName + ".traefik.local"
	noRegexNginxHost   = noRegexIngressName + ".nginx.local"

	exactPathIngressName = "exact-path-test"
	exactPathTraefikHost = exactPathIngressName + ".traefik.local"
	exactPathNginxHost   = exactPathIngressName + ".nginx.local"
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
	simpleRewriteTraefik := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: "/$2"
    nginx.ingress.kubernetes.io/use-regex: "true"
spec:
  ingressClassName: nginx
  rules:
  - host: %s
    http:
      paths:
      - path: /app(/|$)(.*)
        pathType: ImplementationSpecific
        backend:
          service:
            name: snippet-test-backend
            port:
              number: 80
`, rewriteIngressName, rewriteTraefikHost)

	simpleRewriteNginx := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: "/$2"
    nginx.ingress.kubernetes.io/use-regex: "true"
spec:
  ingressClassName: nginx
  rules:
  - host: %s
    http:
      paths:
      - path: /app(/|$)(.*)
        pathType: ImplementationSpecific
        backend:
          service:
            name: snippet-test-backend
            port:
              number: 80
`, rewriteIngressName, rewriteNginxHost)

	err := s.traefik.ApplyManifest(simpleRewriteTraefik)
	require.NoError(s.T(), err, "deploy simple rewrite ingress to traefik cluster")

	err = s.nginx.ApplyManifest(simpleRewriteNginx)
	require.NoError(s.T(), err, "deploy simple rewrite ingress to nginx cluster")

	// Capture group rewrite ingress: /api(/|$)(.*) -> /$2
	captureRewriteTraefik := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: "/$2"
    nginx.ingress.kubernetes.io/use-regex: "true"
spec:
  ingressClassName: nginx
  rules:
  - host: %s
    http:
      paths:
      - path: /api(/|$)(.*)
        pathType: ImplementationSpecific
        backend:
          service:
            name: snippet-test-backend
            port:
              number: 80
`, rewriteCaptureIngressName, rewriteCaptureTraefikHost)

	captureRewriteNginx := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: "/$2"
    nginx.ingress.kubernetes.io/use-regex: "true"
spec:
  ingressClassName: nginx
  rules:
  - host: %s
    http:
      paths:
      - path: /api(/|$)(.*)
        pathType: ImplementationSpecific
        backend:
          service:
            name: snippet-test-backend
            port:
              number: 80
`, rewriteCaptureIngressName, rewriteCaptureNginxHost)

	err = s.traefik.ApplyManifest(captureRewriteTraefik)
	require.NoError(s.T(), err, "deploy capture group rewrite ingress to traefik cluster")

	err = s.nginx.ApplyManifest(captureRewriteNginx)
	require.NoError(s.T(), err, "deploy capture group rewrite ingress to nginx cluster")

	// App-root redirect ingress: / -> /dashboard
	appRootAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/app-root": "/dashboard",
	}

	err = s.traefik.DeployIngress(appRootIngressName, appRootTraefikHost, appRootAnnotations)
	require.NoError(s.T(), err, "deploy app-root ingress to traefik cluster")

	err = s.nginx.DeployIngress(appRootIngressName, appRootNginxHost, appRootAnnotations)
	require.NoError(s.T(), err, "deploy app-root ingress to nginx cluster")

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
	exactPathTraefik := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: "/rewritten"
spec:
  ingressClassName: nginx
  rules:
  - host: %s
    http:
      paths:
      - path: /original
        pathType: Exact
        backend:
          service:
            name: snippet-test-backend
            port:
              number: 80
`, exactPathIngressName, exactPathTraefikHost)

	exactPathNginx := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: "/rewritten"
spec:
  ingressClassName: nginx
  rules:
  - host: %s
    http:
      paths:
      - path: /original
        pathType: Exact
        backend:
          service:
            name: snippet-test-backend
            port:
              number: 80
`, exactPathIngressName, exactPathNginxHost)

	err = s.traefik.ApplyManifest(exactPathTraefik)
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to traefik cluster")

	err = s.nginx.ApplyManifest(exactPathNginx)
	require.NoError(s.T(), err, "deploy exact-path rewrite ingress to nginx cluster")

	// Wait for ingresses to be ready by polling the actual paths.
	s.waitForRewriteIngressReady(s.traefik, rewriteTraefikHost, "/app")
	s.waitForRewriteIngressReady(s.nginx, rewriteNginxHost, "/app")
	s.waitForRewriteIngressReady(s.traefik, rewriteCaptureTraefikHost, "/api/healthz")
	s.waitForRewriteIngressReady(s.nginx, rewriteCaptureNginxHost, "/api/healthz")
	s.waitForRewriteIngressReady(s.traefik, appRootTraefikHost, "/dashboard")
	s.waitForRewriteIngressReady(s.nginx, appRootNginxHost, "/dashboard")
	s.waitForRewriteIngressReady(s.traefik, noRegexTraefikHost, "/")
	s.waitForRewriteIngressReady(s.nginx, noRegexNginxHost, "/")
	s.waitForRewriteIngressReady(s.traefik, exactPathTraefikHost, "/original")
	s.waitForRewriteIngressReady(s.nginx, exactPathNginxHost, "/original")
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
	_ = s.traefik.Kubectl("delete", "ingress", rewriteIngressName, "-n", s.traefik.TestNamespace, "--ignore-not-found")
	_ = s.nginx.Kubectl("delete", "ingress", rewriteIngressName, "-n", s.nginx.TestNamespace, "--ignore-not-found")
	_ = s.traefik.Kubectl("delete", "ingress", rewriteCaptureIngressName, "-n", s.traefik.TestNamespace, "--ignore-not-found")
	_ = s.nginx.Kubectl("delete", "ingress", rewriteCaptureIngressName, "-n", s.nginx.TestNamespace, "--ignore-not-found")
	_ = s.traefik.DeleteIngress(appRootIngressName)
	_ = s.nginx.DeleteIngress(appRootIngressName)
	_ = s.traefik.DeleteIngress(noRegexIngressName)
	_ = s.nginx.DeleteIngress(noRegexIngressName)
	_ = s.traefik.DeleteIngress(exactPathIngressName)
	_ = s.nginx.DeleteIngress(exactPathIngressName)
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

	assert.Contains(s.T(), nginxResp.Body, "GET / HTTP/1.1", "nginx backend should see URI /")
	assert.Contains(s.T(), traefikResp.Body, "GET / HTTP/1.1", "traefik backend should see URI /")
}

func (s *RewriteTargetSuite) TestSimpleRewriteSubpath() {
	traefikResp, nginxResp := s.requestSimple(http.MethodGet, "/app/hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /app/hello")

	assert.Contains(s.T(), nginxResp.Body, "GET /hello HTTP/1.1", "nginx backend should see URI /hello")
	assert.Contains(s.T(), traefikResp.Body, "GET /hello HTTP/1.1", "traefik backend should see URI /hello")
}

func (s *RewriteTargetSuite) TestCaptureGroupRewrite() {
	traefikResp, nginxResp := s.requestCapture(http.MethodGet, "/api/users", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /api/users")

	assert.Contains(s.T(), nginxResp.Body, "GET /users HTTP/1.1", "nginx backend should see URI /users")
	assert.Contains(s.T(), traefikResp.Body, "GET /users HTTP/1.1", "traefik backend should see URI /users")
}

func (s *RewriteTargetSuite) TestCaptureGroupRewriteRoot() {
	traefikResp, nginxResp := s.requestCapture(http.MethodGet, "/api/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /api/")

	assert.Contains(s.T(), nginxResp.Body, "GET / HTTP/1.1", "nginx backend should see URI /")
	assert.Contains(s.T(), traefikResp.Body, "GET / HTTP/1.1", "traefik backend should see URI /")
}

func (s *RewriteTargetSuite) TestCaptureGroupRewriteDeepPath() {
	traefikResp, nginxResp := s.requestCapture(http.MethodGet, "/api/v1/users/123", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for /api/v1/users/123")

	assert.Contains(s.T(), nginxResp.Body, "GET /v1/users/123 HTTP/1.1", "nginx backend should see URI /v1/users/123")
	assert.Contains(s.T(), traefikResp.Body, "GET /v1/users/123 HTTP/1.1", "traefik backend should see URI /v1/users/123")
}

// requestAppRoot makes the same HTTP request against both clusters using the app-root hosts.
func (s *RewriteTargetSuite) requestAppRoot(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), appRootTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), appRootNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RewriteTargetSuite) TestAppRootRedirect() {
	traefikResp, nginxResp := s.requestAppRoot(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	assert.True(s.T(), strings.HasSuffix(traefikLocation, "/dashboard"),
		"traefik Location header should end with /dashboard, got: %s", traefikLocation)
}

func (s *RewriteTargetSuite) TestAppRootRedirectLocation() {
	traefikResp, nginxResp := s.requestAppRoot(http.MethodGet, "/", nil)

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.True(s.T(), strings.HasSuffix(traefikLocation, "/dashboard"),
		"traefik Location header should end with /dashboard, got: %s", traefikLocation)
	assert.True(s.T(), strings.HasSuffix(nginxLocation, "/dashboard"),
		"nginx Location header should end with /dashboard, got: %s", nginxLocation)
}

func (s *RewriteTargetSuite) TestAppRootNonRootPassthrough() {
	traefikResp, nginxResp := s.requestAppRoot(http.MethodGet, "/other", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for non-root path /other")
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
	assert.Contains(s.T(), nginxResp.Body, "GET /rewritten HTTP/1.1", "nginx backend should see rewritten URI")
	assert.Contains(s.T(), traefikResp.Body, "GET /rewritten HTTP/1.1", "traefik backend should see rewritten URI")
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

	assert.Contains(s.T(), nginxResp.Body, "GET /rewritten HTTP/1.1", "nginx backend should see URI /rewritten")
	assert.Contains(s.T(), traefikResp.Body, "GET /rewritten HTTP/1.1", "traefik backend should see URI /rewritten")
}

