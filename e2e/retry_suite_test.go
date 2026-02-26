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
	retryDefaultIngressName = "retry-default-test"
	retryDefaultTraefikHost = retryDefaultIngressName + ".traefik.local"
	retryDefaultNginxHost   = retryDefaultIngressName + ".nginx.local"

	retryDisabledIngressName = "retry-disabled-test"
	retryDisabledTraefikHost = retryDisabledIngressName + ".traefik.local"
	retryDisabledNginxHost   = retryDisabledIngressName + ".nginx.local"

	retryCustomIngressName = "retry-custom-test"
	retryCustomTraefikHost = retryCustomIngressName + ".traefik.local"
	retryCustomNginxHost   = retryCustomIngressName + ".nginx.local"

	retryTriesIngressName = "retry-tries-test"
	retryTriesTraefikHost = retryTriesIngressName + ".traefik.local"
	retryTriesNginxHost   = retryTriesIngressName + ".nginx.local"

	retryTimeoutIngressName = "retry-timeout-test"
	retryTimeoutTraefikHost = retryTimeoutIngressName + ".traefik.local"
	retryTimeoutNginxHost   = retryTimeoutIngressName + ".nginx.local"
)

type RetrySuite struct {
	BaseSuite
}

func TestRetrySuite(t *testing.T) {
	suite.Run(t, new(RetrySuite))
}

func (s *RetrySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// 1. Default retry: no annotations (uses provider defaults: "error timeout", tries=3).
	defaultAnnotations := map[string]string{}

	err := s.traefik.DeployIngress(retryDefaultIngressName, retryDefaultTraefikHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy retry-default ingress to traefik cluster")

	err = s.nginx.DeployIngress(retryDefaultIngressName, retryDefaultNginxHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy retry-default ingress to nginx cluster")

	// 2. Retry disabled: proxy-next-upstream set to "off".
	disabledAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream": "off",
	}

	err = s.traefik.DeployIngress(retryDisabledIngressName, retryDisabledTraefikHost, disabledAnnotations)
	require.NoError(s.T(), err, "deploy retry-disabled ingress to traefik cluster")

	err = s.nginx.DeployIngress(retryDisabledIngressName, retryDisabledNginxHost, disabledAnnotations)
	require.NoError(s.T(), err, "deploy retry-disabled ingress to nginx cluster")

	// 3. Custom retry: all three annotations set.
	customAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream":         "error timeout http_503",
		"nginx.ingress.kubernetes.io/proxy-next-upstream-tries":   "5",
		"nginx.ingress.kubernetes.io/proxy-next-upstream-timeout": "10",
	}

	err = s.traefik.DeployIngress(retryCustomIngressName, retryCustomTraefikHost, customAnnotations)
	require.NoError(s.T(), err, "deploy retry-custom ingress to traefik cluster")

	err = s.nginx.DeployIngress(retryCustomIngressName, retryCustomNginxHost, customAnnotations)
	require.NoError(s.T(), err, "deploy retry-custom ingress to nginx cluster")

	// 4. Tries only: minimal retries.
	triesAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream-tries": "1",
	}

	err = s.traefik.DeployIngress(retryTriesIngressName, retryTriesTraefikHost, triesAnnotations)
	require.NoError(s.T(), err, "deploy retry-tries ingress to traefik cluster")

	err = s.nginx.DeployIngress(retryTriesIngressName, retryTriesNginxHost, triesAnnotations)
	require.NoError(s.T(), err, "deploy retry-tries ingress to nginx cluster")

	// 5. Timeout only: retry timeout set.
	timeoutAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream-timeout": "5",
	}

	err = s.traefik.DeployIngress(retryTimeoutIngressName, retryTimeoutTraefikHost, timeoutAnnotations)
	require.NoError(s.T(), err, "deploy retry-timeout ingress to traefik cluster")

	err = s.nginx.DeployIngress(retryTimeoutIngressName, retryTimeoutNginxHost, timeoutAnnotations)
	require.NoError(s.T(), err, "deploy retry-timeout ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), retryDefaultTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryDefaultNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), retryDisabledTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryDisabledNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), retryCustomTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryCustomNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), retryTriesTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryTriesNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), retryTimeoutTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryTimeoutNginxHost, 20, 1*time.Second)
}

func (s *RetrySuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(retryDefaultIngressName)
	_ = s.nginx.DeleteIngress(retryDefaultIngressName)
	_ = s.traefik.DeleteIngress(retryDisabledIngressName)
	_ = s.nginx.DeleteIngress(retryDisabledIngressName)
	_ = s.traefik.DeleteIngress(retryCustomIngressName)
	_ = s.nginx.DeleteIngress(retryCustomIngressName)
	_ = s.traefik.DeleteIngress(retryTriesIngressName)
	_ = s.nginx.DeleteIngress(retryTriesIngressName)
	_ = s.traefik.DeleteIngress(retryTimeoutIngressName)
	_ = s.nginx.DeleteIngress(retryTimeoutIngressName)
}

func (s *RetrySuite) requestDefault(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), retryDefaultTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), retryDefaultNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RetrySuite) requestDisabled(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), retryDisabledTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), retryDisabledNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RetrySuite) requestCustom(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), retryCustomTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), retryCustomNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RetrySuite) requestTries(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), retryTriesTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), retryTriesNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RetrySuite) requestTimeout(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), retryTimeoutTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), retryTimeoutNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RetrySuite) TestDefaultRetryStatusMatch() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with default retry config")
}

func (s *RetrySuite) TestDefaultRetryOnSubpath() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath with default retry")
}

func (s *RetrySuite) TestDefaultRetryPOST() {
	traefikResp, nginxResp := s.requestDefault(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with default retry")
}

func (s *RetrySuite) TestRetryDisabledStatusMatch() {
	traefikResp, nginxResp := s.requestDisabled(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with retry disabled")
}

func (s *RetrySuite) TestRetryDisabledOnSubpath() {
	traefikResp, nginxResp := s.requestDisabled(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath with retry disabled")
}

func (s *RetrySuite) TestRetryDisabledPOST() {
	traefikResp, nginxResp := s.requestDisabled(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with retry disabled")
}

func (s *RetrySuite) TestRetryDisabledPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "retry-disabled"}
	traefikResp, nginxResp := s.requestDisabled(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")
	assert.Equal(s.T(), "retry-disabled", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header")
	assert.Equal(s.T(), "retry-disabled", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header")
}

func (s *RetrySuite) TestCustomRetryStatusMatch() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with custom retry config")
}

func (s *RetrySuite) TestCustomRetryOnSubpath() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath with custom retry")
}

func (s *RetrySuite) TestCustomRetryPOST() {
	traefikResp, nginxResp := s.requestCustom(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with custom retry")
}

func (s *RetrySuite) TestCustomRetryPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "custom-retry"}
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), "custom-retry", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header")
	assert.Equal(s.T(), "custom-retry", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header")
}

func (s *RetrySuite) TestRetryTriesStatusMatch() {
	traefikResp, nginxResp := s.requestTries(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with tries=1")
}

func (s *RetrySuite) TestRetryTriesOnSubpath() {
	traefikResp, nginxResp := s.requestTries(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath with tries=1")
}

func (s *RetrySuite) TestRetryTriesPOST() {
	traefikResp, nginxResp := s.requestTries(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with tries=1")
}

func (s *RetrySuite) TestRetryTimeoutStatusMatch() {
	traefikResp, nginxResp := s.requestTimeout(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with retry timeout=5")
}

func (s *RetrySuite) TestRetryTimeoutOnSubpath() {
	traefikResp, nginxResp := s.requestTimeout(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath with retry timeout")
}

func (s *RetrySuite) TestRetryTimeoutPOST() {
	traefikResp, nginxResp := s.requestTimeout(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with retry timeout")
}

func (s *RetrySuite) TestRetryTimeoutPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "retry-timeout"}
	traefikResp, nginxResp := s.requestTimeout(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), "retry-timeout", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header")
	assert.Equal(s.T(), "retry-timeout", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header")
}
