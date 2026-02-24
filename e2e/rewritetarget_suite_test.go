package e2e

import (
	"fmt"
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

	// Wait for ingresses to be ready by polling the actual paths.
	s.waitForRewriteIngressReady(s.traefik, rewriteTraefikHost, "/app")
	s.waitForRewriteIngressReady(s.nginx, rewriteNginxHost, "/app")
	s.waitForRewriteIngressReady(s.traefik, rewriteCaptureTraefikHost, "/api/healthz")
	s.waitForRewriteIngressReady(s.nginx, rewriteCaptureNginxHost, "/api/healthz")
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
