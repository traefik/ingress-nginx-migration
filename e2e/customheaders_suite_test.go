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
	customHeadersIngressName = "custom-headers-test"
	customHeadersTraefikHost = customHeadersIngressName + ".traefik.local"
	customHeadersNginxHost   = customHeadersIngressName + ".nginx.local"
)

type CustomHeadersSuite struct {
	BaseSuite
}

func TestCustomHeadersSuite(t *testing.T) {
	suite.Run(t, new(CustomHeadersSuite))
}

func (s *CustomHeadersSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
proxy_set_header X-Custom-Req "custom-request-value";
proxy_set_header X-Forwarded-Proto "https";
more_set_input_headers "X-Input-Static: static-input";
add_header X-Custom-Resp "custom-response-value" always;
add_header X-Frame-Options "DENY" always;
more_set_headers "X-More-Resp: more-response-value";
`,
	}

	err := s.traefik.DeployIngress(customHeadersIngressName, customHeadersTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy custom-headers ingress to traefik cluster")

	err = s.nginx.DeployIngress(customHeadersIngressName, customHeadersNginxHost, annotations)
	require.NoError(s.T(), err, "deploy custom-headers ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), customHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), customHeadersNginxHost, 20, 1*time.Second)
}

func (s *CustomHeadersSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(customHeadersIngressName)
	_ = s.nginx.DeleteIngress(customHeadersIngressName)
}

func (s *CustomHeadersSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), customHeadersTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), customHeadersNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// Custom request headers — verified via whoami backend body.

func (s *CustomHeadersSuite) TestProxySetHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Custom-Req"],
		traefikResp.RequestHeaders["X-Custom-Req"],
		"proxy_set_header mismatch",
	)
}

func (s *CustomHeadersSuite) TestForwardedProtoOverride() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Forwarded-Proto"],
		traefikResp.RequestHeaders["X-Forwarded-Proto"],
		"X-Forwarded-Proto override mismatch",
	)
}

func (s *CustomHeadersSuite) TestMoreSetInputHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Input-Static"],
		traefikResp.RequestHeaders["X-Input-Static"],
		"more_set_input_headers mismatch",
	)
}

// Custom response headers — verified via HTTP response headers.

func (s *CustomHeadersSuite) TestAddResponseHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Custom-Resp"),
		traefikResp.ResponseHeaders.Get("X-Custom-Resp"),
		"add_header response mismatch",
	)
}

func (s *CustomHeadersSuite) TestSecurityResponseHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Frame-Options"),
		traefikResp.ResponseHeaders.Get("X-Frame-Options"),
		"X-Frame-Options mismatch",
	)
}

func (s *CustomHeadersSuite) TestMoreSetResponseHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-More-Resp"),
		traefikResp.ResponseHeaders.Get("X-More-Resp"),
		"more_set_headers response mismatch",
	)
}

// Client-originated headers.

func (s *CustomHeadersSuite) TestClientHeaderPassthrough() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"X-Client-Custom": "from-client",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Client-Custom"],
		traefikResp.RequestHeaders["X-Client-Custom"],
		"client header passthrough mismatch",
	)
}

// Combined verification.

func (s *CustomHeadersSuite) TestAllRequestHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	for _, header := range []string{"X-Custom-Req", "X-Forwarded-Proto", "X-Input-Static"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
	}
}

func (s *CustomHeadersSuite) TestAllResponseHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	for _, header := range []string{"X-Custom-Resp", "X-Frame-Options", "X-More-Resp"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
	}
}
