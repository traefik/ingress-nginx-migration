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
	rateLimitRPSIngressName = "ratelimit-rps-test"
	rateLimitRPSTraefikHost = rateLimitRPSIngressName + ".traefik.local"
	rateLimitRPSNginxHost   = rateLimitRPSIngressName + ".nginx.local"

	rateLimitRPSExceedIngressName = "ratelimit-rps-exceed-test"
	rateLimitRPSExceedTraefikHost = rateLimitRPSExceedIngressName + ".traefik.local"
	rateLimitRPSExceedNginxHost   = rateLimitRPSExceedIngressName + ".nginx.local"

	rateLimitRPMIngressName = "ratelimit-rpm-test"
	rateLimitRPMTraefikHost = rateLimitRPMIngressName + ".traefik.local"
	rateLimitRPMNginxHost   = rateLimitRPMIngressName + ".nginx.local"

	rateLimitRPMExceedIngressName = "ratelimit-rpm-exceed-test"
	rateLimitRPMExceedTraefikHost = rateLimitRPMExceedIngressName + ".traefik.local"
	rateLimitRPMExceedNginxHost   = rateLimitRPMExceedIngressName + ".nginx.local"

	rateLimitSubpathIngressName = "ratelimit-subpath-test"
	rateLimitSubpathTraefikHost = rateLimitSubpathIngressName + ".traefik.local"
	rateLimitSubpathNginxHost   = rateLimitSubpathIngressName + ".nginx.local"

	rateLimitHeadersIngressName = "ratelimit-headers-test"
	rateLimitHeadersTraefikHost = rateLimitHeadersIngressName + ".traefik.local"
	rateLimitHeadersNginxHost   = rateLimitHeadersIngressName + ".nginx.local"

	rateLimitBothIngressName = "ratelimit-both-test"
	rateLimitBothTraefikHost = rateLimitBothIngressName + ".traefik.local"
	rateLimitBothNginxHost   = rateLimitBothIngressName + ".nginx.local"
)

type RateLimitSuite struct {
	BaseSuite
}

func TestRateLimitSuite(t *testing.T) {
	suite.Run(t, new(RateLimitSuite))
}

func (s *RateLimitSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// 1. RPS normal: limit-rps=10 (generous limit, single request should pass).
	rpsAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rps": "10",
	}

	err := s.traefik.DeployIngress(rateLimitRPSIngressName, rateLimitRPSTraefikHost, rpsAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rps ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitRPSIngressName, rateLimitRPSNginxHost, rpsAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rps ingress to nginx cluster")

	// 2. RPS exceed: limit-rps=1 (very low limit to trigger rate limiting).
	rpsExceedAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rps": "1",
	}

	err = s.traefik.DeployIngress(rateLimitRPSExceedIngressName, rateLimitRPSExceedTraefikHost, rpsExceedAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rps-exceed ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitRPSExceedIngressName, rateLimitRPSExceedNginxHost, rpsExceedAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rps-exceed ingress to nginx cluster")

	// 3. RPM normal: limit-rpm=60 (generous limit, single request should pass).
	rpmAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rpm": "60",
	}

	err = s.traefik.DeployIngress(rateLimitRPMIngressName, rateLimitRPMTraefikHost, rpmAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rpm ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitRPMIngressName, rateLimitRPMNginxHost, rpmAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rpm ingress to nginx cluster")

	// 4. RPM exceed: limit-rpm=1 (very low limit to trigger rate limiting).
	rpmExceedAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rpm": "1",
	}

	err = s.traefik.DeployIngress(rateLimitRPMExceedIngressName, rateLimitRPMExceedTraefikHost, rpmExceedAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rpm-exceed ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitRPMExceedIngressName, rateLimitRPMExceedNginxHost, rpmExceedAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-rpm-exceed ingress to nginx cluster")

	// 5. Subpath: limit-rps=10, tested on a subpath.
	subpathAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rps": "10",
	}

	err = s.traefik.DeployIngress(rateLimitSubpathIngressName, rateLimitSubpathTraefikHost, subpathAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-subpath ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitSubpathIngressName, rateLimitSubpathNginxHost, subpathAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-subpath ingress to nginx cluster")

	// 6. Headers: limit-rps=10, verify custom headers pass through.
	headersAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rps": "10",
	}

	err = s.traefik.DeployIngress(rateLimitHeadersIngressName, rateLimitHeadersTraefikHost, headersAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-headers ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitHeadersIngressName, rateLimitHeadersNginxHost, headersAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-headers ingress to nginx cluster")

	// 7. Both RPS and RPM: limit-rps=10, limit-rpm=60.
	bothAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/limit-rps": "10",
		"nginx.ingress.kubernetes.io/limit-rpm": "60",
	}

	err = s.traefik.DeployIngress(rateLimitBothIngressName, rateLimitBothTraefikHost, bothAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-both ingress to traefik cluster")

	err = s.nginx.DeployIngress(rateLimitBothIngressName, rateLimitBothNginxHost, bothAnnotations)
	require.NoError(s.T(), err, "deploy ratelimit-both ingress to nginx cluster")

	// Wait for all ingresses to be ready.
	s.traefik.WaitForIngressReady(s.T(), rateLimitRPSTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitRPSNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), rateLimitRPSExceedTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitRPSExceedNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), rateLimitRPMTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitRPMNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), rateLimitRPMExceedTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitRPMExceedNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), rateLimitSubpathTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitSubpathNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), rateLimitHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitHeadersNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), rateLimitBothTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rateLimitBothNginxHost, 20, 1*time.Second)
}

func (s *RateLimitSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(rateLimitRPSIngressName)
	_ = s.nginx.DeleteIngress(rateLimitRPSIngressName)
	_ = s.traefik.DeleteIngress(rateLimitRPSExceedIngressName)
	_ = s.nginx.DeleteIngress(rateLimitRPSExceedIngressName)
	_ = s.traefik.DeleteIngress(rateLimitRPMIngressName)
	_ = s.nginx.DeleteIngress(rateLimitRPMIngressName)
	_ = s.traefik.DeleteIngress(rateLimitRPMExceedIngressName)
	_ = s.nginx.DeleteIngress(rateLimitRPMExceedIngressName)
	_ = s.traefik.DeleteIngress(rateLimitSubpathIngressName)
	_ = s.nginx.DeleteIngress(rateLimitSubpathIngressName)
	_ = s.traefik.DeleteIngress(rateLimitHeadersIngressName)
	_ = s.nginx.DeleteIngress(rateLimitHeadersIngressName)
	_ = s.traefik.DeleteIngress(rateLimitBothIngressName)
	_ = s.nginx.DeleteIngress(rateLimitBothIngressName)
}

func (s *RateLimitSuite) requestRPS(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rateLimitRPSTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rateLimitRPSNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RateLimitSuite) requestRPM(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rateLimitRPMTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rateLimitRPMNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RateLimitSuite) requestBoth(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rateLimitBothTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rateLimitBothNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// TestRPSNormalRequest verifies that a single request under the rate limit returns 200.
func (s *RateLimitSuite) TestRPSNormalRequest() {
	traefikResp, nginxResp := s.requestRPS(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 under RPS limit")
}

// TestRPSExceedLimit verifies that exceeding the rate limit triggers rate limiting.
// Note: nginx returns 503 for rate-limited requests, while Traefik returns 429.
func (s *RateLimitSuite) TestRPSExceedLimit() {
	var traefikRateLimited bool
	var nginxRateLimited bool

	// Send rapid requests to exceed the limit-rps=1 threshold.
	for i := 0; i < 10; i++ {
		traefikResp := s.traefik.MakeRequest(s.T(), rateLimitRPSExceedTraefikHost, http.MethodGet, "/", nil, 1, 0)
		require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
		if traefikResp.StatusCode == http.StatusTooManyRequests {
			traefikRateLimited = true
		}

		nginxResp := s.nginx.MakeRequest(s.T(), rateLimitRPSExceedNginxHost, http.MethodGet, "/", nil, 1, 0)
		require.NotNil(s.T(), nginxResp, "nginx response should not be nil")
		// nginx uses 503 Service Temporarily Unavailable for rate limiting.
		if nginxResp.StatusCode == http.StatusServiceUnavailable {
			nginxRateLimited = true
		}
	}

	// Traefik returns 429 Too Many Requests when rate limited.
	assert.True(s.T(), traefikRateLimited, "traefik should return 429 when exceeding RPS limit")
	// nginx returns 503 Service Unavailable when rate limited.
	assert.True(s.T(), nginxRateLimited, "nginx should return 503 when exceeding RPS limit")
}

// TestRPMNormalRequest verifies that a single request under the RPM limit returns 200.
func (s *RateLimitSuite) TestRPMNormalRequest() {
	traefikResp, nginxResp := s.requestRPM(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 under RPM limit")
}

// TestRPMExceedLimit verifies that exceeding the RPM limit triggers rate limiting.
// Note: nginx returns 503 for rate-limited requests, while Traefik returns 429.
func (s *RateLimitSuite) TestRPMExceedLimit() {
	var traefikRateLimited bool
	var nginxRateLimited bool

	// Send rapid requests to exceed the limit-rpm=1 threshold.
	// 30 requests ensures we exceed nginx's burst (calculated as round(rpm/60*5) ≥ 1).
	for i := 0; i < 30; i++ {
		traefikResp := s.traefik.MakeRequest(s.T(), rateLimitRPMExceedTraefikHost, http.MethodGet, "/", nil, 1, 0)
		require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
		if traefikResp.StatusCode == http.StatusTooManyRequests {
			traefikRateLimited = true
		}

		nginxResp := s.nginx.MakeRequest(s.T(), rateLimitRPMExceedNginxHost, http.MethodGet, "/", nil, 1, 0)
		require.NotNil(s.T(), nginxResp, "nginx response should not be nil")
		// nginx uses 503 Service Temporarily Unavailable for rate limiting.
		if nginxResp.StatusCode == http.StatusServiceUnavailable {
			nginxRateLimited = true
		}
	}

	// Traefik returns 429 Too Many Requests when rate limited.
	assert.True(s.T(), traefikRateLimited, "traefik should return 429 when exceeding RPM limit")
	// nginx returns 503 Service Unavailable when rate limited.
	assert.True(s.T(), nginxRateLimited, "nginx should return 503 when exceeding RPM limit")
}

// TestRPSOnSubpath verifies that rate limiting works on subpaths.
func (s *RateLimitSuite) TestRPSOnSubpath() {
	traefikResp := s.traefik.MakeRequest(s.T(), rateLimitSubpathTraefikHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), rateLimitSubpathNginxHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath under RPS limit")
}

// TestRPSPreservesHeaders verifies that custom request headers pass through when rate limiting is configured.
func (s *RateLimitSuite) TestRPSPreservesHeaders() {
	headers := map[string]string{"X-Custom-Test": "ratelimit-test"}

	traefikResp := s.traefik.MakeRequest(s.T(), rateLimitHeadersTraefikHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), rateLimitHeadersNginxHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")
	assert.Equal(s.T(), "ratelimit-test", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header")
	assert.Equal(s.T(), "ratelimit-test", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header")
}

// TestBothRPSAndRPM verifies that setting both limit-rps and limit-rpm works correctly.
func (s *RateLimitSuite) TestBothRPSAndRPM() {
	traefikResp, nginxResp := s.requestBoth(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with both RPS and RPM limits set")
}
