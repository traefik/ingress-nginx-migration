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
	varsIngressName = "snippet-vars-test"
	varsTraefikHost = varsIngressName + ".traefik.local"
	varsNginxHost   = varsIngressName + ".nginx.local"
)

type SnippetVariablesSuite struct {
	BaseSuite
}

func TestSnippetVariablesSuite(t *testing.T) {
	suite.Run(t, new(SnippetVariablesSuite))
}

func (s *SnippetVariablesSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Expose each variable via proxy_set_header so the backend echoes it back.
	// Header names use hyphenated segments to match Go's HTTP canonical form,
	// since whoami echoes headers in canonical form and parseWhoamiHeaders
	// does a case-sensitive map lookup.
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
proxy_set_header X-Var-Uri $uri;
proxy_set_header X-Var-Doc-Uri $document_uri;
proxy_set_header X-Var-Host $host;
proxy_set_header X-Var-Server-Name $server_name;
proxy_set_header X-Var-Args $args;
proxy_set_header X-Var-Query-String $query_string;
proxy_set_header X-Var-Is-Args $is_args;
proxy_set_header X-Var-Content-Type $content_type;
proxy_set_header X-Var-Cookie $cookie_testcookie;
proxy_set_header X-Var-Arg-Token $arg_token;
proxy_set_header X-Var-Http-Custom $http_x_custom_var;
proxy_set_header X-Var-Server-Port $server_port;
proxy_set_header X-Var-Best-Http-Host $best_http_host;
proxy_set_header X-Var-Proxy-Add-Xff $proxy_add_x_forwarded_for;
`,
	}

	err := s.traefik.DeployIngress(varsIngressName, varsTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy vars ingress to traefik cluster")

	err = s.nginx.DeployIngress(varsIngressName, varsNginxHost, annotations)
	require.NoError(s.T(), err, "deploy vars ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), varsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), varsNginxHost, 20, 1*time.Second)
}

func (s *SnippetVariablesSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(varsIngressName)
	_ = s.nginx.DeleteIngress(varsIngressName)
}

func (s *SnippetVariablesSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), varsTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), varsNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- $uri / $document_uri ---

func (s *SnippetVariablesSuite) TestVarUri() {
	// $uri should return the path without the query string.
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test?q=hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Uri"],
		traefikResp.RequestHeaders["X-Var-Uri"],
		"$uri mismatch",
	)
	assert.Equal(s.T(), "/vars/test", traefikResp.RequestHeaders["X-Var-Uri"])
}

func (s *SnippetVariablesSuite) TestVarDocumentUri() {
	// $document_uri is an alias for $uri.
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test?q=hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Doc-Uri"],
		traefikResp.RequestHeaders["X-Var-Doc-Uri"],
		"$document_uri mismatch",
	)
	assert.Equal(s.T(), "/vars/test", traefikResp.RequestHeaders["X-Var-Doc-Uri"])
}

// --- $host / $server_name ---

func (s *SnippetVariablesSuite) TestVarHost() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $host should be the Host header value (without port).
	assert.Equal(s.T(), varsTraefikHost, traefikResp.RequestHeaders["X-Var-Host"],
		"traefik $host should match ingress host")
	assert.Equal(s.T(), varsNginxHost, nginxResp.RequestHeaders["X-Var-Host"],
		"nginx $host should match ingress host")
}

func (s *SnippetVariablesSuite) TestVarServerName() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $server_name should be the hostname without port.
	assert.Equal(s.T(), varsTraefikHost, traefikResp.RequestHeaders["X-Var-Server-Name"],
		"traefik $server_name should match ingress host")
	assert.Equal(s.T(), varsNginxHost, nginxResp.RequestHeaders["X-Var-Server-Name"],
		"nginx $server_name should match ingress host")
}

// --- $args / $query_string / $is_args / $arg_* ---

func (s *SnippetVariablesSuite) TestVarArgsPresent() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test?token=abc123&other=val", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Args"],
		traefikResp.RequestHeaders["X-Var-Args"],
		"$args mismatch",
	)
	assert.Contains(s.T(), traefikResp.RequestHeaders["X-Var-Args"], "token=abc123")
	assert.Contains(s.T(), traefikResp.RequestHeaders["X-Var-Args"], "other=val")
}

func (s *SnippetVariablesSuite) TestVarArgsAbsent() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Args"],
		traefikResp.RequestHeaders["X-Var-Args"],
		"$args mismatch when empty",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Args"])
}

func (s *SnippetVariablesSuite) TestVarQueryString() {
	// $query_string is an alias for $args.
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test?token=abc123&other=val", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Query-String"],
		traefikResp.RequestHeaders["X-Var-Query-String"],
		"$query_string mismatch",
	)
	assert.Contains(s.T(), traefikResp.RequestHeaders["X-Var-Query-String"], "token=abc123")
}

func (s *SnippetVariablesSuite) TestVarIsArgsPresent() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test?q=hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Is-Args"],
		traefikResp.RequestHeaders["X-Var-Is-Args"],
		"$is_args mismatch",
	)
	assert.Equal(s.T(), "?", traefikResp.RequestHeaders["X-Var-Is-Args"])
}

func (s *SnippetVariablesSuite) TestVarIsArgsAbsent() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Is-Args"],
		traefikResp.RequestHeaders["X-Var-Is-Args"],
		"$is_args mismatch when empty",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Is-Args"])
}

func (s *SnippetVariablesSuite) TestVarArgSpecific() {
	// $arg_token extracts the "token" query parameter.
	traefikResp, nginxResp := s.request(http.MethodGet, "/vars/test?token=abc123&other=val", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Arg-Token"],
		traefikResp.RequestHeaders["X-Var-Arg-Token"],
		"$arg_token mismatch",
	)
	assert.Equal(s.T(), "abc123", traefikResp.RequestHeaders["X-Var-Arg-Token"])
}

// --- $content_type ---

func (s *SnippetVariablesSuite) TestVarContentType() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"Content-Type": "application/json",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Content-Type"],
		traefikResp.RequestHeaders["X-Var-Content-Type"],
		"$content_type mismatch",
	)
	assert.Equal(s.T(), "application/json", traefikResp.RequestHeaders["X-Var-Content-Type"])
}

// --- $cookie_* ---

func (s *SnippetVariablesSuite) TestVarCookie() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"Cookie": "testcookie=cookie-val",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Cookie"],
		traefikResp.RequestHeaders["X-Var-Cookie"],
		"$cookie_testcookie mismatch",
	)
	assert.Equal(s.T(), "cookie-val", traefikResp.RequestHeaders["X-Var-Cookie"])
}

func (s *SnippetVariablesSuite) TestVarCookieAbsent() {
	// Without the cookie, $cookie_testcookie should be empty.
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Cookie"],
		traefikResp.RequestHeaders["X-Var-Cookie"],
		"$cookie_testcookie mismatch when absent",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Cookie"])
}

// --- $http_* ---

func (s *SnippetVariablesSuite) TestVarHttpHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"X-Custom-Var": "custom-val",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Http-Custom"],
		traefikResp.RequestHeaders["X-Var-Http-Custom"],
		"$http_x_custom_var mismatch",
	)
	assert.Equal(s.T(), "custom-val", traefikResp.RequestHeaders["X-Var-Http-Custom"])
}

func (s *SnippetVariablesSuite) TestVarHttpHeaderAbsent() {
	// Without the header, $http_x_custom_var should be empty.
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Http-Custom"],
		traefikResp.RequestHeaders["X-Var-Http-Custom"],
		"$http_x_custom_var mismatch when absent",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Http-Custom"])
}

// --- $server_port ---

func (s *SnippetVariablesSuite) TestVarServerPort() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $server_port should return the port the server is listening on.
	// In k3s both controllers listen on port 80 for HTTP.
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Server-Port"],
		traefikResp.RequestHeaders["X-Var-Server-Port"],
		"$server_port mismatch",
	)
	assert.NotEmpty(s.T(), traefikResp.RequestHeaders["X-Var-Server-Port"],
		"$server_port should not be empty")
}

// --- $best_http_host ---

func (s *SnippetVariablesSuite) TestVarBestHttpHost() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $best_http_host preserves the port in the Host header.
	// Each controller uses its own hostname, so compare individually.
	assert.Equal(s.T(), varsTraefikHost, traefikResp.RequestHeaders["X-Var-Best-Http-Host"],
		"traefik $best_http_host should match ingress host")
	assert.Equal(s.T(), varsNginxHost, nginxResp.RequestHeaders["X-Var-Best-Http-Host"],
		"nginx $best_http_host should match ingress host")
}

// --- $proxy_add_x_forwarded_for ---

func (s *SnippetVariablesSuite) TestVarProxyAddXForwardedFor() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Without an existing X-Forwarded-For header, $proxy_add_x_forwarded_for
	// should be the client's remote address.
	assert.NotEmpty(s.T(), traefikResp.RequestHeaders["X-Var-Proxy-Add-Xff"],
		"$proxy_add_x_forwarded_for should not be empty")
	assert.NotEmpty(s.T(), nginxResp.RequestHeaders["X-Var-Proxy-Add-Xff"],
		"$proxy_add_x_forwarded_for should not be empty")
}
