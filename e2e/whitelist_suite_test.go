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
	whitelistAllowIngressName = "whitelist-allow-test"
	whitelistDenyIngressName  = "whitelist-deny-test"
	whitelistAllowTraefikHost = whitelistAllowIngressName + ".traefik.local"
	whitelistAllowNginxHost   = whitelistAllowIngressName + ".nginx.local"
	whitelistDenyTraefikHost  = whitelistDenyIngressName + ".traefik.local"
	whitelistDenyNginxHost    = whitelistDenyIngressName + ".nginx.local"
)

type WhitelistSuite struct {
	BaseSuite
}

func TestWhitelistSuite(t *testing.T) {
	suite.Run(t, new(WhitelistSuite))
}

func (s *WhitelistSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress with whitelist allowing all IPs.
	allowAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/whitelist-source-range": "0.0.0.0/0",
	}

	err := s.traefik.DeployIngress(whitelistAllowIngressName, whitelistAllowTraefikHost, allowAnnotations)
	require.NoError(s.T(), err, "deploy whitelist-allow ingress to traefik cluster")

	err = s.nginx.DeployIngress(whitelistAllowIngressName, whitelistAllowNginxHost, allowAnnotations)
	require.NoError(s.T(), err, "deploy whitelist-allow ingress to nginx cluster")

	// Ingress with whitelist restricted to TEST-NET-1 (RFC 5737) — the test client IP will NOT be in this range.
	denyAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/whitelist-source-range": "192.0.2.0/24",
	}

	err = s.traefik.DeployIngress(whitelistDenyIngressName, whitelistDenyTraefikHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy whitelist-deny ingress to traefik cluster")

	err = s.nginx.DeployIngress(whitelistDenyIngressName, whitelistDenyNginxHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy whitelist-deny ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), whitelistAllowTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), whitelistAllowNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), whitelistDenyTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), whitelistDenyNginxHost, 20, 1*time.Second)
}

func (s *WhitelistSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(whitelistAllowIngressName)
	_ = s.nginx.DeleteIngress(whitelistAllowIngressName)
	_ = s.traefik.DeleteIngress(whitelistDenyIngressName)
	_ = s.nginx.DeleteIngress(whitelistDenyIngressName)
}

// requestTo makes the same HTTP request against both clusters for the given host pair.
func (s *WhitelistSuite) requestTo(traefikHost, nginxHost, method, path string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), traefikHost, method, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), nginxHost, method, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *WhitelistSuite) TestAllowAllAccess() {
	traefikResp, nginxResp := s.requestTo(whitelistAllowTraefikHost, whitelistAllowNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with 0.0.0.0/0 whitelist")
}

func (s *WhitelistSuite) TestDenyNonWhitelistedIP() {
	traefikResp, nginxResp := s.requestTo(whitelistDenyTraefikHost, whitelistDenyNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "expected 403 when client IP is not in whitelist")
}

func (s *WhitelistSuite) TestDenyResponseBody() {
	traefikResp, nginxResp := s.requestTo(whitelistDenyTraefikHost, whitelistDenyNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "traefik should return 403")
	assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode, "nginx should return 403")
}

func (s *WhitelistSuite) TestAllowAllWithDifferentPaths() {
	traefikResp, nginxResp := s.requestTo(whitelistAllowTraefikHost, whitelistAllowNginxHost, http.MethodGet, "/some/path")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with 0.0.0.0/0 whitelist on /some/path")
}
