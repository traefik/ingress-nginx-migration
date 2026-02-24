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
		"nginx.ingress.kubernetes.io/cors-allow-methods":      "GET, POST, PUT",
		"nginx.ingress.kubernetes.io/cors-allow-headers":      "Content-Type, Authorization, X-Request-ID",
		"nginx.ingress.kubernetes.io/cors-allow-credentials":  "true",
		"nginx.ingress.kubernetes.io/cors-max-age":            "7200",
		"nginx.ingress.kubernetes.io/cors-expose-headers":     "X-Request-ID, X-Trace-ID",
	}

	err := s.traefik.DeployIngress(corsIngressName, corsTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy CORS ingress to traefik cluster")

	err = s.nginx.DeployIngress(corsIngressName, corsNginxHost, annotations)
	require.NoError(s.T(), err, "deploy CORS ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), corsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), corsNginxHost, 20, 1*time.Second)
}

func (s *CORSSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(corsIngressName)
	_ = s.nginx.DeleteIngress(corsIngressName)
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

func (s *CORSSuite) TestPreflightMatchingOrigin() {
	traefikResp, nginxResp := s.request(http.MethodOptions, "/", map[string]string{
		"Origin":                        "https://app.example.com",
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
		"Origin":                       "https://evil.example.com",
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
		"Origin":                       "https://app.example.com",
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
		"Origin":                       "https://app.example.com",
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
