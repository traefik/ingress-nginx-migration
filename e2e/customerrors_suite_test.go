package e2e

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	customErrorsIngressName = "custom-errors-test"
	customErrorsTraefikHost = customErrorsIngressName + ".traefik.local"
	customErrorsNginxHost   = customErrorsIngressName + ".nginx.local"
	customErrorsGatewayHost = "custom-errors-test.gateway.local"

	customErrorsNoBackendIngressName = "custom-errors-no-backend-test"
	customErrorsNoBackendTraefikHost = customErrorsNoBackendIngressName + ".traefik.local"
	customErrorsNoBackendNginxHost   = customErrorsNoBackendIngressName + ".nginx.local"
	customErrorsNoBackendGatewayHost = "custom-errors-no-backend-test.gateway.local"
)

type CustomErrorsSuite struct {
	BaseSuite
}

func TestCustomErrorsSuite(t *testing.T) {
	suite.Run(t, new(CustomErrorsSuite))
}

func (s *CustomErrorsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Deploy status-backend and error-backend to both clusters.
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		err := cluster.ApplyFixture("status-backend.yaml")
		require.NoError(s.T(), err, "deploy status-backend to %s cluster", cluster.Name)

		err = cluster.ApplyFixture("error-backend.yaml")
		require.NoError(s.T(), err, "deploy error-backend to %s cluster", cluster.Name)
	}

	// Wait for backends to be ready.
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		err := waitForDeployment(cluster, cluster.TestNamespace, "status-backend")
		require.NoError(s.T(), err, "status-backend not ready in %s cluster", cluster.Name)

		err = waitForDeployment(cluster, cluster.TestNamespace, "error-backend")
		require.NoError(s.T(), err, "error-backend not ready in %s cluster", cluster.Name)
	}

	// 1. Ingress with custom-http-errors and explicit default-backend.
	customErrorsAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/custom-http-errors": "404,503",
		"nginx.ingress.kubernetes.io/default-backend":    "error-backend",
	}

	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        customErrorsIngressName,
		Host:        customErrorsTraefikHost,
		Annotations: customErrorsAnnotations,
		ServiceName: "status-backend",
	})
	require.NoError(s.T(), err, "deploy custom-errors ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        customErrorsIngressName,
		Host:        customErrorsNginxHost,
		Annotations: customErrorsAnnotations,
		ServiceName: "status-backend",
	})
	require.NoError(s.T(), err, "deploy custom-errors ingress to nginx cluster")

	// 2. Ingress with custom-http-errors but no explicit default-backend.
	noBackendAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/custom-http-errors": "404,503",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        customErrorsNoBackendIngressName,
		Host:        customErrorsNoBackendTraefikHost,
		Annotations: noBackendAnnotations,
		ServiceName: "status-backend",
	})
	require.NoError(s.T(), err, "deploy custom-errors-no-backend ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        customErrorsNoBackendIngressName,
		Host:        customErrorsNoBackendNginxHost,
		Annotations: noBackendAnnotations,
		ServiceName: "status-backend",
	})
	require.NoError(s.T(), err, "deploy custom-errors-no-backend ingress to nginx cluster")

	// Deploy Gateway API equivalents.
	// The gateway cluster shares the same Traefik instance as s.traefik, so
	// status-backend and error-backend are already deployed and ready.
	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "customerrors", "errors.yaml"))
	require.NoError(s.T(), err, "deploy custom-errors gateway fixture")

	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "customerrors", "no-backend.yaml"))
	require.NoError(s.T(), err, "deploy custom-errors-no-backend gateway fixture")

	s.traefik.WaitForIngressReady(s.T(), customErrorsTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), customErrorsNginxHost, 30, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), customErrorsNoBackendTraefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), customErrorsNoBackendNginxHost, 30, 1*time.Second)
	// Gateway API routes need more time — CRD provider must publish middleware config first.
	s.gateway.WaitForIngressReady(s.T(), customErrorsGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), customErrorsNoBackendGatewayHost, 60, 1*time.Second)
}

func (s *CustomErrorsSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(customErrorsIngressName)
	_ = s.nginx.DeleteIngress(customErrorsIngressName)
	_ = s.traefik.DeleteIngress(customErrorsNoBackendIngressName)
	_ = s.nginx.DeleteIngress(customErrorsNoBackendIngressName)

	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "customerrors", "errors.yaml"))
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "customerrors", "no-backend.yaml"))

	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		_ = cluster.Kubectl("delete", "-f", fmt.Sprintf("%s/status-backend.yaml", fixturesDir), "-n", cluster.TestNamespace, "--ignore-not-found")
		_ = cluster.Kubectl("delete", "-f", fmt.Sprintf("%s/error-backend.yaml", fixturesDir), "-n", cluster.TestNamespace, "--ignore-not-found")
	}
}

func (s *CustomErrorsSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), customErrorsTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), customErrorsNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *CustomErrorsSuite) requestNoBackend(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), customErrorsNoBackendTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), customErrorsNoBackendNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *CustomErrorsSuite) TestNonErrorStatusPassesThrough() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when upstream returns 200 (not in custom-http-errors list)")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode,
		"gateway: expected 200 when upstream returns 200 (not in custom-http-errors list)")
}

func (s *CustomErrorsSuite) TestNonErrorBodyPassesThrough() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Contains(s.T(), traefikResp.Body, "status backend OK",
		"traefik should pass through upstream body on non-error status")
	assert.Contains(s.T(), nginxResp.Body, "status backend OK",
		"nginx should pass through upstream body on non-error status")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Contains(s.T(), gatewayResp.Body, "status backend OK",
		"gateway should pass through upstream body on non-error status")
}

func (s *CustomErrorsSuite) Test404TriggersCustomErrorPage() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/not-found", nil)

	assert.Contains(s.T(), traefikResp.Body, "custom error page",
		"traefik should serve custom error page on 404")
	assert.Contains(s.T(), nginxResp.Body, "custom error page",
		"nginx should serve custom error page on 404")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/not-found", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Contains(s.T(), gatewayResp.Body, "custom error page",
		"gateway should serve custom error page on 404")
}

func (s *CustomErrorsSuite) Test503TriggersCustomErrorPage() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/unavailable", nil)

	assert.Contains(s.T(), traefikResp.Body, "custom error page",
		"traefik should serve custom error page on 503")
	assert.Contains(s.T(), nginxResp.Body, "custom error page",
		"nginx should serve custom error page on 503")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/unavailable", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Contains(s.T(), gatewayResp.Body, "custom error page",
		"gateway should serve custom error page on 503")
}

func (s *CustomErrorsSuite) Test404StatusCodePreserved() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/not-found", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status code mismatch between traefik and nginx on 404")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/not-found", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	// MIGRATION NOTE: Gateway API provider preserves the original 404 status code (matching nginx behavior).
	// Traefik's Ingress provider returns the error-backend's status code (200) instead.
	assert.Equal(s.T(), http.StatusNotFound, gatewayResp.StatusCode,
		"gateway migration: 404 status code should be preserved (matches nginx, differs from Traefik ingress)")
}

func (s *CustomErrorsSuite) Test503StatusCodePreserved() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/unavailable", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status code mismatch between traefik and nginx on 503")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/unavailable", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	// MIGRATION NOTE: Gateway API provider preserves the original 503 status code (matching nginx behavior).
	// Traefik's Ingress provider returns the error-backend's status code (200) instead.
	assert.Equal(s.T(), http.StatusServiceUnavailable, gatewayResp.StatusCode,
		"gateway migration: 503 status code should be preserved (matches nginx, differs from Traefik ingress)")
}

func (s *CustomErrorsSuite) TestUnlistedErrorCodeNotIntercepted() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.NotContains(s.T(), traefikResp.Body, "custom error page",
		"traefik should not serve custom error page for 200 response")
	assert.NotContains(s.T(), nginxResp.Body, "custom error page",
		"nginx should not serve custom error page for 200 response")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.NotContains(s.T(), gatewayResp.Body, "custom error page",
		"gateway should not serve custom error page for 200 response")
}

func (s *CustomErrorsSuite) TestNoBackend404() {
	traefikResp, nginxResp := s.requestNoBackend(http.MethodGet, "/not-found", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status code mismatch between traefik and nginx on 404 without explicit default-backend")

	// Gateway has no errors middleware (migration gap: no default-backend), so just compare status codes.
	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsNoBackendGatewayHost, http.MethodGet, "/not-found", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode,
		"gateway migration: status code mismatch on 404 without errors middleware")
}

func (s *CustomErrorsSuite) TestNoBackendNonError() {
	traefikResp, nginxResp := s.requestNoBackend(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 when upstream returns 200")
	assert.Contains(s.T(), traefikResp.Body, "status backend OK",
		"traefik should pass through upstream body on non-error status")
	assert.Contains(s.T(), nginxResp.Body, "status backend OK",
		"nginx should pass through upstream body on non-error status")

	gatewayResp := s.gateway.MakeRequest(s.T(), customErrorsNoBackendGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode,
		"gateway: expected 200 when upstream returns 200")
	assert.Contains(s.T(), gatewayResp.Body, "status backend OK",
		"gateway should pass through upstream body on non-error status")
}

func (s *CustomErrorsSuite) Test404WithDifferentMethods() {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		s.T().Run(method, func(t *testing.T) {
			traefikResp := s.traefik.MakeRequest(t, customErrorsTraefikHost, method, "/not-found", nil, 3, 1*time.Second)
			require.NotNil(t, traefikResp, "traefik response should not be nil")

			nginxResp := s.nginx.MakeRequest(t, customErrorsNginxHost, method, "/not-found", nil, 3, 1*time.Second)
			require.NotNil(t, nginxResp, "nginx response should not be nil")

			assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode,
				"status code mismatch for method %s", method)
			assert.Contains(t, traefikResp.Body, "custom error page",
				"traefik should serve custom error page on 404 for method %s", method)
			assert.Contains(t, nginxResp.Body, "custom error page",
				"nginx should serve custom error page on 404 for method %s", method)

			gatewayResp := s.gateway.MakeRequest(t, customErrorsGatewayHost, method, "/not-found", nil, 3, 1*time.Second)
			require.NotNil(t, gatewayResp, "gateway response should not be nil")
			// MIGRATION NOTE: Gateway API provider preserves the original 404 status code.
			assert.Equal(t, http.StatusNotFound, gatewayResp.StatusCode,
				"gateway migration: 404 status code should be preserved for method %s", method)
			assert.Contains(t, gatewayResp.Body, "custom error page",
				"gateway should serve custom error page on 404 for method %s", method)
		})
	}
}
