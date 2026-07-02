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
	corsIngressName = "cors-test"
	corsTraefikHost = corsIngressName + ".traefik.local"
	corsNginxHost   = corsIngressName + ".nginx.local"
	corsGatewayHost = corsIngressName + ".gateway.local"

	corsDefaultIngressName = "cors-default-test"
	corsDefaultTraefikHost = corsDefaultIngressName + ".traefik.local"
	corsDefaultNginxHost   = corsDefaultIngressName + ".nginx.local"
	corsDefaultGatewayHost = corsDefaultIngressName + ".gateway.local"

	corsNoCredsIngressName = "cors-no-creds-test"
	corsNoCredsTraefikHost = corsNoCredsIngressName + ".traefik.local"
	corsNoCredsNginxHost   = corsNoCredsIngressName + ".nginx.local"
	corsNoCredsGatewayHost = corsNoCredsIngressName + ".gateway.local"
)

type CORSSuite struct {
	BaseSuite
}

func TestCORSSuite(t *testing.T) {
	suite.Run(t, new(CORSSuite))
}

func (s *CORSSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/enable-cors":             "true",
		"nginx.ingress.kubernetes.io/cors-allow-origin":       "https://app.example.com",
		"nginx.ingress.kubernetes.io/cors-allow-methods":      "GET,POST,PUT",
		"nginx.ingress.kubernetes.io/cors-allow-headers":      "Content-Type,Authorization,X-Request-ID",
		"nginx.ingress.kubernetes.io/cors-allow-credentials":  "true",
		"nginx.ingress.kubernetes.io/cors-max-age":            "7200",
		"nginx.ingress.kubernetes.io/cors-expose-headers":     "X-Request-ID,X-Trace-ID",
	}

	err := s.traefik.DeployIngress(corsIngressName, corsTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy CORS ingress to traefik cluster")

	err = s.nginx.DeployIngress(corsIngressName, corsNginxHost, annotations)
	require.NoError(s.T(), err, "deploy CORS ingress to nginx cluster")

	// Deploy defaults-only ingress (enable-cors: "true", no other CORS annotations).
	defaultAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/enable-cors": "true",
	}

	err = s.traefik.DeployIngress(corsDefaultIngressName, corsDefaultTraefikHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy CORS default ingress to traefik cluster")

	err = s.nginx.DeployIngress(corsDefaultIngressName, corsDefaultNginxHost, defaultAnnotations)
	require.NoError(s.T(), err, "deploy CORS default ingress to nginx cluster")

	// Deploy no-credentials ingress (cors-allow-credentials: "false").
	noCredsAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/enable-cors":            "true",
		"nginx.ingress.kubernetes.io/cors-allow-origin":      "https://app.example.com",
		"nginx.ingress.kubernetes.io/cors-allow-credentials": "false",
	}

	err = s.traefik.DeployIngress(corsNoCredsIngressName, corsNoCredsTraefikHost, noCredsAnnotations)
	require.NoError(s.T(), err, "deploy CORS no-creds ingress to traefik cluster")

	err = s.nginx.DeployIngress(corsNoCredsIngressName, corsNoCredsNginxHost, noCredsAnnotations)
	require.NoError(s.T(), err, "deploy CORS no-creds ingress to nginx cluster")

	// Deploy Gateway API equivalents.
	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "cors", "custom.yaml"))
	require.NoError(s.T(), err, "deploy CORS custom gateway fixture")

	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "cors", "default.yaml"))
	require.NoError(s.T(), err, "deploy CORS default gateway fixture")

	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "cors", "no-creds.yaml"))
	require.NoError(s.T(), err, "deploy CORS no-creds gateway fixture")

	s.traefik.WaitForIngressReady(s.T(), corsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), corsDefaultTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsDefaultNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), corsNoCredsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsNoCredsNginxHost, 20, 1*time.Second)
	// Gateway API routes need more time — CRD provider must publish middleware config first.
	s.gateway.WaitForIngressReady(s.T(), corsGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), corsDefaultGatewayHost, 60, 1*time.Second)
	s.gateway.WaitForIngressReady(s.T(), corsNoCredsGatewayHost, 60, 1*time.Second)
}

func (s *CORSSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(corsIngressName)
	_ = s.nginx.DeleteIngress(corsIngressName)
	_ = s.traefik.DeleteIngress(corsDefaultIngressName)
	_ = s.nginx.DeleteIngress(corsDefaultIngressName)
	_ = s.traefik.DeleteIngress(corsNoCredsIngressName)
	_ = s.nginx.DeleteIngress(corsNoCredsIngressName)
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "cors", "custom.yaml"))
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "cors", "default.yaml"))
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "cors", "no-creds.yaml"))
}

// request makes the same HTTP request against both clusters and returns both responses.
func (s *CORSSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), corsTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), corsNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestDefault makes the same HTTP request against both clusters using the defaults-only ingress.
func (s *CORSSuite) requestDefault(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), corsDefaultTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), corsDefaultNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// gatewayRequest makes a request to the gateway cluster for the custom CORS variant.
func (s *CORSSuite) gatewayRequest(method, path string, headers map[string]string) *Response {
	s.T().Helper()
	resp := s.gateway.MakeRequest(s.T(), corsGatewayHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), resp, "gateway response should not be nil")
	return resp
}

// gatewayRequestDefault makes a request to the gateway cluster for the defaults CORS variant.
func (s *CORSSuite) gatewayRequestDefault(method, path string, headers map[string]string) *Response {
	s.T().Helper()
	resp := s.gateway.MakeRequest(s.T(), corsDefaultGatewayHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), resp, "gateway default response should not be nil")
	return resp
}

// gatewayRequestNoCreds makes a request to the gateway cluster for the no-creds CORS variant.
func (s *CORSSuite) gatewayRequestNoCreds(method, path string, headers map[string]string) *Response {
	s.T().Helper()
	resp := s.gateway.MakeRequest(s.T(), corsNoCredsGatewayHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), resp, "gateway no-creds response should not be nil")
	return resp
}

// assertGatewayHeaders compares traefik-ingress response headers with gateway response headers.
func (s *CORSSuite) assertGatewayHeaders(traefikResp, gatewayResp *Response, headers []string, context string) {
	s.T().Helper()
	for _, h := range headers {
		assert.Equal(s.T(),
			traefikResp.ResponseHeaders.Get(h),
			gatewayResp.ResponseHeaders.Get(h),
			"gateway migration (%s): header %s mismatch", context, h,
		)
	}
}

// requestNoCreds makes the same HTTP request against both clusters using the no-credentials ingress.
func (s *CORSSuite) requestNoCreds(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), corsNoCredsTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), corsNoCredsNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *CORSSuite) TestPreflightMatchingOrigin() {
	preflightHeaders := map[string]string{
		"Origin":                         "https://app.example.com",
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "Content-Type, Authorization",
	}

	traefikResp, nginxResp := s.request(http.MethodOptions, "/", preflightHeaders)

	corsHeaders := []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Credentials",
		"Access-Control-Max-Age",
	}

	for _, header := range corsHeaders {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"preflight header %s mismatch", header,
		)
	}

	// Gateway API migration: compare traefik-ingress vs traefik-gateway.
	gatewayResp := s.gateway.MakeRequest(s.T(), corsGatewayHost, http.MethodOptions, "/", preflightHeaders, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")

	for _, header := range corsHeaders {
		assert.Equal(s.T(),
			traefikResp.ResponseHeaders.Get(header),
			gatewayResp.ResponseHeaders.Get(header),
			"gateway migration: preflight header %s mismatch", header,
		)
	}
}

func (s *CORSSuite) TestPreflightNonMatchingOrigin() {
	headers := map[string]string{
		"Origin":                        "https://evil.example.com",
		"Access-Control-Request-Method": "GET",
	}
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", headers)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"non-matching origin should be handled the same way",
	)

	gatewayResp := s.gatewayRequest(http.MethodOptions, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"gateway migration: non-matching origin mismatch",
	)
}

func (s *CORSSuite) TestSimpleGetMatchingOrigin() {
	headers := map[string]string{"Origin": "https://app.example.com"}
	traefikResp, nginxResp := s.request(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	corsHeaders := []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Expose-Headers",
	}
	for _, header := range corsHeaders {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"simple request header %s mismatch", header,
		)
	}

	gatewayResp := s.gatewayRequest(http.MethodGet, "/", headers)
	s.assertGatewayHeaders(traefikResp, gatewayResp, corsHeaders, "simple GET matching origin")
}

func (s *CORSSuite) TestSimpleGetNonMatchingOrigin() {
	headers := map[string]string{"Origin": "https://other.example.com"}
	traefikResp, nginxResp := s.request(http.MethodGet, "/", headers)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"non-matching origin should be handled the same way",
	)

	gatewayResp := s.gatewayRequest(http.MethodGet, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"gateway migration: non-matching origin mismatch",
	)
}

func (s *CORSSuite) TestAllowMethodsValue() {
	headers := map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "PUT",
	}
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", headers)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Methods"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Methods"),
		"allowed methods mismatch",
	)

	gatewayResp := s.gatewayRequest(http.MethodOptions, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Methods"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Allow-Methods"),
		"gateway migration: allowed methods mismatch",
	)
}

func (s *CORSSuite) TestAllowHeadersValue() {
	headers := map[string]string{
		"Origin":                         "https://app.example.com",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "X-Request-ID",
	}
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", headers)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Headers"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Headers"),
		"allowed headers mismatch",
	)

	gatewayResp := s.gatewayRequest(http.MethodOptions, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Headers"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Allow-Headers"),
		"gateway migration: allowed headers mismatch",
	)
}

func (s *CORSSuite) TestMaxAgeValue() {
	headers := map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "GET",
	}
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", headers)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Max-Age"),
		traefikResp.ResponseHeaders.Get("Access-Control-Max-Age"),
		"max age mismatch",
	)

	gatewayResp := s.gatewayRequest(http.MethodOptions, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Max-Age"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Max-Age"),
		"gateway migration: max age mismatch",
	)
}

func (s *CORSSuite) TestExposeHeadersOnSimpleRequest() {
	headers := map[string]string{"Origin": "https://app.example.com"}
	traefikResp, nginxResp := s.request(http.MethodGet, "/", headers)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Expose-Headers"),
		traefikResp.ResponseHeaders.Get("Access-Control-Expose-Headers"),
		"expose headers mismatch",
	)

	gatewayResp := s.gatewayRequest(http.MethodGet, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Expose-Headers"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Expose-Headers"),
		"gateway migration: expose headers mismatch",
	)
}

func (s *CORSSuite) TestDefaultCORSPreflightHeaders() {
	headers := map[string]string{
		"Origin":                        "https://any.example.com",
		"Access-Control-Request-Method": "GET",
	}
	traefikResp, nginxResp := s.requestDefault(http.MethodOptions, "/", headers)

	// Note: Access-Control-Allow-Methods is excluded because nginx and traefik
	// format the default method list differently (nginx adds spaces after commas).
	// Note: Access-Control-Allow-Origin is excluded from the nginx vs traefik comparison:
	// Traefik now echoes the request Origin when credentials=true (CORS spec compliant);
	// nginx returns "*" which is a CORS spec violation. See TestDefaultCORSAllowOriginWildcard.
	corsHeaders := []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Max-Age",
	}
	for _, header := range corsHeaders {
		if header == "Access-Control-Allow-Origin" {
			continue
		}
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"default preflight header %s mismatch", header,
		)
	}

	gatewayResp := s.gatewayRequestDefault(http.MethodOptions, "/", headers)
	s.assertGatewayHeaders(traefikResp, gatewayResp, corsHeaders, "default preflight")
}

func (s *CORSSuite) TestDefaultCORSAllowOriginWildcard() {
	headers := map[string]string{"Origin": "https://any.example.com"}
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", headers)

	// MIGRATION NOTE: nginx returns "*" even when credentials=true (CORS spec violation).
	// Traefik correctly echoes the request Origin when credentials=true (CORS spec compliant).
	// This is a known behavior difference: Traefik is more correct per the W3C CORS spec.
	assert.Equal(s.T(), "*", nginxResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"nginx default allow-origin should be wildcard")
	assert.NotEmpty(s.T(), traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"traefik default allow-origin should not be empty")

	// Gateway follows Traefik behavior (echoes origin per CORS spec).
	gatewayResp := s.gatewayRequestDefault(http.MethodGet, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"gateway migration: default allow-origin mismatch",
	)
}

func (s *CORSSuite) TestCredentialsFalse() {
	headers := map[string]string{"Origin": "https://app.example.com"}
	traefikResp, nginxResp := s.requestNoCreds(http.MethodGet, "/", headers)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Credentials"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Credentials"),
		"credentials false should match between clusters",
	)

	gatewayResp := s.gatewayRequestNoCreds(http.MethodGet, "/", headers)
	assert.Equal(s.T(),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Credentials"),
		gatewayResp.ResponseHeaders.Get("Access-Control-Allow-Credentials"),
		"gateway migration: credentials false mismatch",
	)
}

func (s *CORSSuite) TestPreflightStatusCode() {
	headers := map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "POST",
	}
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", headers)

	// nginx returns 204, traefik returns 200 for preflight — both are valid.
	assert.True(s.T(), traefikResp.StatusCode >= 200 && traefikResp.StatusCode < 300,
		"traefik preflight should return 2xx, got: %d", traefikResp.StatusCode)
	assert.True(s.T(), nginxResp.StatusCode >= 200 && nginxResp.StatusCode < 300,
		"nginx preflight should return 2xx, got: %d", nginxResp.StatusCode)

	gatewayResp := s.gatewayRequest(http.MethodOptions, "/", headers)
	assert.True(s.T(), gatewayResp.StatusCode >= 200 && gatewayResp.StatusCode < 300,
		"gateway preflight should return 2xx, got: %d", gatewayResp.StatusCode)
}
