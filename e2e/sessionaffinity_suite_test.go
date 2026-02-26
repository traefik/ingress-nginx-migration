package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	affinityDefaultIngressName = "affinity-default-test"
	affinityDefaultTraefikHost = affinityDefaultIngressName + ".traefik.local"
	affinityDefaultNginxHost   = affinityDefaultIngressName + ".nginx.local"

	affinityCustomIngressName = "affinity-custom-test"
	affinityCustomTraefikHost = affinityCustomIngressName + ".traefik.local"
	affinityCustomNginxHost   = affinityCustomIngressName + ".nginx.local"
)

type SessionAffinitySuite struct {
	BaseSuite
}

func TestSessionAffinitySuite(t *testing.T) {
	suite.Run(t, new(SessionAffinitySuite))
}

func (s *SessionAffinitySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Default affinity: cookie with default settings.
	defaultAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/affinity": "cookie",
	}

	err := s.traefik.DeployIngress(affinityDefaultIngressName, affinityDefaultTraefikHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy default affinity ingress to traefik cluster")

	err = s.nginx.DeployIngress(affinityDefaultIngressName, affinityDefaultNginxHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy default affinity ingress to nginx cluster")

	// Custom affinity with all cookie parameters.
	customAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/affinity":                "cookie",
		"nginx.ingress.kubernetes.io/session-cookie-name":     "SERVERID",
		"nginx.ingress.kubernetes.io/session-cookie-secure":   "true",
		"nginx.ingress.kubernetes.io/session-cookie-path":     "/app",
		"nginx.ingress.kubernetes.io/session-cookie-samesite": "Strict",
		"nginx.ingress.kubernetes.io/session-cookie-max-age":  "3600",
	}

	err = s.traefik.DeployIngress(affinityCustomIngressName, affinityCustomTraefikHost, customAnnotations)
	require.NoError(s.T(), err, "deploy custom affinity ingress to traefik cluster")

	err = s.nginx.DeployIngress(affinityCustomIngressName, affinityCustomNginxHost, customAnnotations)
	require.NoError(s.T(), err, "deploy custom affinity ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), affinityDefaultTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), affinityDefaultNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), affinityCustomTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), affinityCustomNginxHost, 20, 1*time.Second)
}

func (s *SessionAffinitySuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(affinityDefaultIngressName)
	_ = s.nginx.DeleteIngress(affinityDefaultIngressName)
	_ = s.traefik.DeleteIngress(affinityCustomIngressName)
	_ = s.nginx.DeleteIngress(affinityCustomIngressName)
}

// requestDefault makes the same HTTP request against both clusters using the default affinity ingress.
func (s *SessionAffinitySuite) requestDefault(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), affinityDefaultTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), affinityDefaultNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestCustom makes the same HTTP request against both clusters using the custom affinity ingress.
func (s *SessionAffinitySuite) requestCustom(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), affinityCustomTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), affinityCustomNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// findCookie returns the Set-Cookie header value for a cookie with the given name prefix.
func findCookie(headers http.Header, name string) string {
	for _, cookie := range headers.Values("Set-Cookie") {
		if strings.HasPrefix(cookie, name+"=") {
			return cookie
		}
	}
	return ""
}

func (s *SessionAffinitySuite) TestDefaultCookiePresent() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	// Both controllers should set an affinity cookie.
	traefikCookie := findCookie(traefikResp.ResponseHeaders, "INGRESSCOOKIE")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "INGRESSCOOKIE")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set INGRESSCOOKIE")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set INGRESSCOOKIE")
}

func (s *SessionAffinitySuite) TestDefaultCookiePath() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "INGRESSCOOKIE")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "INGRESSCOOKIE")

	// Default path should be "/".
	assert.Contains(s.T(), traefikCookie, "Path=/", "traefik cookie should have default Path=/")
	assert.Contains(s.T(), nginxCookie, "Path=/", "nginx cookie should have default Path=/")
}

func (s *SessionAffinitySuite) TestCustomCookieName() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set SERVERID cookie")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set SERVERID cookie")
}

func (s *SessionAffinitySuite) TestCustomCookieSecure() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "Secure", "traefik cookie should have Secure flag")
	assert.Contains(s.T(), nginxCookie, "Secure", "nginx cookie should have Secure flag")
}

func (s *SessionAffinitySuite) TestCustomCookiePath() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "Path=/app", "traefik cookie should have Path=/app")
	assert.Contains(s.T(), nginxCookie, "Path=/app", "nginx cookie should have Path=/app")
}

func (s *SessionAffinitySuite) TestCustomCookieSameSite() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "SameSite=Strict", "traefik cookie should have SameSite=Strict")
	assert.Contains(s.T(), nginxCookie, "SameSite=Strict", "nginx cookie should have SameSite=Strict")
}

func (s *SessionAffinitySuite) TestCustomCookieMaxAge() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "Max-Age=3600", "traefik cookie should have Max-Age=3600")
	assert.Contains(s.T(), nginxCookie, "Max-Age=3600", "nginx cookie should have Max-Age=3600")
}

func (s *SessionAffinitySuite) TestStickySessionWithCookie() {
	// First request should receive a Set-Cookie.
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "INGRESSCOOKIE")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "INGRESSCOOKIE")

	require.NotEmpty(s.T(), traefikCookie, "traefik should set INGRESSCOOKIE")
	require.NotEmpty(s.T(), nginxCookie, "nginx should set INGRESSCOOKIE")

	// Extract cookie value for the second request.
	traefikValue := strings.SplitN(traefikCookie, ";", 2)[0]
	nginxValue := strings.SplitN(nginxCookie, ";", 2)[0]

	// Second request with cookie should succeed.
	traefikResp2 := s.traefik.MakeRequest(s.T(), affinityDefaultTraefikHost, http.MethodGet, "/",
		map[string]string{"Cookie": traefikValue}, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp2, "traefik response with cookie should not be nil")

	nginxResp2 := s.nginx.MakeRequest(s.T(), affinityDefaultNginxHost, http.MethodGet, "/",
		map[string]string{"Cookie": nginxValue}, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp2, "nginx response with cookie should not be nil")

	assert.Equal(s.T(), http.StatusOK, traefikResp2.StatusCode, "traefik should return 200 with cookie")
	assert.Equal(s.T(), http.StatusOK, nginxResp2.StatusCode, "nginx should return 200 with cookie")
}
