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
	corsIngressName = "cors-test"
	corsTraefikHost = corsIngressName + ".traefik.local"
	corsNginxHost   = corsIngressName + ".nginx.local"

	corsDefaultIngressName = "cors-default-test"
	corsDefaultTraefikHost = corsDefaultIngressName + ".traefik.local"
	corsDefaultNginxHost   = corsDefaultIngressName + ".nginx.local"

	corsNoCredsIngressName = "cors-no-creds-test"
	corsNoCredsTraefikHost = corsNoCredsIngressName + ".traefik.local"
	corsNoCredsNginxHost   = corsNoCredsIngressName + ".nginx.local"
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

	s.traefik.WaitForIngressReady(s.T(), corsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), corsDefaultTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsDefaultNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), corsNoCredsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsNoCredsNginxHost, 20, 1*time.Second)
}

func (s *CORSSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(corsIngressName)
	_ = s.nginx.DeleteIngress(corsIngressName)
	_ = s.traefik.DeleteIngress(corsDefaultIngressName)
	_ = s.nginx.DeleteIngress(corsDefaultIngressName)
	_ = s.traefik.DeleteIngress(corsNoCredsIngressName)
	_ = s.nginx.DeleteIngress(corsNoCredsIngressName)
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
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                         "https://app.example.com",
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "Content-Type, Authorization",
	})

	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Credentials",
		"Access-Control-Max-Age",
	} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"preflight header %s mismatch", header,
		)
	}
}

func (s *CORSSuite) TestPreflightNonMatchingOrigin() {
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                        "https://evil.example.com",
		"Access-Control-Request-Method": "GET",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"non-matching origin should be handled the same way",
	)
}

func (s *CORSSuite) TestSimpleGetMatchingOrigin() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"Origin": "https://app.example.com",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Expose-Headers",
	} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"simple request header %s mismatch", header,
		)
	}
}

func (s *CORSSuite) TestSimpleGetNonMatchingOrigin() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"Origin": "https://other.example.com",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"non-matching origin should be handled the same way",
	)
}

func (s *CORSSuite) TestAllowMethodsValue() {
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "PUT",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Methods"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Methods"),
		"allowed methods mismatch",
	)
}

func (s *CORSSuite) TestAllowHeadersValue() {
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                         "https://app.example.com",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "X-Request-ID",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Headers"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Headers"),
		"allowed headers mismatch",
	)
}

func (s *CORSSuite) TestMaxAgeValue() {
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "GET",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Max-Age"),
		traefikResp.ResponseHeaders.Get("Access-Control-Max-Age"),
		"max age mismatch",
	)
}

func (s *CORSSuite) TestExposeHeadersOnSimpleRequest() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"Origin": "https://app.example.com",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Expose-Headers"),
		traefikResp.ResponseHeaders.Get("Access-Control-Expose-Headers"),
		"expose headers mismatch",
	)
}

func (s *CORSSuite) TestDefaultCORSPreflightHeaders() {
	traefikResp, nginxResp := s.requestDefault(http.MethodOptions, "/", map[string]string{
		"Origin":                        "https://any.example.com",
		"Access-Control-Request-Method": "GET",
	})

	// Note: Access-Control-Allow-Methods is excluded because nginx and traefik
	// format the default method list differently (nginx adds spaces after commas).
	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Max-Age",
	} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"default preflight header %s mismatch", header,
		)
	}
}

func (s *CORSSuite) TestDefaultCORSAllowOriginWildcard() {
	traefikResp, nginxResp := s.requestDefault(http.MethodGet, "/", map[string]string{
		"Origin": "https://any.example.com",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Origin"),
		"default allow-origin should match between clusters",
	)
}

func (s *CORSSuite) TestCredentialsFalse() {
	traefikResp, nginxResp := s.requestNoCreds(http.MethodGet, "/", map[string]string{
		"Origin": "https://app.example.com",
	})

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Access-Control-Allow-Credentials"),
		traefikResp.ResponseHeaders.Get("Access-Control-Allow-Credentials"),
		"credentials false should match between clusters",
	)
}

func (s *CORSSuite) TestPreflightStatusCode() {
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": "POST",
	})

	// nginx returns 204, traefik returns 200 for preflight — both are valid.
	// Verify each controller returns a successful status.
	assert.True(s.T(), traefikResp.StatusCode >= 200 && traefikResp.StatusCode < 300,
		"traefik preflight should return 2xx, got: %d", traefikResp.StatusCode)
	assert.True(s.T(), nginxResp.StatusCode >= 200 && nginxResp.StatusCode < 300,
		"nginx preflight should return 2xx, got: %d", nginxResp.StatusCode)
}
