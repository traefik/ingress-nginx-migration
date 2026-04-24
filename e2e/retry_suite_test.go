package e2e

import (
	"bytes"
	"fmt"
	"io"
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

	retryBodyLimitIngressName = "retry-body-limit-test"
	retryBodyLimitTraefikHost = retryBodyLimitIngressName + ".traefik.local"
	retryBodyLimitNginxHost   = retryBodyLimitIngressName + ".nginx.local"

	retryFlakyOnIngressName  = "retry-flaky-on-test"
	retryFlakyOnTraefikHost  = retryFlakyOnIngressName + ".traefik.local"
	retryFlakyOnNginxHost    = retryFlakyOnIngressName + ".nginx.local"
	retryFlakyOffIngressName = "retry-flaky-off-test"
	retryFlakyOffTraefikHost = retryFlakyOffIngressName + ".traefik.local"
	retryFlakyOffNginxHost   = retryFlakyOffIngressName + ".nginx.local"
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

	// 6. Retry + proxy-body-size: body-size enforcement precedes retry.
	bodyLimitAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream":       "error timeout http_503",
		"nginx.ingress.kubernetes.io/proxy-next-upstream-tries": "3",
		"nginx.ingress.kubernetes.io/proxy-body-size":           "1k",
	}

	err = s.traefik.DeployIngress(retryBodyLimitIngressName, retryBodyLimitTraefikHost, bodyLimitAnnotations)
	require.NoError(s.T(), err, "deploy retry-body-limit ingress to traefik cluster")

	err = s.nginx.DeployIngress(retryBodyLimitIngressName, retryBodyLimitNginxHost, bodyLimitAnnotations)
	require.NoError(s.T(), err, "deploy retry-body-limit ingress to nginx cluster")

	// Flaky backend: /flaky?fail=N returns 503 for the first N requests, then 200.
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		err = cluster.ApplyFixture("flaky-backend.yaml")
		require.NoError(s.T(), err, "deploy flaky-backend to %s cluster", cluster.Name)
	}
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		err = waitForDeployment(cluster, cluster.TestNamespace, "flaky-backend")
		require.NoError(s.T(), err, "flaky-backend not ready in %s cluster", cluster.Name)
	}

	// 7. Flaky backend, default buffering.
	flakyOnAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream":       "error timeout http_503",
		"nginx.ingress.kubernetes.io/proxy-next-upstream-tries": "3",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        retryFlakyOnIngressName,
		Host:        retryFlakyOnTraefikHost,
		Annotations: flakyOnAnnotations,
		ServiceName: "flaky-backend",
	})
	require.NoError(s.T(), err, "deploy retry-flaky-on ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        retryFlakyOnIngressName,
		Host:        retryFlakyOnNginxHost,
		Annotations: flakyOnAnnotations,
		ServiceName: "flaky-backend",
	})
	require.NoError(s.T(), err, "deploy retry-flaky-on ingress to nginx cluster")

	// 8. Flaky backend, proxy-request-buffering=off.
	flakyOffAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-next-upstream":       "error timeout http_503",
		"nginx.ingress.kubernetes.io/proxy-next-upstream-tries": "3",
		"nginx.ingress.kubernetes.io/proxy-request-buffering":   "off",
	}

	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        retryFlakyOffIngressName,
		Host:        retryFlakyOffTraefikHost,
		Annotations: flakyOffAnnotations,
		ServiceName: "flaky-backend",
	})
	require.NoError(s.T(), err, "deploy retry-flaky-off ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        retryFlakyOffIngressName,
		Host:        retryFlakyOffNginxHost,
		Annotations: flakyOffAnnotations,
		ServiceName: "flaky-backend",
	})
	require.NoError(s.T(), err, "deploy retry-flaky-off ingress to nginx cluster")

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
	s.traefik.WaitForIngressReady(s.T(), retryBodyLimitTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryBodyLimitNginxHost, 20, 1*time.Second)
	// Flaky-backend pulls openresty/openresty:alpine on a cold cluster;
	// give its ingresses extra headroom on top of the 20s default.
	s.traefik.WaitForIngressReady(s.T(), retryFlakyOnTraefikHost, 60, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryFlakyOnNginxHost, 60, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), retryFlakyOffTraefikHost, 60, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), retryFlakyOffNginxHost, 60, 1*time.Second)
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
	_ = s.traefik.DeleteIngress(retryBodyLimitIngressName)
	_ = s.nginx.DeleteIngress(retryBodyLimitIngressName)
	_ = s.traefik.DeleteIngress(retryFlakyOnIngressName)
	_ = s.nginx.DeleteIngress(retryFlakyOnIngressName)
	_ = s.traefik.DeleteIngress(retryFlakyOffIngressName)
	_ = s.nginx.DeleteIngress(retryFlakyOffIngressName)
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		_ = cluster.Kubectl("delete", "-f", fmt.Sprintf("%s/flaky-backend.yaml", fixturesDir), "-n", cluster.TestNamespace, "--ignore-not-found")
	}
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

// makeRequestWithBody sends a request with a body to the given host against
// the given cluster. Mirrors the helper in proxybodysize_suite_test.go.
func (s *RetrySuite) makeRequestWithBody(c *Cluster, host, method, path string, body []byte) *Response {
	s.T().Helper()

	url := fmt.Sprintf("http://%s:%s%s", c.Host, c.Port, path)
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	require.NoError(s.T(), err)

	req.Host = host
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	// Tolerate read errors on the body — a controller may close the
	// connection after sending the status on rejected/large requests.
	respBody, _ := io.ReadAll(resp.Body)

	return &Response{
		StatusCode:      resp.StatusCode,
		Body:            string(respBody),
		ResponseHeaders: resp.Header,
	}
}

func (s *RetrySuite) TestRetryBodyLimitWithinLimit() {
	body := bytes.Repeat([]byte("R"), 500)

	traefikResp := s.makeRequestWithBody(s.traefik, retryBodyLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, retryBodyLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 for body within proxy-body-size=1k")
}

func (s *RetrySuite) TestRetryBodyLimitExceedsLimit() {
	// 2KB body against a 1k limit: nginx rejects with 413 before any
	// retry happens. Traefik's provider should match.
	body := bytes.Repeat([]byte("R"), 2*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, retryBodyLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, retryBodyLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusRequestEntityTooLarge, nginxResp.StatusCode,
		"expected 413 when body exceeds proxy-body-size=1k")
}

func (s *RetrySuite) resetFlaky(c *Cluster, host string) {
	s.T().Helper()
	resp := c.MakeRequest(s.T(), host, http.MethodGet, "/reset", nil, 3, 1*time.Second)
	require.NotNil(s.T(), resp, "flaky /reset should not be nil")
	require.Equal(s.T(), http.StatusOK, resp.StatusCode, "flaky /reset should return 200")
}

func (s *RetrySuite) TestRetryFlakyBufferingOnRecovers() {
	body := bytes.Repeat([]byte("R"), 500)

	s.resetFlaky(s.traefik, retryFlakyOnTraefikHost)
	traefikResp := s.makeRequestWithBody(s.traefik, retryFlakyOnTraefikHost, http.MethodPut, "/flaky?fail=2", body)

	s.resetFlaky(s.nginx, retryFlakyOnNginxHost)
	nginxResp := s.makeRequestWithBody(s.nginx, retryFlakyOnNginxHost, http.MethodPut, "/flaky?fail=2", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status code mismatch — retry should fire with buffering on")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 after 2 failed + 1 successful upstream attempt")
}

func (s *RetrySuite) TestRetryFlakyBufferingOffSuppresses() {
	// 500 KB: above nginx's ~16 KB body buffer (forces streaming) but below
	// traefik's 2 MB retry-middleware buffer.
	body := bytes.Repeat([]byte("R"), 500*1024)

	s.resetFlaky(s.traefik, retryFlakyOffTraefikHost)
	traefikResp := s.makeRequestWithBody(s.traefik, retryFlakyOffTraefikHost, http.MethodPut, "/flaky?fail=2", body)

	s.resetFlaky(s.nginx, retryFlakyOffNginxHost)
	nginxResp := s.makeRequestWithBody(s.nginx, retryFlakyOffNginxHost, http.MethodPut, "/flaky?fail=2", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status code mismatch — traefik should suppress retry when proxy-request-buffering=off")
	assert.Equal(s.T(), http.StatusServiceUnavailable, nginxResp.StatusCode,
		"expected 503 on first-attempt failure with buffering off (retry suppressed)")
}
