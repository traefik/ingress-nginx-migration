package e2e

import (
	"net/http"
	"path/filepath"
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
	whitelistAllowGatewayHost = whitelistAllowIngressName + ".gateway.local"
	whitelistDenyTraefikHost  = whitelistDenyIngressName + ".traefik.local"
	whitelistDenyNginxHost    = whitelistDenyIngressName + ".nginx.local"
	whitelistDenyGatewayHost  = whitelistDenyIngressName + ".gateway.local"
	allowlistIngressName      = "allowlist-test"
	allowlistTraefikHost      = allowlistIngressName + ".traefik.local"
	allowlistNginxHost        = allowlistIngressName + ".nginx.local"
	allowlistGatewayHost      = allowlistIngressName + ".gateway.local"
	precedenceIngressName     = "allowlist-precedence-test"
	precedenceTraefikHost     = precedenceIngressName + ".traefik.local"
	precedenceNginxHost       = precedenceIngressName + ".nginx.local"
	precedenceGatewayHost     = precedenceIngressName + ".gateway.local"
	multiCIDRIngressName      = "whitelist-multi-cidr-test"
	multiCIDRTraefikHost      = multiCIDRIngressName + ".traefik.local"
	multiCIDRNginxHost        = multiCIDRIngressName + ".nginx.local"
	multiCIDRGatewayHost      = multiCIDRIngressName + ".gateway.local"
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

	// Ingress with allowlist-source-range (modern annotation) allowing all IPs.
	allowlistAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/allowlist-source-range": "0.0.0.0/0",
	}

	err = s.traefik.DeployIngress(allowlistIngressName, allowlistTraefikHost, allowlistAnnotations)
	require.NoError(s.T(), err, "deploy allowlist ingress to traefik cluster")

	err = s.nginx.DeployIngress(allowlistIngressName, allowlistNginxHost, allowlistAnnotations)
	require.NoError(s.T(), err, "deploy allowlist ingress to nginx cluster")

	// Ingress with both annotations — allowlist-source-range should take precedence.
	precedenceAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/allowlist-source-range": "192.0.2.0/24",
		"nginx.ingress.kubernetes.io/whitelist-source-range": "0.0.0.0/0",
	}

	err = s.traefik.DeployIngress(precedenceIngressName, precedenceTraefikHost, precedenceAnnotations)
	require.NoError(s.T(), err, "deploy allowlist-precedence ingress to traefik cluster")

	err = s.nginx.DeployIngress(precedenceIngressName, precedenceNginxHost, precedenceAnnotations)
	require.NoError(s.T(), err, "deploy allowlist-precedence ingress to nginx cluster")

	// Ingress with multiple CIDR ranges including 0.0.0.0/0 to allow all traffic.
	multiCIDRAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/whitelist-source-range": "10.0.0.0/8,172.16.0.0/12,0.0.0.0/0",
	}

	err = s.traefik.DeployIngress(multiCIDRIngressName, multiCIDRTraefikHost, multiCIDRAnnotations)
	require.NoError(s.T(), err, "deploy whitelist-multi-cidr ingress to traefik cluster")

	err = s.nginx.DeployIngress(multiCIDRIngressName, multiCIDRNginxHost, multiCIDRAnnotations)
	require.NoError(s.T(), err, "deploy whitelist-multi-cidr ingress to nginx cluster")

	// Deploy Gateway API equivalents.
	gwDir := filepath.Join(fixturesDir, "gateway", "whitelist")
	for _, f := range []string{"allow.yaml", "deny.yaml", "allowlist.yaml", "precedence.yaml", "multi-cidr.yaml"} {
		err = s.gateway.DeployGatewayFixture(filepath.Join(gwDir, f))
		require.NoError(s.T(), err, "deploy gateway fixture %s", f)
	}

	s.traefik.WaitForIngressReady(s.T(), whitelistAllowTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), whitelistAllowNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), whitelistDenyTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), whitelistDenyNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), allowlistTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), allowlistNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), precedenceTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), precedenceNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), multiCIDRTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), multiCIDRNginxHost, 20, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), whitelistAllowGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), whitelistDenyGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), allowlistGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), precedenceGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), multiCIDRGatewayHost, 60, 1*time.Second)
}

func (s *WhitelistSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(whitelistAllowIngressName)
	_ = s.nginx.DeleteIngress(whitelistAllowIngressName)
	_ = s.traefik.DeleteIngress(whitelistDenyIngressName)
	_ = s.nginx.DeleteIngress(whitelistDenyIngressName)
	_ = s.traefik.DeleteIngress(allowlistIngressName)
	_ = s.nginx.DeleteIngress(allowlistIngressName)
	_ = s.traefik.DeleteIngress(precedenceIngressName)
	_ = s.nginx.DeleteIngress(precedenceIngressName)
	_ = s.traefik.DeleteIngress(multiCIDRIngressName)
	_ = s.nginx.DeleteIngress(multiCIDRIngressName)

	gwDir := filepath.Join(fixturesDir, "gateway", "whitelist")
	for _, f := range []string{"allow.yaml", "deny.yaml", "allowlist.yaml", "precedence.yaml", "multi-cidr.yaml"} {
		_ = s.gateway.DeleteGatewayFixture(filepath.Join(gwDir, f))
	}
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

func (s *WhitelistSuite) gatewayRequestTo(gatewayHost, method, path string) *Response {
	s.T().Helper()
	resp := s.gateway.MakeRequest(s.T(), gatewayHost, method, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), resp, "gateway response should not be nil")
	return resp
}

func (s *WhitelistSuite) TestAllowAllAccess() {
	traefikResp, nginxResp := s.requestTo(whitelistAllowTraefikHost, whitelistAllowNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with 0.0.0.0/0 whitelist")

	gatewayResp := s.gatewayRequestTo(whitelistAllowGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
}

func (s *WhitelistSuite) TestDenyNonWhitelistedIP() {
	traefikResp, nginxResp := s.requestTo(whitelistDenyTraefikHost, whitelistDenyNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "expected 403 when client IP is not in whitelist")

	gatewayResp := s.gatewayRequestTo(whitelistDenyGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
}

func (s *WhitelistSuite) TestDenyResponseBody() {
	traefikResp, nginxResp := s.requestTo(whitelistDenyTraefikHost, whitelistDenyNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "traefik should return 403")
	assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode, "nginx should return 403")

	gatewayResp := s.gatewayRequestTo(whitelistDenyGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), http.StatusForbidden, gatewayResp.StatusCode, "gateway should return 403")
}

func (s *WhitelistSuite) TestAllowAllWithDifferentPaths() {
	traefikResp, nginxResp := s.requestTo(whitelistAllowTraefikHost, whitelistAllowNginxHost, http.MethodGet, "/some/path")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with 0.0.0.0/0 whitelist on /some/path")

	gatewayResp := s.gatewayRequestTo(whitelistAllowGatewayHost, http.MethodGet, "/some/path")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch on subpath")
}

func (s *WhitelistSuite) TestAllowlistAllAccess() {
	traefikResp, nginxResp := s.requestTo(allowlistTraefikHost, allowlistNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with 0.0.0.0/0 allowlist-source-range")

	gatewayResp := s.gatewayRequestTo(allowlistGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
}

func (s *WhitelistSuite) TestAllowlistPrecedenceDeny() {
	traefikResp, nginxResp := s.requestTo(precedenceTraefikHost, precedenceNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "expected 403 when allowlist-source-range takes precedence over whitelist-source-range")

	gatewayResp := s.gatewayRequestTo(precedenceGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
}

func (s *WhitelistSuite) TestAllowlistPrecedenceStatusMatch() {
	traefikResp, nginxResp := s.requestTo(precedenceTraefikHost, precedenceNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "traefik should return 403 when allowlist-source-range restricts access")
	assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode, "nginx should return 403 when allowlist-source-range restricts access")

	gatewayResp := s.gatewayRequestTo(precedenceGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), http.StatusForbidden, gatewayResp.StatusCode, "gateway should return 403")
}

func (s *WhitelistSuite) TestMultipleCIDRAllowAccess() {
	traefikResp, nginxResp := s.requestTo(multiCIDRTraefikHost, multiCIDRNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with 0.0.0.0/0 in multi-CIDR whitelist")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 with 0.0.0.0/0 in multi-CIDR whitelist")

	gatewayResp := s.gatewayRequestTo(multiCIDRGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
}

func (s *WhitelistSuite) TestMultipleCIDRStatusMatch() {
	traefikResp, nginxResp := s.requestTo(multiCIDRTraefikHost, multiCIDRNginxHost, http.MethodGet, "/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch between traefik and nginx for multi-CIDR whitelist")

	gatewayResp := s.gatewayRequestTo(multiCIDRGatewayHost, http.MethodGet, "/")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
}
