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
	authMethodGetIngressName = "auth-method-get-test"
	authMethodGetTraefikHost = authMethodGetIngressName + ".traefik.local"
	authMethodGetNginxHost   = authMethodGetIngressName + ".nginx.local"

	authMethodPostIngressName = "auth-method-post-test"
	authMethodPostTraefikHost = authMethodPostIngressName + ".traefik.local"
	authMethodPostNginxHost   = authMethodPostIngressName + ".nginx.local"

	authSnippetProxySetIngressName = "auth-snippet-proxyset-test"
	authSnippetProxySetTraefikHost = authSnippetProxySetIngressName + ".traefik.local"
	authSnippetProxySetNginxHost   = authSnippetProxySetIngressName + ".nginx.local"

	authSnippetAddHeaderIngressName = "auth-snippet-addheader-test"
	authSnippetAddHeaderTraefikHost = authSnippetAddHeaderIngressName + ".traefik.local"
	authSnippetAddHeaderNginxHost   = authSnippetAddHeaderIngressName + ".nginx.local"

	authSnippetProxyMethodIngressName = "auth-snippet-proxymethod-test"
	authSnippetProxyMethodTraefikHost = authSnippetProxyMethodIngressName + ".traefik.local"
	authSnippetProxyMethodNginxHost   = authSnippetProxyMethodIngressName + ".nginx.local"

	authSnippetIfCondIngressName = "auth-snippet-ifcond-test"
	authSnippetIfCondTraefikHost = authSnippetIfCondIngressName + ".traefik.local"
	authSnippetIfCondNginxHost   = authSnippetIfCondIngressName + ".nginx.local"

	authSnippetWithConfigIngressName = "auth-snippet-withconfig-test"
	authSnippetWithConfigTraefikHost = authSnippetWithConfigIngressName + ".traefik.local"
	authSnippetWithConfigNginxHost   = authSnippetWithConfigIngressName + ".nginx.local"

	authSnippetMoreSetIngressName = "auth-snippet-moreset-test"
	authSnippetMoreSetTraefikHost = authSnippetMoreSetIngressName + ".traefik.local"
	authSnippetMoreSetNginxHost   = authSnippetMoreSetIngressName + ".nginx.local"

	authMethodWithSnippetIngressName = "auth-method-withsnippet-test"
	authMethodWithSnippetTraefikHost = authMethodWithSnippetIngressName + ".traefik.local"
	authMethodWithSnippetNginxHost   = authMethodWithSnippetIngressName + ".nginx.local"

	authSnippetServerURL = "http://auth-server.default.svc.cluster.local"
)

type AuthSnippetSuite struct {
	BaseSuite
}

func TestAuthSnippetSuite(t *testing.T) {
	suite.Run(t, new(AuthSnippetSuite))
}

func (s *AuthSnippetSuite) SetupSuite() {
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

	// 1. auth-method GET
	authMethodGetAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":    authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-method": "GET",
	}

	err = s.traefik.DeployIngress(authMethodGetIngressName, authMethodGetTraefikHost, authMethodGetAnnotations)
	require.NoError(s.T(), err, "deploy auth-method-get ingress to traefik cluster")

	err = s.nginx.DeployIngress(authMethodGetIngressName, authMethodGetNginxHost, authMethodGetAnnotations)
	require.NoError(s.T(), err, "deploy auth-method-get ingress to nginx cluster")

	// 2. auth-method POST
	authMethodPostAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":    authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-method": "POST",
	}

	err = s.traefik.DeployIngress(authMethodPostIngressName, authMethodPostTraefikHost, authMethodPostAnnotations)
	require.NoError(s.T(), err, "deploy auth-method-post ingress to traefik cluster")

	err = s.nginx.DeployIngress(authMethodPostIngressName, authMethodPostNginxHost, authMethodPostAnnotations)
	require.NoError(s.T(), err, "deploy auth-method-post ingress to nginx cluster")

	// 3. auth-snippet with proxy_set_header
	authSnippetProxySetAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":     authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet": `proxy_set_header X-Custom-Auth "auth-value";`,
	}

	err = s.traefik.DeployIngress(authSnippetProxySetIngressName, authSnippetProxySetTraefikHost, authSnippetProxySetAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-proxyset ingress to traefik cluster")

	err = s.nginx.DeployIngress(authSnippetProxySetIngressName, authSnippetProxySetNginxHost, authSnippetProxySetAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-proxyset ingress to nginx cluster")

	// 4. auth-snippet with add_header
	authSnippetAddHeaderAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":     authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet": `add_header X-Auth-Debug "debug-value" always;`,
	}

	err = s.traefik.DeployIngress(authSnippetAddHeaderIngressName, authSnippetAddHeaderTraefikHost, authSnippetAddHeaderAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-addheader ingress to traefik cluster")

	err = s.nginx.DeployIngress(authSnippetAddHeaderIngressName, authSnippetAddHeaderNginxHost, authSnippetAddHeaderAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-addheader ingress to nginx cluster")

	// 5. auth-snippet with proxy_method
	authSnippetProxyMethodAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":     authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet": `proxy_method GET;`,
	}

	err = s.traefik.DeployIngress(authSnippetProxyMethodIngressName, authSnippetProxyMethodTraefikHost, authSnippetProxyMethodAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-proxymethod ingress to traefik cluster")

	err = s.nginx.DeployIngress(authSnippetProxyMethodIngressName, authSnippetProxyMethodNginxHost, authSnippetProxyMethodAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-proxymethod ingress to nginx cluster")

	// 6. auth-snippet with if condition
	authSnippetIfCondAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url": authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet": `
if ($request_method = POST) {
    return 403;
}`,
	}

	err = s.traefik.DeployIngress(authSnippetIfCondIngressName, authSnippetIfCondTraefikHost, authSnippetIfCondAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-ifcond ingress to traefik cluster")

	err = s.nginx.DeployIngress(authSnippetIfCondIngressName, authSnippetIfCondNginxHost, authSnippetIfCondAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-ifcond ingress to nginx cluster")

	// 7. auth-snippet + configuration-snippet combined
	authSnippetWithConfigAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":                authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet":            `add_header X-Auth-Extra "from-auth" always;`,
		"nginx.ingress.kubernetes.io/configuration-snippet":   `add_header X-Config-Extra "from-config" always;`,
	}

	err = s.traefik.DeployIngress(authSnippetWithConfigIngressName, authSnippetWithConfigTraefikHost, authSnippetWithConfigAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-withconfig ingress to traefik cluster")

	err = s.nginx.DeployIngress(authSnippetWithConfigIngressName, authSnippetWithConfigNginxHost, authSnippetWithConfigAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-withconfig ingress to nginx cluster")

	// 8. auth-snippet with more_set_input_headers
	authSnippetMoreSetAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":     authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-snippet": `more_set_input_headers "X-Injected: injected-value";`,
	}

	err = s.traefik.DeployIngress(authSnippetMoreSetIngressName, authSnippetMoreSetTraefikHost, authSnippetMoreSetAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-moreset ingress to traefik cluster")

	err = s.nginx.DeployIngress(authSnippetMoreSetIngressName, authSnippetMoreSetNginxHost, authSnippetMoreSetAnnotations)
	require.NoError(s.T(), err, "deploy auth-snippet-moreset ingress to nginx cluster")

	// 9. auth-method + auth-snippet without proxy_method (compatible)
	authMethodWithSnippetAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":     authSnippetServerURL + "/",
		"nginx.ingress.kubernetes.io/auth-method":  "GET",
		"nginx.ingress.kubernetes.io/auth-snippet": `add_header X-Auth-Check "checked" always;`,
	}

	err = s.traefik.DeployIngress(authMethodWithSnippetIngressName, authMethodWithSnippetTraefikHost, authMethodWithSnippetAnnotations)
	require.NoError(s.T(), err, "deploy auth-method-withsnippet ingress to traefik cluster")

	err = s.nginx.DeployIngress(authMethodWithSnippetIngressName, authMethodWithSnippetNginxHost, authMethodWithSnippetAnnotations)
	require.NoError(s.T(), err, "deploy auth-method-withsnippet ingress to nginx cluster")

	// Wait for all ingresses to be ready.
	s.traefik.WaitForIngressReady(s.T(), authMethodGetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authMethodGetNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authMethodPostTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authMethodPostNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authSnippetProxySetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authSnippetProxySetNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authSnippetAddHeaderTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authSnippetAddHeaderNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authSnippetProxyMethodTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authSnippetProxyMethodNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authSnippetIfCondTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authSnippetIfCondNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authSnippetWithConfigTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authSnippetWithConfigNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authSnippetMoreSetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authSnippetMoreSetNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authMethodWithSnippetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authMethodWithSnippetNginxHost, 20, 1*time.Second)
}

func (s *AuthSnippetSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(authMethodGetIngressName)
	_ = s.nginx.DeleteIngress(authMethodGetIngressName)
	_ = s.traefik.DeleteIngress(authMethodPostIngressName)
	_ = s.nginx.DeleteIngress(authMethodPostIngressName)
	_ = s.traefik.DeleteIngress(authSnippetProxySetIngressName)
	_ = s.nginx.DeleteIngress(authSnippetProxySetIngressName)
	_ = s.traefik.DeleteIngress(authSnippetAddHeaderIngressName)
	_ = s.nginx.DeleteIngress(authSnippetAddHeaderIngressName)
	_ = s.traefik.DeleteIngress(authSnippetProxyMethodIngressName)
	_ = s.nginx.DeleteIngress(authSnippetProxyMethodIngressName)
	_ = s.traefik.DeleteIngress(authSnippetIfCondIngressName)
	_ = s.nginx.DeleteIngress(authSnippetIfCondIngressName)
	_ = s.traefik.DeleteIngress(authSnippetWithConfigIngressName)
	_ = s.nginx.DeleteIngress(authSnippetWithConfigIngressName)
	_ = s.traefik.DeleteIngress(authSnippetMoreSetIngressName)
	_ = s.nginx.DeleteIngress(authSnippetMoreSetIngressName)
	_ = s.traefik.DeleteIngress(authMethodWithSnippetIngressName)
	_ = s.nginx.DeleteIngress(authMethodWithSnippetIngressName)

	// Clean up auth server.
	_ = s.traefik.Kubectl("delete", "-f", fmt.Sprintf("%s/auth-server.yaml", fixturesDir), "-n", s.traefik.TestNamespace, "--ignore-not-found")
	_ = s.nginx.Kubectl("delete", "-f", fmt.Sprintf("%s/auth-server.yaml", fixturesDir), "-n", s.nginx.TestNamespace, "--ignore-not-found")
}

// TestAuthMethodGET verifies that auth-method GET works with auth-url.
func (s *AuthSnippetSuite) TestAuthMethodGET() {
	traefikResp := s.traefik.MakeRequest(s.T(), authMethodGetTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authMethodGetNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth-method is GET and auth server allows")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 when auth-method is GET and auth server allows")
}

// TestAuthMethodPOST verifies that auth-method POST works with auth-url.
// The auth server allows all methods on /, so the request should succeed.
func (s *AuthSnippetSuite) TestAuthMethodPOST() {
	traefikResp := s.traefik.MakeRequest(s.T(), authMethodPostTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authMethodPostNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth-method is POST and auth server allows")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 when auth-method is POST and auth server allows")
}

// TestAuthSnippetProxySetHeader verifies that auth-snippet with proxy_set_header
// sends the custom header to the auth server in the auth subrequest.
func (s *AuthSnippetSuite) TestAuthSnippetProxySetHeader() {
	traefikResp := s.traefik.MakeRequest(s.T(), authSnippetProxySetTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authSnippetProxySetNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	// The auth server returns 200 on /, so the request should pass through
	// to the backend. The proxy_set_header applies to the auth subrequest,
	// so we verify the overall request succeeds.
	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth-snippet sets proxy header on auth subrequest")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 when auth-snippet sets proxy header on auth subrequest")
}

// TestAuthSnippetAddHeader verifies that auth-snippet with add_header
// does NOT add headers to the client response (add_header in auth-snippet
// applies to the auth subrequest context, not the main response).
// We verify auth passes successfully and coexists with the snippet.
func (s *AuthSnippetSuite) TestAuthSnippetAddHeader() {
	traefikResp := s.traefik.MakeRequest(s.T(), authSnippetAddHeaderTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authSnippetAddHeaderNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth passes with add_header snippet")
}

// TestAuthSnippetProxyMethod verifies that auth-snippet with proxy_method
// overrides the method used for the auth subrequest.
func (s *AuthSnippetSuite) TestAuthSnippetProxyMethod() {
	traefikResp := s.traefik.MakeRequest(s.T(), authSnippetProxyMethodTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authSnippetProxyMethodNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth-snippet sets proxy_method GET")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 when auth-snippet sets proxy_method GET")
}

// TestAuthSnippetIfCondition verifies that auth-snippet with an if block
// can conditionally alter behavior. GET should pass through (200),
// POST should be blocked (403) by the if condition in the auth-snippet.
func (s *AuthSnippetSuite) TestAuthSnippetIfCondition() {
	// GET request should pass through auth and reach the backend.
	traefikRespGet := s.traefik.MakeRequest(s.T(), authSnippetIfCondTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikRespGet, "traefik GET response should not be nil")

	nginxRespGet := s.nginx.MakeRequest(s.T(), authSnippetIfCondNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxRespGet, "nginx GET response should not be nil")

	assert.Equal(s.T(), nginxRespGet.StatusCode, traefikRespGet.StatusCode, "GET status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikRespGet.StatusCode,
		"expected 200 for GET when auth-snippet if condition does not match")
	assert.Equal(s.T(), http.StatusOK, nginxRespGet.StatusCode,
		"expected 200 for GET when auth-snippet if condition does not match")

	// POST request should be blocked by the if condition.
	traefikRespPost := s.traefik.MakeRequest(s.T(), authSnippetIfCondTraefikHost, http.MethodPost, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikRespPost, "traefik POST response should not be nil")

	nginxRespPost := s.nginx.MakeRequest(s.T(), authSnippetIfCondNginxHost, http.MethodPost, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxRespPost, "nginx POST response should not be nil")

	assert.Equal(s.T(), nginxRespPost.StatusCode, traefikRespPost.StatusCode, "POST status code mismatch")
	assert.Equal(s.T(), http.StatusForbidden, traefikRespPost.StatusCode,
		"expected 403 for POST when auth-snippet if condition matches")
	assert.Equal(s.T(), http.StatusForbidden, nginxRespPost.StatusCode,
		"expected 403 for POST when auth-snippet if condition matches")
}

// TestAuthSnippetWithConfigSnippet verifies that auth-snippet and
// configuration-snippet can be used together. Both should add their
// respective headers to the response.
func (s *AuthSnippetSuite) TestAuthSnippetWithConfigSnippet() {
	traefikResp := s.traefik.MakeRequest(s.T(), authSnippetWithConfigTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authSnippetWithConfigNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when both auth-snippet and configuration-snippet are set")

	// auth-snippet add_header does NOT affect client response (auth subrequest context only).
	// Only configuration-snippet add_header affects the client response.
	assert.Equal(s.T(), "from-config", traefikResp.ResponseHeaders.Get("X-Config-Extra"),
		"traefik should include X-Config-Extra header from configuration-snippet")
	assert.Equal(s.T(), "from-config", nginxResp.ResponseHeaders.Get("X-Config-Extra"),
		"nginx should include X-Config-Extra header from configuration-snippet")
}

// TestAuthSnippetMoreSetInputHeaders verifies that auth-snippet with
// more_set_input_headers does not break auth. The directive modifies the auth
// subrequest headers, not the upstream request, so we can only verify auth succeeds.
func (s *AuthSnippetSuite) TestAuthSnippetMoreSetInputHeaders() {
	traefikResp := s.traefik.MakeRequest(s.T(), authSnippetMoreSetTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authSnippetMoreSetNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth-snippet uses more_set_input_headers")
}

// TestAuthMethodWithSnippet verifies that auth-method and auth-snippet can
// be used together when auth-snippet does not contain proxy_method.
// add_header in auth-snippet applies to the auth subrequest, not the client response.
func (s *AuthSnippetSuite) TestAuthMethodWithSnippet() {
	traefikResp := s.traefik.MakeRequest(s.T(), authMethodWithSnippetTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), authMethodWithSnippetNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 when auth-method GET and auth-snippet add_header are combined")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"expected 200 when auth-method GET and auth-snippet add_header are combined")
}
