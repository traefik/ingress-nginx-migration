package e2e

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// This suite covers an Ingress that combines an authentication annotation with
// from-to-www-redirect. ingress-nginx serves the www variant purely as a
// redirect and never exposes the auth-protected backend without credentials.
// The migration requirement is that Traefik match that behaviour for the www
// host — including when the Host header carries a non-numeric or empty port,
// which some clients and intermediaries send. An unauthenticated request must
// never reach the protected backend.

const (
	wwwAuthIngressName = "wwwredir-auth-test"
	wwwAuthTraefikHost = wwwAuthIngressName + ".traefik.local"
	wwwAuthNginxHost   = wwwAuthIngressName + ".nginx.local"

	wwwAuthSecretName = "wwwredir-auth"
	wwwAuthUser       = "wwwuser"
	wwwAuthPass       = "wwwpass"
	wwwAuthRealm      = "Restricted"
)

type WWWRedirectAuthSuite struct {
	BaseSuite
}

func TestWWWRedirectAuthSuite(t *testing.T) {
	suite.Run(t, new(WWWRedirectAuthSuite))
}

func (s *WWWRedirectAuthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	authSecret := secretTemplateData{
		Name: wwwAuthSecretName,
		Data: map[string]string{
			"auth": base64.StdEncoding.EncodeToString([]byte(htpasswdSHA(wwwAuthUser, wwwAuthPass))),
		},
	}
	require.NoError(s.T(), s.traefik.DeploySecret(authSecret), "create auth secret in traefik cluster")
	require.NoError(s.T(), s.nginx.DeploySecret(authSecret), "create auth secret in nginx cluster")

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-type":            "basic",
		"nginx.ingress.kubernetes.io/auth-secret":          wwwAuthSecretName,
		"nginx.ingress.kubernetes.io/auth-realm":           wwwAuthRealm,
		"nginx.ingress.kubernetes.io/from-to-www-redirect": "true",
	}

	require.NoError(s.T(), s.traefik.DeployIngress(wwwAuthIngressName, wwwAuthTraefikHost, annotations),
		"deploy ingress to traefik cluster")
	require.NoError(s.T(), s.nginx.DeployIngress(wwwAuthIngressName, wwwAuthNginxHost, annotations),
		"deploy ingress to nginx cluster")

	// Parent (auth-protected) host and the auto-generated www redirect host.
	s.traefik.WaitForIngressReady(s.T(), wwwAuthTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), wwwAuthNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), "www."+wwwAuthTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), "www."+wwwAuthNginxHost, 20, 1*time.Second)
}

func (s *WWWRedirectAuthSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(wwwAuthIngressName)
	_ = s.nginx.DeleteIngress(wwwAuthIngressName)
	_ = s.traefik.DeleteSecret(wwwAuthSecretName)
	_ = s.nginx.DeleteSecret(wwwAuthSecretName)
}

func wwwAuthCreds(user, pass string) map[string]string {
	return map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)),
	}
}

// backendReached reports whether the whoami backend actually served the
// request (it echoes a Hostname header only when the request reaches it).
func backendReached(resp *Response) bool {
	return resp != nil && strings.HasPrefix(resp.RequestHeaders["Hostname"], "backend")
}

func (s *WWWRedirectAuthSuite) TestParentHostRequiresAuth() {
	traefikResp := s.traefik.MakeRequest(s.T(), wwwAuthTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), wwwAuthNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode, "parent host must require auth")
	assert.False(s.T(), backendReached(traefikResp), "backend must not be reached without credentials")
}

func (s *WWWRedirectAuthSuite) TestParentHostServesWithCredentials() {
	creds := wwwAuthCreds(wwwAuthUser, wwwAuthPass)
	traefikResp := s.traefik.MakeRequest(s.T(), wwwAuthTraefikHost, http.MethodGet, "/", creds, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), wwwAuthNginxHost, http.MethodGet, "/", creds, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "parent host must serve with valid credentials")
	assert.True(s.T(), backendReached(traefikResp), "backend should be reached with valid credentials")
}

func (s *WWWRedirectAuthSuite) TestWWWHostRedirectsUnauthenticated() {
	// The plain www host (no port) already redirects on both controllers; this
	// is the baseline the malformed-port cases are compared against.
	traefikResp := s.traefik.MakeRequest(s.T(), "www."+wwwAuthTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), "www."+wwwAuthNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusPermanentRedirect, nginxResp.StatusCode, "[nginx] www host must redirect")
	assert.Equal(s.T(), http.StatusPermanentRedirect, traefikResp.StatusCode, "[traefik] www host must redirect")
	assert.False(s.T(), backendReached(traefikResp), "www host must redirect, not serve the backend")

	// The Location is built from the capture groups of the www redirect regex,
	// so assert the target and not only the status code.
	assertRedirectTarget(s.T(), nginxResp, wwwAuthNginxHost, "/", "nginx")
	assertRedirectTarget(s.T(), traefikResp, wwwAuthTraefikHost, "/", "traefik")
}

// assertRedirectTarget checks that the response carries a Location header
// pointing at wantHost and wantPath.
func assertRedirectTarget(t *testing.T, resp *Response, wantHost, wantPath, controller string) {
	t.Helper()

	loc := resp.ResponseHeaders.Get("Location")
	require.NotEmpty(t, loc, "[%s] missing Location header", controller)

	// url.Parse rejects a non-numeric port, so a malformed port leaking from the
	// request Host header into the Location fails here.
	u, err := url.Parse(loc)
	require.NoError(t, err, "[%s] invalid Location %q", controller, loc)

	// ingress-nginx keeps the listening port in the Location (":80"), Traefik does not.
	assert.Equal(t, wantHost, u.Hostname(), "[%s] unexpected redirect host in %q", controller, loc)

	// ingress-nginx omits the trailing slash of the root path, Traefik keeps it.
	path := u.Path
	if path == "" {
		path = "/"
	}
	assert.Equal(t, wantPath, path, "[%s] unexpected redirect path in %q", controller, loc)
}

// TestWWWHostNonNumericPortMatchesNginx sends the www host with a non-numeric
// port in the Host header. ingress-nginx still treats it as the redirect host
// and never exposes the protected backend; Traefik must do the same.
func (s *WWWRedirectAuthSuite) TestWWWHostNonNumericPortMatchesNginx() {
	s.assertWWWHostMatchesNginx("www." + wwwAuthTraefikHost + ":x")
}

// TestWWWHostTrailingColonMatchesNginx is the same check with a bare trailing
// colon, which is enough to exhibit any authority-parsing divergence.
func (s *WWWRedirectAuthSuite) TestWWWHostTrailingColonMatchesNginx() {
	s.assertWWWHostMatchesNginx("www." + wwwAuthTraefikHost + ":")
}

func (s *WWWRedirectAuthSuite) assertWWWHostMatchesNginx(traefikHostHeader string) {
	nginxHostHeader := strings.Replace(traefikHostHeader, ".traefik.local", ".nginx.local", 1)

	traefikResp := s.traefik.MakeRequest(s.T(), traefikHostHeader, http.MethodGet, "/admin/secret", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), nginxHostHeader, http.MethodGet, "/admin/secret", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	// Security-critical: an unauthenticated request must never reach the
	// protected backend, whatever the port in the Host header.
	assert.False(s.T(), backendReached(nginxResp),
		"[nginx] protected backend served without credentials for Host %q", nginxHostHeader)
	assert.False(s.T(), backendReached(traefikResp),
		"[traefik] protected backend served without credentials for Host %q", traefikHostHeader)

	// Migration parity: Traefik must match ingress-nginx for this host.
	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status code mismatch for Host %q (nginx=%d traefik=%d)",
		traefikHostHeader, nginxResp.StatusCode, traefikResp.StatusCode)

	// Pin the expected status on both sides: the parity assert alone would also
	// pass if neither ingress were routed at all (404 == 404).
	assert.Equal(s.T(), http.StatusPermanentRedirect, nginxResp.StatusCode,
		"[nginx] expected a redirect for Host %q", nginxHostHeader)
	assert.Equal(s.T(), http.StatusPermanentRedirect, traefikResp.StatusCode,
		"[traefik] expected a redirect for Host %q", traefikHostHeader)

	// The malformed port must not leak into the redirect target either.
	assertRedirectTarget(s.T(), nginxResp, wwwAuthNginxHost, "/admin/secret", "nginx")
	assertRedirectTarget(s.T(), traefikResp, wwwAuthTraefikHost, "/admin/secret", "traefik")
}
