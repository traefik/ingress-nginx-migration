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
	hashByIngressName    = "upstream-hash-by-test"
	hashByTraefikHost    = hashByIngressName + ".traefik.local"
	hashByNginxHost      = hashByIngressName + ".nginx.local"

	noHashByIngressName  = "no-upstream-hash-by-test"
	noHashByTraefikHost  = noHashByIngressName + ".traefik.local"
	noHashByNginxHost    = noHashByIngressName + ".nginx.local"
)

type UpstreamHashBySuite struct {
	BaseSuite
}

func TestUpstreamHashBySuite(t *testing.T) {
	suite.Run(t, new(UpstreamHashBySuite))
}

func (s *UpstreamHashBySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Deploy ingress with upstream-hash-by annotation.
	hashByAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/upstream-hash-by": "$request_uri",
	}

	err := s.traefik.DeployIngress(hashByIngressName, hashByTraefikHost, hashByAnnotations)
	require.NoError(s.T(), err, "deploy upstream-hash-by ingress to traefik cluster")

	err = s.nginx.DeployIngress(hashByIngressName, hashByNginxHost, hashByAnnotations)
	require.NoError(s.T(), err, "deploy upstream-hash-by ingress to nginx cluster")

	// Deploy ingress without upstream-hash-by annotation.
	err = s.traefik.DeployIngress(noHashByIngressName, noHashByTraefikHost, nil)
	require.NoError(s.T(), err, "deploy no-upstream-hash-by ingress to traefik cluster")

	err = s.nginx.DeployIngress(noHashByIngressName, noHashByNginxHost, nil)
	require.NoError(s.T(), err, "deploy no-upstream-hash-by ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), hashByTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), hashByNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), noHashByTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), noHashByNginxHost, 20, 1*time.Second)
}

func (s *UpstreamHashBySuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(hashByIngressName)
	_ = s.nginx.DeleteIngress(hashByIngressName)
	_ = s.traefik.DeleteIngress(noHashByIngressName)
	_ = s.nginx.DeleteIngress(noHashByIngressName)
}

// requestHashBy makes the same HTTP request against both clusters using the upstream-hash-by ingress.
func (s *UpstreamHashBySuite) requestHashBy(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), hashByTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), hashByNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestNoHashBy makes the same HTTP request against both clusters using the no-upstream-hash-by ingress.
func (s *UpstreamHashBySuite) requestNoHashBy(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), noHashByTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), noHashByNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *UpstreamHashBySuite) TestUpstreamHashByReturnsOK() {
	traefikResp, nginxResp := s.requestHashBy(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200")
}

func (s *UpstreamHashBySuite) TestUpstreamHashByConsistentRouting() {
	// Make multiple requests to the same path and verify the response is stable
	// (same backend pod handles all requests for the same URI).
	var traefikHostnames []string
	var nginxHostnames []string

	for i := 0; i < 5; i++ {
		traefikResp, nginxResp := s.requestHashBy(http.MethodGet, "/consistent-test", nil)

		assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik request %d should return 200", i)
		assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx request %d should return 200", i)

		traefikHostnames = append(traefikHostnames, traefikResp.RequestHeaders["Hostname"])
		nginxHostnames = append(nginxHostnames, nginxResp.RequestHeaders["Hostname"])
	}

	// All requests to the same path should hit the same backend.
	for i := 1; i < len(traefikHostnames); i++ {
		assert.Equal(s.T(), traefikHostnames[0], traefikHostnames[i],
			"traefik request %d should hit the same backend as request 0", i)
	}
	for i := 1; i < len(nginxHostnames); i++ {
		assert.Equal(s.T(), nginxHostnames[0], nginxHostnames[i],
			"nginx request %d should hit the same backend as request 0", i)
	}
}

func (s *UpstreamHashBySuite) TestUpstreamHashByOnSubpath() {
	traefikResp, nginxResp := s.requestHashBy(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200")
}

func (s *UpstreamHashBySuite) TestUpstreamHashByPreservesHeaders() {
	traefikResp, nginxResp := s.requestHashBy(http.MethodGet, "/", map[string]string{
		"X-Custom-Test": "hash-by-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Custom-Test"],
		traefikResp.RequestHeaders["X-Custom-Test"],
		"custom header passthrough mismatch",
	)
	assert.Equal(s.T(), "hash-by-value", traefikResp.RequestHeaders["X-Custom-Test"],
		"traefik should preserve custom header value")
	assert.Equal(s.T(), "hash-by-value", nginxResp.RequestHeaders["X-Custom-Test"],
		"nginx should preserve custom header value")
}

func (s *UpstreamHashBySuite) TestNoUpstreamHashByReturnsOK() {
	traefikResp, nginxResp := s.requestNoHashBy(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200")
}
