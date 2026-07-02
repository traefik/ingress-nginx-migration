package e2e

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	affinityDefaultIngressName  = "affinity-default-test"
	affinityDefaultTraefikHost  = affinityDefaultIngressName + ".traefik.local"
	affinityDefaultNginxHost    = affinityDefaultIngressName + ".nginx.local"
	affinityDefaultGatewayHost  = affinityDefaultIngressName + ".gateway.local"

	affinityCustomIngressName  = "affinity-custom-test"
	affinityCustomTraefikHost  = affinityCustomIngressName + ".traefik.local"
	affinityCustomNginxHost    = affinityCustomIngressName + ".nginx.local"
	affinityCustomGatewayHost  = affinityCustomIngressName + ".gateway.local"

	affinityExtendedIngressName  = "affinity-extended-test"
	affinityExtendedTraefikHost  = affinityExtendedIngressName + ".traefik.local"
	affinityExtendedNginxHost    = affinityExtendedIngressName + ".nginx.local"
	affinityExtendedGatewayHost  = affinityExtendedIngressName + ".gateway.local"
)

type SessionAffinitySuite struct {
	BaseSuite
}

func TestSessionAffinitySuite(t *testing.T) {
	suite.Run(t, new(SessionAffinitySuite))
}

func (s *SessionAffinitySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// nginx-ingress Lua balancer merges sticky-session configs per upstream (service:port).
	// When multiple ingresses route to the same "backend:80" upstream, the last-applied
	// sticky config wins for ALL of them. Deploy isolated services so each ingress gets
	// its own Lua upstream instance and its own cookie configuration.
	for _, svcName := range []string{"backend-sa-default", "backend-sa-custom", "backend-sa-extended"} {
		manifest := "apiVersion: v1\nkind: Service\nmetadata:\n  name: " + svcName + "\nspec:\n  selector:\n    app: backend\n  ports:\n  - port: 80\n    targetPort: 80"
		err := s.nginx.ApplyManifest(manifest)
		require.NoError(s.T(), err, "deploy isolated service %s", svcName)
	}

	// Default affinity: cookie with default settings.
	defaultAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/affinity": "cookie",
	}

	err := s.traefik.DeployIngress(affinityDefaultIngressName, affinityDefaultTraefikHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy default affinity ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        affinityDefaultIngressName,
		Host:        affinityDefaultNginxHost,
		Annotations: defaultAnnotations,
		ServiceName: "backend-sa-default",
		ServicePort: 80,
	})
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

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        affinityCustomIngressName,
		Host:        affinityCustomNginxHost,
		Annotations: customAnnotations,
		ServiceName: "backend-sa-custom",
		ServicePort: 80,
	})
	require.NoError(s.T(), err, "deploy custom affinity ingress to nginx cluster")

	// Extended affinity with domain and expires.
	extendedAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/affinity":               "cookie",
		"nginx.ingress.kubernetes.io/session-cookie-name":    "EXTSESSION",
		"nginx.ingress.kubernetes.io/session-cookie-domain":  ".example.com",
		"nginx.ingress.kubernetes.io/session-cookie-expires": "172800",
	}

	err = s.traefik.DeployIngress(affinityExtendedIngressName, affinityExtendedTraefikHost, extendedAnnotations)
	require.NoError(s.T(), err, "deploy extended affinity ingress to traefik cluster")

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        affinityExtendedIngressName,
		Host:        affinityExtendedNginxHost,
		Annotations: extendedAnnotations,
		ServiceName: "backend-sa-extended",
		ServicePort: 80,
	})
	require.NoError(s.T(), err, "deploy extended affinity ingress to nginx cluster")

	// Deploy Gateway API equivalents using TraefikService sticky cookie CRDs.
	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "sessionaffinity", "default.yaml"))
	require.NoError(s.T(), err, "deploy default affinity gateway fixture")

	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "sessionaffinity", "custom.yaml"))
	require.NoError(s.T(), err, "deploy custom affinity gateway fixture")

	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "sessionaffinity", "extended.yaml"))
	require.NoError(s.T(), err, "deploy extended affinity gateway fixture")

	s.traefik.WaitForIngressReady(s.T(), affinityDefaultTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), affinityDefaultNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), affinityCustomTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), affinityCustomNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), affinityExtendedTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), affinityExtendedNginxHost, 20, 1*time.Second)
	// Gateway API routes need more time — CRD provider must publish TraefikService config first.
	s.gateway.WaitForIngressReady(s.T(), affinityDefaultGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), affinityCustomGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), affinityExtendedGatewayHost, 60, 1*time.Second)
}

func (s *SessionAffinitySuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(affinityDefaultIngressName)
	_ = s.nginx.DeleteIngress(affinityDefaultIngressName)
	_ = s.traefik.DeleteIngress(affinityCustomIngressName)
	_ = s.nginx.DeleteIngress(affinityCustomIngressName)
	_ = s.traefik.DeleteIngress(affinityExtendedIngressName)
	_ = s.nginx.DeleteIngress(affinityExtendedIngressName)
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "sessionaffinity", "default.yaml"))
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "sessionaffinity", "custom.yaml"))
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "sessionaffinity", "extended.yaml"))
	for _, svcName := range []string{"backend-sa-default", "backend-sa-custom", "backend-sa-extended"} {
		_ = s.nginx.Kubectl("delete", "service", svcName, "-n", testNamespace, "--ignore-not-found")
	}
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

	// Gateway API migration: TraefikService sticky cookie should also set INGRESSCOOKIE.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityDefaultGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "INGRESSCOOKIE")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set INGRESSCOOKIE")
}

func (s *SessionAffinitySuite) TestDefaultCookiePath() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "INGRESSCOOKIE")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "INGRESSCOOKIE")

	// Default path should be "/".
	assert.Contains(s.T(), traefikCookie, "Path=/", "traefik cookie should have default Path=/")
	assert.Contains(s.T(), nginxCookie, "Path=/", "nginx cookie should have default Path=/")

	// Gateway API migration: default cookie path should be "/".
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityDefaultGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "INGRESSCOOKIE")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set INGRESSCOOKIE")
	assert.Contains(s.T(), gatewayCookie, "Path=/", "gateway cookie should have default Path=/")
}

func (s *SessionAffinitySuite) TestCustomCookieName() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set SERVERID cookie")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set SERVERID cookie")

	// Gateway API migration: custom cookie name SERVERID should be set.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityCustomGatewayHost, http.MethodGet, "/app", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "SERVERID")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set SERVERID cookie")
}

func (s *SessionAffinitySuite) TestCustomCookieSecure() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "Secure", "traefik cookie should have Secure flag")
	assert.Contains(s.T(), nginxCookie, "Secure", "nginx cookie should have Secure flag")

	// Gateway API migration: Secure flag should be present on the SERVERID cookie.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityCustomGatewayHost, http.MethodGet, "/app", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "SERVERID")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set SERVERID cookie")
	assert.Contains(s.T(), gatewayCookie, "Secure", "gateway cookie should have Secure flag")
}

func (s *SessionAffinitySuite) TestCustomCookiePath() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "Path=/app", "traefik cookie should have Path=/app")
	assert.Contains(s.T(), nginxCookie, "Path=/app", "nginx cookie should have Path=/app")

	// Gateway API migration: cookie path /app should be set.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityCustomGatewayHost, http.MethodGet, "/app", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "SERVERID")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set SERVERID cookie")
	assert.Contains(s.T(), gatewayCookie, "Path=/app", "gateway cookie should have Path=/app")
}

func (s *SessionAffinitySuite) TestCustomCookieSameSite() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "SameSite=Strict", "traefik cookie should have SameSite=Strict")
	assert.Contains(s.T(), nginxCookie, "SameSite=Strict", "nginx cookie should have SameSite=Strict")

	// Gateway API migration: SameSite=Strict should be present.
	// Note: Traefik CRD uses lowercase sameSite value ("strict") but the HTTP
	// Set-Cookie header is emitted as "SameSite=Strict" by the Go HTTP stack.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityCustomGatewayHost, http.MethodGet, "/app", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "SERVERID")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set SERVERID cookie")
	assert.Contains(s.T(), strings.ToLower(gatewayCookie), "samesite=strict", "gateway cookie should have SameSite=Strict")
}

func (s *SessionAffinitySuite) TestCustomCookieMaxAge() {
	traefikResp, nginxResp := s.requestCustom(http.MethodGet, "/app", nil)

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "SERVERID")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "SERVERID")

	assert.Contains(s.T(), traefikCookie, "Max-Age=3600", "traefik cookie should have Max-Age=3600")
	assert.Contains(s.T(), nginxCookie, "Max-Age=3600", "nginx cookie should have Max-Age=3600")

	// Gateway API migration: Max-Age=3600 should be present.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityCustomGatewayHost, http.MethodGet, "/app", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "SERVERID")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set SERVERID cookie")
	assert.Contains(s.T(), gatewayCookie, "Max-Age=3600", "gateway cookie should have Max-Age=3600")
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

	// Gateway API migration: first request sets INGRESSCOOKIE, second request with cookie succeeds.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityDefaultGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "INGRESSCOOKIE")
	require.NotEmpty(s.T(), gatewayCookie, "gateway should set INGRESSCOOKIE")

	gatewayValue := strings.SplitN(gatewayCookie, ";", 2)[0]
	gatewayResp2 := s.gateway.MakeRequest(s.T(), affinityDefaultGatewayHost, http.MethodGet, "/",
		map[string]string{"Cookie": gatewayValue}, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp2, "gateway response with cookie should not be nil")
	assert.Equal(s.T(), http.StatusOK, gatewayResp2.StatusCode, "gateway should return 200 with cookie")
}

// requestExtended makes the same HTTP request against both clusters using the extended affinity ingress.
func (s *SessionAffinitySuite) requestExtended(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), affinityExtendedTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), affinityExtendedNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SessionAffinitySuite) TestExtendedCookieDomain() {
	traefikResp, nginxResp := s.requestExtended(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "EXTSESSION")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "EXTSESSION")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set EXTSESSION cookie")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set EXTSESSION cookie")

	assert.Contains(s.T(), traefikCookie, "Domain=.example.com", "traefik cookie should have Domain=.example.com")
	assert.Contains(s.T(), nginxCookie, "Domain=.example.com", "nginx cookie should have Domain=.example.com")

	// Gateway API migration: Traefik's sticky cookie does not support domain directly.
	// This is a known migration gap — the EXTSESSION cookie is set but without Domain=.example.com.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityExtendedGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	assert.Equal(s.T(), traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "EXTSESSION")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set EXTSESSION cookie")
	// Migration gap: domain is not supported by TraefikService sticky cookie.
	// assert.Contains(s.T(), gatewayCookie, "Domain=.example.com", "gateway cookie domain not supported")
}

func (s *SessionAffinitySuite) TestExtendedCookieExpires() {
	traefikResp, nginxResp := s.requestExtended(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	traefikCookie := findCookie(traefikResp.ResponseHeaders, "EXTSESSION")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "EXTSESSION")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set EXTSESSION cookie")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set EXTSESSION cookie")

	// session-cookie-expires=172800 should result in the cookie having an expiry.
	traefikHasExpiry := strings.Contains(traefikCookie, "Expires=") || strings.Contains(traefikCookie, "Max-Age=")
	nginxHasExpiry := strings.Contains(nginxCookie, "Expires=") || strings.Contains(nginxCookie, "Max-Age=")

	assert.True(s.T(), traefikHasExpiry, "traefik cookie should have Expires or Max-Age set")
	assert.True(s.T(), nginxHasExpiry, "nginx cookie should have Expires or Max-Age set")

	// Gateway API migration: expires is mapped to maxAge: 172800 in the TraefikService sticky cookie.
	gatewayResp := s.gateway.MakeRequest(s.T(), affinityExtendedGatewayHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")
	gatewayCookie := findCookie(gatewayResp.ResponseHeaders, "EXTSESSION")
	assert.NotEmpty(s.T(), gatewayCookie, "gateway should set EXTSESSION cookie")
	gatewayHasExpiry := strings.Contains(gatewayCookie, "Expires=") || strings.Contains(gatewayCookie, "Max-Age=")
	assert.True(s.T(), gatewayHasExpiry, "gateway cookie should have Expires or Max-Age set")
	assert.Contains(s.T(), gatewayCookie, "Max-Age=172800", "gateway cookie should have Max-Age=172800")
}
