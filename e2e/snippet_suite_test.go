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
	snippetIngressName = "snippet-test"
	snippetTraefikHost = snippetIngressName + ".traefik.local"
	snippetNginxHost   = snippetIngressName + ".nginx.local"
)

type SnippetSuite struct {
	BaseSuite
}

func TestSnippetSuite(t *testing.T) {
	suite.Run(t, new(SnippetSuite))
}

func (s *SnippetSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
location = /exact {
    return 200 "exact-match";
}

location ~ ^/regex/v[0-9]+/ {
    return 200 "versioned";
}

location /loc-headers {
    add_header X-Loc-Add "add-value";
    more_set_headers "X-Loc-More:more-value";
    return 200 "with-headers";
}

location /loc-return {
    return 403 "Forbidden";
}

location /inherit-test {
    add_header X-Level "location";
    return 200 "inherited";
}

add_header X-Level "root";
`,
		"nginx.ingress.kubernetes.io/configuration-snippet": `
add_header X-Method $request_method;
set $my_var "hello";
add_header X-My-Var $my_var;
set $combined "$request_method-$request_uri";
add_header X-Combined $combined;
proxy_set_header X-Backend-Method $request_method;
proxy_set_header X-Backend-Uri $request_uri;
more_set_input_headers "X-Custom-Input:input-value";
more_set_input_headers "X-Method-Input:$request_method";
more_set_headers "X-Config-More:config-value";
if ($request_method = POST) {
    return 405 "Method Not Allowed";
}
`,
	}

	err := s.traefik.DeployIngress(snippetIngressName, snippetTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy snippet ingress to traefik cluster")

	err = s.nginx.DeployIngress(snippetIngressName, snippetNginxHost, annotations)
	require.NoError(s.T(), err, "deploy snippet ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), snippetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), snippetNginxHost, 20, 1*time.Second)
}

func (s *SnippetSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(snippetIngressName)
	_ = s.nginx.DeleteIngress(snippetIngressName)
}

// request makes the same HTTP request against both clusters and returns both responses.
func (s *SnippetSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), snippetTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), snippetNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// Configuration-snippet tests — exercised on the default path (GET /).

func (s *SnippetSuite) TestVariableInterpolation() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Method", "X-My-Var", "X-Combined"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
	}
}

func (s *SnippetSuite) TestProxySetHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Backend-Method", "X-Backend-Uri"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
	}
}

func (s *SnippetSuite) TestMoreSetInputHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Custom-Input", "X-Method-Input"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
	}
}

func (s *SnippetSuite) TestMoreSetHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Config-More"),
		traefikResp.ResponseHeaders.Get("X-Config-More"),
		"more_set_headers value mismatch",
	)
}

func (s *SnippetSuite) TestIfDirectiveReturn() {
	traefikResp, nginxResp := s.request(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

// Server-snippet location tests — each exercised via a specific path.

func (s *SnippetSuite) TestLocationExactMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/exact", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

func (s *SnippetSuite) TestLocationExactNoMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/exact/more", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// Should fall through to default location (whoami backend).
	assert.NotEqual(s.T(), "exact-match", traefikResp.Body, "should not match exact location")
}

func (s *SnippetSuite) TestLocationRegexMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/regex/v2/users", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

func (s *SnippetSuite) TestLocationRegexNoMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/regex/latest/users", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// Should fall through to default location (whoami backend).
	assert.NotEqual(s.T(), "versioned", traefikResp.Body, "should not match regex location")
}

func (s *SnippetSuite) TestLocationWithHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/loc-headers", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")

	for _, header := range []string{"X-Loc-Add", "X-Loc-More"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
	}
}

func (s *SnippetSuite) TestLocationReturn() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/loc-return", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

func (s *SnippetSuite) TestAddHeaderInheritance() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/inherit-test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// The location block has its own add_header X-Level "location",
	// which overrides the server-level add_header X-Level "root".
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Level"),
		traefikResp.ResponseHeaders.Get("X-Level"),
		"add_header inheritance mismatch",
	)
}
