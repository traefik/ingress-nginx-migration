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
	forwardAuthIngressName  = "forward-auth-test"
	forwardAuthTraefikHost  = forwardAuthIngressName + ".traefik.local"
	forwardAuthNginxHost    = forwardAuthIngressName + ".nginx.local"
	forwardAuthGatewayHost  = forwardAuthIngressName + ".gateway.local"

	forwardAuthDenyIngressName = "forward-auth-deny-test"
	forwardAuthDenyTraefikHost = forwardAuthDenyIngressName + ".traefik.local"
	forwardAuthDenyNginxHost   = forwardAuthDenyIngressName + ".nginx.local"
	forwardAuthDenyGatewayHost = forwardAuthDenyIngressName + ".gateway.local"

	forwardAuthHeadersIngressName = "forward-auth-headers-test"
	forwardAuthHeadersTraefikHost = forwardAuthHeadersIngressName + ".traefik.local"
	forwardAuthHeadersNginxHost   = forwardAuthHeadersIngressName + ".nginx.local"
	forwardAuthHeadersGatewayHost = forwardAuthHeadersIngressName + ".gateway.local"

	forwardAuthSigninIngressName = "forward-auth-signin-test"
	forwardAuthSigninTraefikHost = forwardAuthSigninIngressName + ".traefik.local"
	forwardAuthSigninNginxHost   = forwardAuthSigninIngressName + ".nginx.local"
	forwardAuthSigninGatewayHost = forwardAuthSigninIngressName + ".gateway.local"

	authServerServiceURL = "http://auth-server.default.svc.cluster.local"
	authSigninURL        = "https://login.example.com/oauth2/start"
)

type ForwardAuthSuite struct {
	BaseSuite
}

func TestForwardAuthSuite(t *testing.T) {
	suite.Run(t, new(ForwardAuthSuite))
}

func (s *ForwardAuthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Deploy auth server to both clusters.
	err := s.traefik.ApplyFixture("auth-server.yaml")
	require.NoError(s.T(), err, "deploy auth-server to traefik cluster")

	err = s.nginx.ApplyFixture("auth-server.yaml")
	require.NoError(s.T(), err, "deploy auth-server to nginx cluster")

	// Wait for auth server to be ready.
	err = waitForDeployment(s.traefik, s.traefik.TestNamespace, "auth-server")
	require.NoError(s.T(), err, "auth-server not ready in traefik cluster")

	err = waitForDeployment(s.nginx, s.nginx.TestNamespace, "auth-server")
	require.NoError(s.T(), err, "auth-server not ready in nginx cluster")

	// 1. Forward auth with allow endpoint (auth-url returns 200).
	allowAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url": authServerServiceURL + "/",
	}

	err = s.traefik.DeployIngress(forwardAuthIngressName, forwardAuthTraefikHost, allowAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth ingress to traefik cluster")

	err = s.nginx.DeployIngress(forwardAuthIngressName, forwardAuthNginxHost, allowAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth ingress to nginx cluster")

	// 2. Forward auth with deny endpoint (auth-url returns 401).
	denyAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url": authServerServiceURL + "/deny",
	}

	err = s.traefik.DeployIngress(forwardAuthDenyIngressName, forwardAuthDenyTraefikHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth-deny ingress to traefik cluster")

	err = s.nginx.DeployIngress(forwardAuthDenyIngressName, forwardAuthDenyNginxHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth-deny ingress to nginx cluster")

	// 3. Forward auth with response headers forwarding.
	headersAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
		"nginx.ingress.kubernetes.io/auth-response-headers": "X-Auth-User,X-Auth-Role",
	}

	err = s.traefik.DeployIngress(forwardAuthHeadersIngressName, forwardAuthHeadersTraefikHost, headersAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth-headers ingress to traefik cluster")

	err = s.nginx.DeployIngress(forwardAuthHeadersIngressName, forwardAuthHeadersNginxHost, headersAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth-headers ingress to nginx cluster")

	// 4. Forward auth with signin URL (auth-url returns 401, redirects to signin).
	signinAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":    authServerServiceURL + "/deny",
		"nginx.ingress.kubernetes.io/auth-signin": authSigninURL,
	}

	err = s.traefik.DeployIngress(forwardAuthSigninIngressName, forwardAuthSigninTraefikHost, signinAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth-signin ingress to traefik cluster")

	err = s.nginx.DeployIngress(forwardAuthSigninIngressName, forwardAuthSigninNginxHost, signinAnnotations)
	require.NoError(s.T(), err, "deploy forward-auth-signin ingress to nginx cluster")

	// Deploy Gateway API equivalents.
	// Note: s.gateway shares the same Traefik instance as s.traefik, so the
	// auth-server is already running in the cluster — no extra deployment needed.
	gwDir := filepath.Join(fixturesDir, "gateway", "forwardauth")
	for _, f := range []string{"allow.yaml", "deny.yaml", "headers.yaml", "signin.yaml"} {
		err = s.gateway.DeployGatewayFixture(filepath.Join(gwDir, f))
		require.NoError(s.T(), err, "deploy gateway fixture %s", f)
	}

	s.traefik.WaitForIngressReady(s.T(), forwardAuthTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), forwardAuthNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), forwardAuthDenyTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), forwardAuthDenyNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), forwardAuthHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), forwardAuthHeadersNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), forwardAuthSigninTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), forwardAuthSigninNginxHost, 20, 1*time.Second)
	// Gateway API routes need more time — CRD provider must publish middleware config first.
	s.gateway.WaitForIngressReady(s.T(), forwardAuthGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), forwardAuthDenyGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), forwardAuthHeadersGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), forwardAuthSigninGatewayHost, 60, 1*time.Second)
}

func (s *ForwardAuthSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}

	_ = s.traefik.DeleteIngress(forwardAuthIngressName)
	_ = s.nginx.DeleteIngress(forwardAuthIngressName)
	_ = s.traefik.DeleteIngress(forwardAuthDenyIngressName)
	_ = s.nginx.DeleteIngress(forwardAuthDenyIngressName)
	_ = s.traefik.DeleteIngress(forwardAuthHeadersIngressName)
	_ = s.nginx.DeleteIngress(forwardAuthHeadersIngressName)
	_ = s.traefik.DeleteIngress(forwardAuthSigninIngressName)
	_ = s.nginx.DeleteIngress(forwardAuthSigninIngressName)

	// Clean up Gateway API fixtures.
	gwDir := filepath.Join(fixturesDir, "gateway", "forwardauth")
	for _, f := range []string{"allow.yaml", "deny.yaml", "headers.yaml", "signin.yaml"} {
		_ = s.gateway.DeleteGatewayFixture(filepath.Join(gwDir, f))
	}

	// Clean up auth server.
	_ = s.traefik.Kubectl("delete", "-f", fmt.Sprintf("%s/auth-server.yaml", fixturesDir), "-n", s.traefik.TestNamespace, "--ignore-not-found")
	_ = s.nginx.Kubectl("delete", "-f", fmt.Sprintf("%s/auth-server.yaml", fixturesDir), "-n", s.nginx.TestNamespace, "--ignore-not-found")
}

// requestAllow makes the same HTTP request against both clusters using the allow forward-auth ingress.
func (s *ForwardAuthSuite) requestAllow(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), forwardAuthTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), forwardAuthNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestDeny makes the same HTTP request against both clusters using the deny forward-auth ingress.
func (s *ForwardAuthSuite) requestDeny(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), forwardAuthDenyTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), forwardAuthDenyNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestHeaders makes the same HTTP request against both clusters using the headers forward-auth ingress.
func (s *ForwardAuthSuite) requestHeaders(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), forwardAuthHeadersTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), forwardAuthHeadersNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *ForwardAuthSuite) TestAuthAllowPassesThrough() {
	traefikResp, nginxResp := s.requestAllow(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth service returns 200")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 when auth service returns 200")

	// Gateway API migration: same request through HTTPRoute + forwardAuth Middleware.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode,
		"gateway: expected 200 when auth service returns 200")
}

func (s *ForwardAuthSuite) TestAuthDenyReturnsUnauthorized() {
	traefikResp, nginxResp := s.requestDeny(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode,
		"expected 401 when auth service returns 401")
	assert.Equal(s.T(), http.StatusUnauthorized, nginxResp.StatusCode,
		"expected 401 when auth service returns 401")

	// Gateway API migration: forwardAuth middleware should deny the request with 401.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthDenyGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusUnauthorized, gatewayResp.StatusCode,
		"gateway: expected 401 when auth service returns 401")
}

func (s *ForwardAuthSuite) TestAuthResponseHeadersForwarded() {
	traefikResp, nginxResp := s.requestHeaders(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 when auth passes")

	// The auth server returns X-Auth-User and X-Auth-Role as response headers.
	// With auth-response-headers set, these should be forwarded to the upstream as request headers.
	// The whoami backend echoes request headers, so we can check them.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Auth-User"],
		traefikResp.RequestHeaders["X-Auth-User"],
		"X-Auth-User forwarding mismatch",
	)
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Auth-Role"],
		traefikResp.RequestHeaders["X-Auth-Role"],
		"X-Auth-Role forwarding mismatch",
	)
	assert.Equal(s.T(), "authenticated-user", traefikResp.RequestHeaders["X-Auth-User"],
		"traefik should forward X-Auth-User from auth response")
	assert.Equal(s.T(), "admin", traefikResp.RequestHeaders["X-Auth-Role"],
		"traefik should forward X-Auth-Role from auth response")

	// Gateway API migration: authResponseHeaders in forwardAuth middleware should
	// forward X-Auth-User and X-Auth-Role to the upstream backend.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthHeadersGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode, "gateway: expected 200 when auth passes")
	assert.Equal(s.T(), "authenticated-user", gatewayResp.RequestHeaders["X-Auth-User"],
		"gateway: should forward X-Auth-User from auth response")
	assert.Equal(s.T(), "admin", gatewayResp.RequestHeaders["X-Auth-Role"],
		"gateway: should forward X-Auth-Role from auth response")
}

// TestAuthResponseHeaderSpoofing verifies that a client cannot spoof headers
// listed in auth-response-headers: the value from the auth service must
// overwrite any client-supplied value before reaching the upstream. Headers
// NOT listed in the annotation are passed through unchanged (and remain
// spoofable) on both clusters.
func (s *ForwardAuthSuite) TestAuthResponseHeaderSpoofing() {
	// Client tries to spoof the listed auth headers, and sends an unlisted header.
	traefikResp, nginxResp := s.requestHeaders(http.MethodGet, "/", map[string]string{
		"X-Auth-User":      "spoofed-attacker",
		"X-Auth-Role":      "spoofed-admin",
		"X-Unlisted-Spoof": "passthrough",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 when auth passes")

	// Listed headers: the auth service value must win over the client-supplied one.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Auth-User"],
		traefikResp.RequestHeaders["X-Auth-User"],
		"X-Auth-User spoofing behavior mismatch between nginx and traefik",
	)
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Auth-Role"],
		traefikResp.RequestHeaders["X-Auth-Role"],
		"X-Auth-Role spoofing behavior mismatch between nginx and traefik",
	)
	assert.Equal(s.T(), "authenticated-user", traefikResp.RequestHeaders["X-Auth-User"],
		"traefik must overwrite spoofed X-Auth-User with the auth service value")
	assert.Equal(s.T(), "admin", traefikResp.RequestHeaders["X-Auth-Role"],
		"traefik must overwrite spoofed X-Auth-Role with the auth service value")
	assert.NotEqual(s.T(), "spoofed-attacker", traefikResp.ResponseHeaders["X-Auth-User"],
		"client must not be able to spoof a listed auth-response-header")
	assert.NotEqual(s.T(), "spoofed-admin", traefikResp.ResponseHeaders["X-Auth-Role"],
		"client must not be able to spoof a listed auth-response-header")

	// Unlisted header: not stripped, passed through to the backend on both clusters.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Unlisted-Spoof"],
		traefikResp.RequestHeaders["X-Unlisted-Spoof"],
		"unlisted header passthrough mismatch between nginx and traefik",
	)
	assert.Equal(s.T(), "passthrough", traefikResp.RequestHeaders["X-Unlisted-Spoof"],
		"unlisted header should pass through to the backend untouched")

	// Gateway API migration: forwardAuth middleware must also overwrite spoofed headers.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthHeadersGatewayHost, http.MethodGet, "/", map[string]string{
		"X-Auth-User":      "spoofed-attacker",
		"X-Auth-Role":      "spoofed-admin",
		"X-Unlisted-Spoof": "passthrough",
	}, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode, "gateway: expected 200 when auth passes")
	assert.Equal(s.T(), "authenticated-user", gatewayResp.RequestHeaders["X-Auth-User"],
		"gateway: must overwrite spoofed X-Auth-User with the auth service value")
	assert.Equal(s.T(), "admin", gatewayResp.RequestHeaders["X-Auth-Role"],
		"gateway: must overwrite spoofed X-Auth-Role with the auth service value")
}

func (s *ForwardAuthSuite) TestAuthAllowOnSubpath() {
	traefikResp, nginxResp := s.requestAllow(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth service returns 200 on subpath")

	// Gateway API migration: PathPrefix / covers all subpaths.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthGatewayHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode,
		"gateway: expected 200 when auth service returns 200 on subpath")
}

func (s *ForwardAuthSuite) TestAuthDenyOnSubpath() {
	traefikResp, nginxResp := s.requestDeny(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode,
		"expected 401 when auth service returns 401 on subpath")

	// Gateway API migration: forwardAuth middleware applies to all subpaths via PathPrefix /.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthDenyGatewayHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusUnauthorized, gatewayResp.StatusCode,
		"gateway: expected 401 when auth service returns 401 on subpath")
}

func (s *ForwardAuthSuite) TestAuthAllowWithCustomHeaders() {
	traefikResp, nginxResp := s.requestAllow(http.MethodGet, "/", map[string]string{
		"X-Custom-Header": "custom-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with custom headers")

	// Gateway API migration: custom request headers are forwarded to the auth service.
	gatewayResp := s.gateway.MakeRequest(s.T(), forwardAuthGatewayHost, http.MethodGet, "/", map[string]string{
		"X-Custom-Header": "custom-value",
	}, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp.StatusCode,
		"gateway: expected 200 with custom headers when auth service allows")
}

// requestSignin makes the same HTTP request against both clusters using the signin forward-auth ingress.
func (s *ForwardAuthSuite) requestSignin(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), forwardAuthSigninTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), forwardAuthSigninNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *ForwardAuthSuite) TestAuthSigninRedirectsOnDeny() {
	traefikResp, nginxResp := s.requestSignin(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode,
		"expected 302 redirect when auth service returns 401 with auth-signin configured")
	assert.Equal(s.T(), http.StatusFound, nginxResp.StatusCode,
		"expected 302 redirect when auth service returns 401 with auth-signin configured")

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.Contains(s.T(), traefikLocation, "login.example.com",
		"traefik Location header should contain the auth-signin host")
	assert.Contains(s.T(), nginxLocation, "login.example.com",
		"nginx Location header should contain the auth-signin host")

	// MIGRATION GAP: Traefik's forwardAuth middleware has no equivalent for the
	// nginx auth-signin annotation. nginx automatically redirects the client to
	// the signin URL when the auth service returns 4xx. With the Gateway API
	// forwardAuth middleware, the 401 from the auth service is forwarded directly
	// to the client — no signin redirect occurs. To achieve redirect behavior,
	// the auth service itself must issue a 302, or an additional errors middleware
	// must be configured. No gateway comparison assertion is made here.
}

func (s *ForwardAuthSuite) TestAuthSigninRedirectsOnSubpath() {
	traefikResp, nginxResp := s.requestSignin(http.MethodGet, "/protected/resource", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode,
		"expected 302 redirect on subpath when auth-signin is configured")

	traefikLocation := traefikResp.ResponseHeaders.Get("Location")
	nginxLocation := nginxResp.ResponseHeaders.Get("Location")

	assert.Contains(s.T(), traefikLocation, "login.example.com",
		"traefik Location header should contain the auth-signin host on subpath")
	assert.Contains(s.T(), nginxLocation, "login.example.com",
		"nginx Location header should contain the auth-signin host on subpath")
}

func (s *ForwardAuthSuite) TestSnippet() {
	hostTraefik := "auth-snippet.traefik.local"
	hostNginx := "auth-snippet.nginx.local"
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet":          "proxy_set_header X-From-Request \"Ok\";",
		"nginx.ingress.kubernetes.io/auth-response-headers": "X-From-Request",
	}

	err := s.traefik.DeployIngress("test-auth-snippet-traefik", hostTraefik, annotations)
	require.NoError(s.T(), err, "deploy forward-auth-snippet ingress to traefik cluster")

	err = s.nginx.DeployIngress("test-auth-snippet-nginx", hostNginx, annotations)
	require.NoError(s.T(), err, "deploy forward-auth-snippet ingress to nginx cluster")

	s.T().Cleanup(func() {
		_ = s.traefik.DeleteIngress("test-auth-snippet-traefik")
		_ = s.nginx.DeleteIngress("test-auth-snippet-nginx")
	})

	s.traefik.WaitForIngressReady(s.T(), hostTraefik, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), hostNginx, 20, 1*time.Second)

	traefikResp := s.traefik.MakeRequest(s.T(), hostTraefik, http.MethodGet, "/protected/resource", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), hostNginx, http.MethodGet, "/protected/resource", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

	assert.Equal(s.T(), traefikResp.RequestHeaders["X-From-Request"], "Ok")
	assert.Equal(s.T(), nginxResp.RequestHeaders["X-From-Request"], "Ok")
}

func (s *ForwardAuthSuite) TestAuthSnippet() {
	testCases := []struct {
		desc        string
		annotations map[string]string
		test        func(t *testing.T, hostTraefik, hostNginx string)
	}{
		{
			desc: "empty proxy_method",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Request-Method",
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, http.MethodGet, traefikResp.RequestHeaders["X-Request-Method"], "traefik response header mismatch")
				assert.Equal(t, http.MethodGet, nginxResp.RequestHeaders["X-Request-Method"], "nginx response header mismatch")

			},
		},
		{
			desc: "proxy_method",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
				"nginx.ingress.kubernetes.io/auth-snippet":          "proxy_method \"PUT\";",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Request-Method",
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, http.MethodPut, traefikResp.RequestHeaders["X-Request-Method"], "traefik response header mismatch")
				assert.Equal(t, http.MethodPut, nginxResp.RequestHeaders["X-Request-Method"], "nginx response header mismatch")

			},
		},
		{
			desc: "proxy_method inherited",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
				"nginx.ingress.kubernetes.io/auth-snippet":          "proxy_method $request_method;",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Request-Method",
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, http.MethodPost, traefikResp.RequestHeaders["X-Request-Method"], "traefik response header mismatch")
				assert.Equal(t, http.MethodPost, nginxResp.RequestHeaders["X-Request-Method"], "nginx response header mismatch")
			},
		},
		{
			desc: "proxy_method pass",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
				"nginx.ingress.kubernetes.io/auth-snippet":          "proxy_method $var;\nset $var \"PUT\";",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Request-Method",
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, http.MethodPut, traefikResp.RequestHeaders["X-Request-Method"], "traefik response header mismatch")
				assert.Equal(t, http.MethodPut, nginxResp.RequestHeaders["X-Request-Method"], "nginx response header mismatch")
			},
		},
		{
			desc: "proxy_method with if",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
				"nginx.ingress.kubernetes.io/auth-snippet":          "if ($request_method = GET) { \nreturn 200;}\nproxy_method $request_method;",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Request-Method",
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodGet, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodGet, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, "", traefikResp.RequestHeaders["X-Request-Method"], "traefik response header mismatch")
				assert.Equal(t, "", nginxResp.RequestHeaders["X-Request-Method"], "nginx response header mismatch")

				traefikResp = s.traefik.MakeRequest(t, hostTraefik, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp = s.nginx.MakeRequest(t, hostNginx, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, http.MethodPost, traefikResp.RequestHeaders["X-Request-Method"], "traefik response header mismatch")
				assert.Equal(t, http.MethodPost, nginxResp.RequestHeaders["X-Request-Method"], "nginx response header mismatch")
			},
		},
		{
			desc: "proxy_method with if and 401",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url":              authServerServiceURL + "/",
				"nginx.ingress.kubernetes.io/auth-snippet":          "if ($request_method = GET) { \nreturn 401;}\nproxy_method $request_method;",
				"nginx.ingress.kubernetes.io/auth-response-headers": "X-Request-Method",
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodGet, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodGet, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusUnauthorized, traefikResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusUnauthorized, nginxResp.StatusCode, "traefik status code mismatch")

				assert.Equal(t, "", traefikResp.RequestHeaders["X-Request-Method"], "traefik request header mismatch")
				assert.Equal(t, "", nginxResp.RequestHeaders["X-Request-Method"], "nginx request header mismatch")

				traefikResp = s.traefik.MakeRequest(t, hostTraefik, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp = s.nginx.MakeRequest(t, hostNginx, http.MethodPost, "/protected/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "traefik status code mismatch")
				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "nginx status code mismatch")

				assert.Equal(t, http.MethodPost, traefikResp.RequestHeaders["X-Request-Method"], "traefik request header mismatch")
				assert.Equal(t, http.MethodPost, nginxResp.RequestHeaders["X-Request-Method"], "nginx request header mismatch")
			},
		},
	}

	for _, test := range testCases {
		s.T().Run(test.desc, func(t *testing.T) {
			t.Parallel()
			prefix := sanitizeName(test.desc)
			hostTraefik := prefix + ".traefik.local"
			hostNginx := prefix + ".nginx.local"

			err := s.traefik.DeployIngress(prefix, hostTraefik, test.annotations)
			require.NoError(s.T(), err, "deploy %s ingress to traefik cluster", prefix)

			err = s.nginx.DeployIngress(prefix, hostNginx, test.annotations)
			require.NoError(s.T(), err, "deploy %s ingress to nginx cluster", prefix)

			s.T().Cleanup(func() {
				_ = s.traefik.DeleteIngress(prefix)
				_ = s.nginx.DeleteIngress(prefix)
			})

			s.traefik.WaitForIngressReady(s.T(), hostTraefik, 20, 1*time.Second)
			s.nginx.WaitForIngressReady(s.T(), hostNginx, 20, 1*time.Second)

			test.test(s.T(), hostTraefik, hostNginx)
		})
	}
}
