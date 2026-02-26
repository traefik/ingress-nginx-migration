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
	upstreamVhostIngressName = "upstream-vhost-test"
	upstreamVhostTraefikHost = upstreamVhostIngressName + ".traefik.local"
	upstreamVhostNginxHost   = upstreamVhostIngressName + ".nginx.local"

	noVhostIngressName = "no-vhost-test"
	noVhostTraefikHost = noVhostIngressName + ".traefik.local"
	noVhostNginxHost   = noVhostIngressName + ".nginx.local"

	customUpstreamHost = "custom.backend.internal"
)

type UpstreamVhostSuite struct {
	BaseSuite
}

func TestUpstreamVhostSuite(t *testing.T) {
	suite.Run(t, new(UpstreamVhostSuite))
}

func (s *UpstreamVhostSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress with upstream-vhost annotation to override Host header sent to backend.
	vhostAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/upstream-vhost": customUpstreamHost,
	}

	err := s.traefik.DeployIngress(upstreamVhostIngressName, upstreamVhostTraefikHost, vhostAnnotations)
	require.NoError(s.T(), err, "deploy upstream-vhost ingress to traefik cluster")

	err = s.nginx.DeployIngress(upstreamVhostIngressName, upstreamVhostNginxHost, vhostAnnotations)
	require.NoError(s.T(), err, "deploy upstream-vhost ingress to nginx cluster")

	// Ingress without upstream-vhost for comparison.
	err = s.traefik.DeployIngress(noVhostIngressName, noVhostTraefikHost, nil)
	require.NoError(s.T(), err, "deploy no-vhost ingress to traefik cluster")

	err = s.nginx.DeployIngress(noVhostIngressName, noVhostNginxHost, nil)
	require.NoError(s.T(), err, "deploy no-vhost ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), upstreamVhostTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), upstreamVhostNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), noVhostTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), noVhostNginxHost, 20, 1*time.Second)
}

func (s *UpstreamVhostSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(upstreamVhostIngressName)
	_ = s.nginx.DeleteIngress(upstreamVhostIngressName)
	_ = s.traefik.DeleteIngress(noVhostIngressName)
	_ = s.nginx.DeleteIngress(noVhostIngressName)
}

// requestVhost makes the same HTTP request against both clusters using the upstream-vhost ingress.
func (s *UpstreamVhostSuite) requestVhost(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), upstreamVhostTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), upstreamVhostNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestNoVhost makes the same HTTP request against both clusters using the no-vhost ingress.
func (s *UpstreamVhostSuite) requestNoVhost(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), noVhostTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), noVhostNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *UpstreamVhostSuite) TestUpstreamVhostOverridesHost() {
	traefikResp, nginxResp := s.requestVhost(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	// The whoami backend echoes request headers in the body.
	// With upstream-vhost set, the Host header seen by the backend should be the custom value.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["Host"],
		traefikResp.RequestHeaders["Host"],
		"Host header seen by backend should match between controllers",
	)
	assert.Equal(s.T(), customUpstreamHost, traefikResp.RequestHeaders["Host"],
		"traefik backend should see Host: %s", customUpstreamHost)
	assert.Equal(s.T(), customUpstreamHost, nginxResp.RequestHeaders["Host"],
		"nginx backend should see Host: %s", customUpstreamHost)
}

func (s *UpstreamVhostSuite) TestNoVhostUsesOriginalHost() {
	traefikResp, nginxResp := s.requestNoVhost(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	// Without upstream-vhost, the backend should see the original Host header.
	assert.Equal(s.T(), noVhostTraefikHost, traefikResp.RequestHeaders["Host"],
		"traefik backend should see original Host header")
	assert.Equal(s.T(), noVhostNginxHost, nginxResp.RequestHeaders["Host"],
		"nginx backend should see original Host header")
}

func (s *UpstreamVhostSuite) TestUpstreamVhostOnSubpath() {
	traefikResp, nginxResp := s.requestVhost(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Host override should apply to all paths.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["Host"],
		traefikResp.RequestHeaders["Host"],
		"Host header on subpath should match between controllers",
	)
	assert.Equal(s.T(), customUpstreamHost, traefikResp.RequestHeaders["Host"],
		"traefik backend should see Host: %s on subpath", customUpstreamHost)
}

func (s *UpstreamVhostSuite) TestUpstreamVhostPreservesOtherHeaders() {
	traefikResp, nginxResp := s.requestVhost(http.MethodGet, "/", map[string]string{
		"X-Custom-Header": "test-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Custom headers should still be forwarded to the backend.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Custom-Header"],
		traefikResp.RequestHeaders["X-Custom-Header"],
		"custom header should be forwarded to backend",
	)
}
