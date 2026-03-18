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

	newDirIngressName = "snippet-newdir-test"
	newDirTraefikHost = newDirIngressName + ".traefik.local"
	newDirNginxHost   = newDirIngressName + ".nginx.local"

	denyAllIngressName = "snippet-deny-all-test"
	denyAllTraefikHost = denyAllIngressName + ".traefik.local"
	denyAllNginxHost   = denyAllIngressName + ".nginx.local"

	ifHeaderIngressName = "snippet-if-header-test"
	ifHeaderTraefikHost = ifHeaderIngressName + ".traefik.local"
	ifHeaderNginxHost   = ifHeaderIngressName + ".nginx.local"

	ifNegIngressName = "snippet-if-neg-test"
	ifNegTraefikHost = ifNegIngressName + ".traefik.local"
	ifNegNginxHost   = ifNegIngressName + ".nginx.local"

	ifVarIngressName = "snippet-if-var-test"
	ifVarTraefikHost = ifVarIngressName + ".traefik.local"
	ifVarNginxHost   = ifVarIngressName + ".nginx.local"

	locationCIIngressName = "snippet-location-ci-test"
	locationCITraefikHost = locationCIIngressName + ".traefik.local"
	locationCINginxHost   = locationCIIngressName + ".nginx.local"

	varsIngressName = "snippet-vars-test"
	varsTraefikHost = varsIngressName + ".traefik.local"
	varsNginxHost   = varsIngressName + ".nginx.local"

	moreHeadersIngressName = "snippet-more-headers-test"
	moreHeadersTraefikHost = moreHeadersIngressName + ".traefik.local"
	moreHeadersNginxHost   = moreHeadersIngressName + ".nginx.local"

	rewriteSnippetIngressName = "snippet-rewrite-test"
	rewriteSnippetTraefikHost = rewriteSnippetIngressName + ".traefik.local"
	rewriteSnippetNginxHost   = rewriteSnippetIngressName + ".nginx.local"
)

type SnippetSuite struct {
	BaseSuite
}

func TestSnippetSuite(t *testing.T) {
	suite.Run(t, new(SnippetSuite))
}

func (s *SnippetSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// --- snippet ingress ---
	snippetAnnotations := map[string]string{
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

	err := s.traefik.DeployIngress(snippetIngressName, snippetTraefikHost, snippetAnnotations)
	require.NoError(s.T(), err, "deploy snippet ingress to traefik cluster")

	err = s.nginx.DeployIngress(snippetIngressName, snippetNginxHost, snippetAnnotations)
	require.NoError(s.T(), err, "deploy snippet ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), snippetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), snippetNginxHost, 20, 1*time.Second)

	// --- newdir ingress ---
	newDirAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
location /always-404 {
    add_header X-Custom "present" always;
    return 404 "Not Found";
}
location /noalways-404 {
    add_header X-Custom "present";
    return 404 "Not Found";
}
location /always-200 {
    add_header X-Custom "present" always;
    return 200 "OK";
}
location /noalways-200 {
    add_header X-Custom "present";
    return 200 "OK";
}
location /expires-epoch {
    expires epoch;
    return 200 "ok";
}
location /expires-max {
    expires max;
    return 200 "ok";
}
location /expires-off {
    expires off;
    return 200 "ok";
}
`,
		"nginx.ingress.kubernetes.io/configuration-snippet": `
more_set_headers "X-Append: first";
more_set_headers -a "X-Append: second";
more_set_input_headers -r "X-Restrict: restricted-value";
`,
	}

	err = s.traefik.DeployIngress(newDirIngressName, newDirTraefikHost, newDirAnnotations)
	require.NoError(s.T(), err, "deploy newdir ingress to traefik cluster")

	err = s.nginx.DeployIngress(newDirIngressName, newDirNginxHost, newDirAnnotations)
	require.NoError(s.T(), err, "deploy newdir ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), newDirTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), newDirNginxHost, 20, 1*time.Second)

	// --- deny-all ingress ---
	denyAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
deny all;
`,
	}

	err = s.traefik.DeployIngress(denyAllIngressName, denyAllTraefikHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy deny-all ingress to traefik cluster")

	err = s.nginx.DeployIngress(denyAllIngressName, denyAllNginxHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy deny-all ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), denyAllTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), denyAllNginxHost, 20, 1*time.Second)

	// --- if-header ingress ---
	headerAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
if ($http_x_check_equal = "expected") {
    more_set_headers "X-Equal-Matched: yes";
}
if ($request_uri ~ "^/api-check") {
    more_set_headers "X-Regex-Matched: true";
}
if ($http_x_check_ci ~* "^test") {
    more_set_headers "X-CI-Matched: yes";
}
`,
	}

	err = s.traefik.DeployIngress(ifHeaderIngressName, ifHeaderTraefikHost, headerAnnotations)
	require.NoError(s.T(), err, "deploy if-header ingress to traefik cluster")

	err = s.nginx.DeployIngress(ifHeaderIngressName, ifHeaderNginxHost, headerAnnotations)
	require.NoError(s.T(), err, "deploy if-header ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), ifHeaderTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), ifHeaderNginxHost, 20, 1*time.Second)

	// --- if-neg ingress ---
	negAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
if ($http_x_check_neg !~* "^admin") {
    more_set_headers "X-Neg-Matched: true";
}
`,
	}

	err = s.traefik.DeployIngress(ifNegIngressName, ifNegTraefikHost, negAnnotations)
	require.NoError(s.T(), err, "deploy if-neg ingress to traefik cluster")

	err = s.nginx.DeployIngress(ifNegIngressName, ifNegNginxHost, negAnnotations)
	require.NoError(s.T(), err, "deploy if-neg ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), ifNegTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), ifNegNginxHost, 20, 1*time.Second)

	// --- if-var ingress ---
	varAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
set $myflag "enabled";
if ($myflag) {
    more_set_headers "X-Flag-Matched: yes";
}
if ($request_uri ~ "^/capture/(.*)") {
    more_set_headers "X-Captured: $1";
}
`,
	}

	err = s.traefik.DeployIngress(ifVarIngressName, ifVarTraefikHost, varAnnotations)
	require.NoError(s.T(), err, "deploy if-var ingress to traefik cluster")

	err = s.nginx.DeployIngress(ifVarIngressName, ifVarNginxHost, varAnnotations)
	require.NoError(s.T(), err, "deploy if-var ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), ifVarTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), ifVarNginxHost, 20, 1*time.Second)

	// --- location-ci ingress ---
	locAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
location ~* \.css$ {
    add_header X-Type "css" always;
    return 200 "CSS";
}
`,
	}

	err = s.traefik.DeployIngress(locationCIIngressName, locationCITraefikHost, locAnnotations)
	require.NoError(s.T(), err, "deploy location-ci ingress to traefik cluster")

	err = s.nginx.DeployIngress(locationCIIngressName, locationCINginxHost, locAnnotations)
	require.NoError(s.T(), err, "deploy location-ci ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), locationCITraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), locationCINginxHost, 20, 1*time.Second)

	// --- vars ingress ---
	varsAnnotations := map[string]string{
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

	err = s.traefik.DeployIngress(varsIngressName, varsTraefikHost, varsAnnotations)
	require.NoError(s.T(), err, "deploy vars ingress to traefik cluster")

	err = s.nginx.DeployIngress(varsIngressName, varsNginxHost, varsAnnotations)
	require.NoError(s.T(), err, "deploy vars ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), varsTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), varsNginxHost, 20, 1*time.Second)

	// --- more-headers ingress ---
	moreHeadersAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
more_set_headers "X-Colon-Clear: colon-value";
more_set_headers "X-NoColon-Clear: nocolon-value";
more_set_headers "X-Server-Cross: server-cross-value";
`,
		"nginx.ingress.kubernetes.io/configuration-snippet": `
more_set_headers "X-Multi-A: a-val" "X-Multi-B: b-val";
more_set_input_headers "X-Input-Multi-A: ia-val" "X-Input-Multi-B: ib-val";
more_set_input_headers "X-Clear-Input";
more_set_headers "X-To-Clear: clear-me";
more_clear_headers "X-To-Clear";
more_set_headers "X-Clear-One: one-val";
more_set_headers "X-Clear-Two: two-val";
more_set_headers "X-Keep-This: keep-val";
more_clear_headers "X-Clear-One" "X-Clear-Two";
more_set_headers "X-Wild-One: w1";
more_set_headers "X-Wild-Two: w2";
more_set_headers "X-Other-Resp: other";
more_clear_headers "X-Wild-*";
more_clear_input_headers "X-Secret";
more_clear_input_headers "X-Prefix-*";
more_set_headers "X-Colon-Clear:";
more_set_headers "X-NoColon-Clear";
more_clear_headers "X-Server-Cross";
`,
	}

	err = s.traefik.DeployIngress(moreHeadersIngressName, moreHeadersTraefikHost, moreHeadersAnnotations)
	require.NoError(s.T(), err, "deploy more-headers ingress to traefik cluster")

	err = s.nginx.DeployIngress(moreHeadersIngressName, moreHeadersNginxHost, moreHeadersAnnotations)
	require.NoError(s.T(), err, "deploy more-headers ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), moreHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), moreHeadersNginxHost, 20, 1*time.Second)

	// --- rewrite ingress ---
	rewriteAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
rewrite ^/rw-last/(.*)$ /rw-dest/$1 last;
rewrite ^/rw-permanent$ /rw-perm-dest permanent;
rewrite ^/rw-redirect$ /rw-redir-dest redirect;
rewrite ^/rw-multicap/(.*)/media/(.*)\..*$ /rw-dest/$1/mp3/$2.mp3 last;
rewrite ^/rw-chain$ /rw-step;
rewrite ^/rw-step$ /rw-chain-done last;
rewrite ^/rw-url-redir$ http://other.example.com/new last;
`,
		"nginx.ingress.kubernetes.io/configuration-snippet": `
rewrite ^/rw-break/(.*)$ /rw-dest/$1 break;
rewrite ^/rw-cfg/(.*)$ /rw-cfg-dest/$1 last;
rewrite ^/rw-query$ /rw-dest last;
rewrite ^/rw-noquery$ /rw-dest? last;
if ($request_method = POST) {
    rewrite ^/rw-if/(.*)$ /rw-dest/$1 last;
}
`,
	}

	err = s.traefik.DeployIngress(rewriteSnippetIngressName, rewriteSnippetTraefikHost, rewriteAnnotations)
	require.NoError(s.T(), err, "deploy rewrite-snippet ingress to traefik cluster")

	err = s.nginx.DeployIngress(rewriteSnippetIngressName, rewriteSnippetNginxHost, rewriteAnnotations)
	require.NoError(s.T(), err, "deploy rewrite-snippet ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), rewriteSnippetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rewriteSnippetNginxHost, 20, 1*time.Second)
}

func (s *SnippetSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(snippetIngressName)
	_ = s.nginx.DeleteIngress(snippetIngressName)
	_ = s.traefik.DeleteIngress(newDirIngressName)
	_ = s.nginx.DeleteIngress(newDirIngressName)
	_ = s.traefik.DeleteIngress(denyAllIngressName)
	_ = s.nginx.DeleteIngress(denyAllIngressName)
	_ = s.traefik.DeleteIngress(ifHeaderIngressName)
	_ = s.nginx.DeleteIngress(ifHeaderIngressName)
	_ = s.traefik.DeleteIngress(ifNegIngressName)
	_ = s.nginx.DeleteIngress(ifNegIngressName)
	_ = s.traefik.DeleteIngress(ifVarIngressName)
	_ = s.nginx.DeleteIngress(ifVarIngressName)
	_ = s.traefik.DeleteIngress(locationCIIngressName)
	_ = s.nginx.DeleteIngress(locationCIIngressName)
	_ = s.traefik.DeleteIngress(varsIngressName)
	_ = s.nginx.DeleteIngress(varsIngressName)
	_ = s.traefik.DeleteIngress(moreHeadersIngressName)
	_ = s.nginx.DeleteIngress(moreHeadersIngressName)
	_ = s.traefik.DeleteIngress(rewriteSnippetIngressName)
	_ = s.nginx.DeleteIngress(rewriteSnippetIngressName)
}

// --- Helper methods ---

// requestSnippet makes the same HTTP request against both clusters using the snippet ingress host.
func (s *SnippetSuite) requestSnippet(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), snippetTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), snippetNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestNewDir(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), newDirTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), newDirNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestDenyAll() (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), denyAllTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), denyAllNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestIfHeader(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifHeaderTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifHeaderNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestIfNeg(headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifNegTraefikHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifNegNginxHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestIfVar(path string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifVarTraefikHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifVarNginxHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestLocationCI(path string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), locationCITraefikHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), locationCINginxHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestVars(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), varsTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), varsNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestMoreHeaders(headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), moreHeadersTraefikHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), moreHeadersNginxHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetSuite) requestRewrite(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rewriteSnippetTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rewriteSnippetNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// =============================================================================
// Tests from snippet_suite (configuration-snippet tests)
// =============================================================================

func (s *SnippetSuite) TestVariableInterpolation() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/", nil)

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
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/", nil)

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
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/", nil)

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
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Config-More"),
		traefikResp.ResponseHeaders.Get("X-Config-More"),
		"more_set_headers value mismatch",
	)
}

func (s *SnippetSuite) TestIfDirectiveReturn() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

// Server-snippet location tests

func (s *SnippetSuite) TestLocationExactMatch() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/exact", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

func (s *SnippetSuite) TestLocationExactNoMatch() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/exact/more", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// Should fall through to default location (whoami backend).
	assert.NotEqual(s.T(), "exact-match", traefikResp.Body, "should not match exact location")
}

func (s *SnippetSuite) TestLocationRegexMatch() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/regex/v2/users", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

func (s *SnippetSuite) TestLocationRegexNoMatch() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/regex/latest/users", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// Should fall through to default location (whoami backend).
	assert.NotEqual(s.T(), "versioned", traefikResp.Body, "should not match regex location")
}

func (s *SnippetSuite) TestLocationWithHeaders() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/loc-headers", nil)

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
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/loc-return", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
}

func (s *SnippetSuite) TestAddHeaderInheritance() {
	traefikResp, nginxResp := s.requestSnippet(http.MethodGet, "/inherit-test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// The location block has its own add_header X-Level "location",
	// which overrides the server-level add_header X-Level "root".
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Level"),
		traefikResp.ResponseHeaders.Get("X-Level"),
		"add_header inheritance mismatch",
	)
}

// =============================================================================
// Tests from snippet_newdirectives_suite (add_header always, expires, deny all, more_set_headers flags)
// =============================================================================

// --- add_header always tests ---

func (s *SnippetSuite) TestAddHeaderAlwaysOnError() {
	// add_header with "always" should apply even on 404 responses.
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/always-404", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusNotFound, traefikResp.StatusCode, "expected 404")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Custom"),
		traefikResp.ResponseHeaders.Get("X-Custom"),
		"X-Custom mismatch with always on error",
	)
	assert.Equal(s.T(), "present", traefikResp.ResponseHeaders.Get("X-Custom"),
		"add_header with always should be present on 404")
}

func (s *SnippetSuite) TestAddHeaderNoAlwaysOnError() {
	// add_header without "always" should NOT apply on 404 responses.
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/noalways-404", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusNotFound, traefikResp.StatusCode, "expected 404")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Custom"),
		traefikResp.ResponseHeaders.Get("X-Custom"),
		"X-Custom mismatch without always on error",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Custom"),
		"add_header without always should be absent on 404")
}

func (s *SnippetSuite) TestAddHeaderAlwaysOnSuccess() {
	// add_header with "always" should also apply on 200 responses.
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/always-200", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Custom"),
		traefikResp.ResponseHeaders.Get("X-Custom"),
		"X-Custom mismatch with always on success",
	)
	assert.Equal(s.T(), "present", traefikResp.ResponseHeaders.Get("X-Custom"))
}

func (s *SnippetSuite) TestAddHeaderNoAlwaysOnSuccess() {
	// add_header without "always" should apply on 200 responses.
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/noalways-200", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Custom"),
		traefikResp.ResponseHeaders.Get("X-Custom"),
		"X-Custom mismatch without always on success",
	)
	assert.Equal(s.T(), "present", traefikResp.ResponseHeaders.Get("X-Custom"))
}

// --- expires tests ---

func (s *SnippetSuite) TestExpiresEpoch() {
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/expires-epoch", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Expires"),
		traefikResp.ResponseHeaders.Get("Expires"),
		"Expires header mismatch for epoch",
	)
	assert.Equal(s.T(), "Thu, 01 Jan 1970 00:00:01 GMT", traefikResp.ResponseHeaders.Get("Expires"))

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Cache-Control"),
		traefikResp.ResponseHeaders.Get("Cache-Control"),
		"Cache-Control header mismatch for epoch",
	)
	assert.Equal(s.T(), "no-cache", traefikResp.ResponseHeaders.Get("Cache-Control"))
}

func (s *SnippetSuite) TestExpiresMax() {
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/expires-max", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Expires"),
		traefikResp.ResponseHeaders.Get("Expires"),
		"Expires header mismatch for max",
	)
	assert.Equal(s.T(), "Thu, 31 Dec 2037 23:55:55 GMT", traefikResp.ResponseHeaders.Get("Expires"))

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Cache-Control"),
		traefikResp.ResponseHeaders.Get("Cache-Control"),
		"Cache-Control header mismatch for max",
	)
	assert.Equal(s.T(), "max-age=315360000", traefikResp.ResponseHeaders.Get("Cache-Control"))
}

func (s *SnippetSuite) TestExpiresOff() {
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/expires-off", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	// expires off should not add Expires or Cache-Control headers.
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Expires"),
		traefikResp.ResponseHeaders.Get("Expires"),
		"Expires header mismatch for off",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("Expires"),
		"expires off should not set Expires header")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Cache-Control"),
		traefikResp.ResponseHeaders.Get("Cache-Control"),
		"Cache-Control header mismatch for off",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("Cache-Control"),
		"expires off should not set Cache-Control header")
}

// --- deny all test ---

func (s *SnippetSuite) TestDenyAll() {
	traefikResp, nginxResp := s.requestDenyAll()

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "deny all should return 403")
}

// --- more_set_headers -a append flag ---

func (s *SnippetSuite) TestMoreSetHeadersAppend() {
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// The first value of X-Append should be "first".
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Append"),
		traefikResp.ResponseHeaders.Get("X-Append"),
		"X-Append first value mismatch",
	)
	assert.Equal(s.T(), "first", traefikResp.ResponseHeaders.Get("X-Append"))

	// The -a flag appends a second value; verify both controllers produce the same set.
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Values("X-Append"),
		traefikResp.ResponseHeaders.Values("X-Append"),
		"X-Append values mismatch",
	)
}

// --- more_set_input_headers -r restrict flag ---

func (s *SnippetSuite) TestMoreSetInputHeadersRestrictExisting() {
	// With -r, the header is only overwritten if it already exists in the request.
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/", map[string]string{
		"X-Restrict": "old-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Restrict"],
		traefikResp.RequestHeaders["X-Restrict"],
		"X-Restrict mismatch when existing",
	)
	assert.Equal(s.T(), "restricted-value", traefikResp.RequestHeaders["X-Restrict"],
		"with -r, existing header should be overwritten")
}

func (s *SnippetSuite) TestMoreSetInputHeadersRestrictMissing() {
	// With -r, the header is NOT set if it doesn't already exist.
	traefikResp, nginxResp := s.requestNewDir(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Restrict"],
		traefikResp.RequestHeaders["X-Restrict"],
		"X-Restrict mismatch when missing",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Restrict"],
		"with -r, missing header should not be set")
}

// =============================================================================
// Tests from snippet_ifdirective_suite (if conditions)
// =============================================================================

func (s *SnippetSuite) TestIfHeaderEqualMatch() {
	traefikResp, nginxResp := s.requestIfHeader(http.MethodGet, "/", map[string]string{
		"X-Check-Equal": "expected",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Equal-Matched"),
		traefikResp.ResponseHeaders.Get("X-Equal-Matched"),
		"X-Equal-Matched mismatch",
	)
	assert.Equal(s.T(), "yes", traefikResp.ResponseHeaders.Get("X-Equal-Matched"))
}

func (s *SnippetSuite) TestIfHeaderEqualNoMatch() {
	traefikResp, nginxResp := s.requestIfHeader(http.MethodGet, "/", map[string]string{
		"X-Check-Equal": "wrong",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Equal-Matched"),
		traefikResp.ResponseHeaders.Get("X-Equal-Matched"),
		"X-Equal-Matched mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Equal-Matched"))
}

func (s *SnippetSuite) TestIfRegexMatch() {
	traefikResp, nginxResp := s.requestIfHeader(http.MethodGet, "/api-check/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Regex-Matched"),
		traefikResp.ResponseHeaders.Get("X-Regex-Matched"),
		"X-Regex-Matched mismatch",
	)
	assert.Equal(s.T(), "true", traefikResp.ResponseHeaders.Get("X-Regex-Matched"))
}

func (s *SnippetSuite) TestIfRegexNoMatch() {
	traefikResp, nginxResp := s.requestIfHeader(http.MethodGet, "/other", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Regex-Matched"),
		traefikResp.ResponseHeaders.Get("X-Regex-Matched"),
		"X-Regex-Matched mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Regex-Matched"))
}

func (s *SnippetSuite) TestIfCaseInsensitiveRegexMatch() {
	traefikResp, nginxResp := s.requestIfHeader(http.MethodGet, "/", map[string]string{
		"X-Check-Ci": "TEST-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-CI-Matched"),
		traefikResp.ResponseHeaders.Get("X-CI-Matched"),
		"X-CI-Matched mismatch",
	)
	assert.Equal(s.T(), "yes", traefikResp.ResponseHeaders.Get("X-CI-Matched"))
}

func (s *SnippetSuite) TestIfCaseInsensitiveRegexNoMatch() {
	traefikResp, nginxResp := s.requestIfHeader(http.MethodGet, "/", map[string]string{
		"X-Check-Ci": "other-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-CI-Matched"),
		traefikResp.ResponseHeaders.Get("X-CI-Matched"),
		"X-CI-Matched mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-CI-Matched"))
}

func (s *SnippetSuite) TestIfNegativeRegexMatch() {
	// !~* "^admin" should match when header does NOT start with admin.
	traefikResp, nginxResp := s.requestIfNeg(map[string]string{
		"X-Check-Neg": "user-request",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Neg-Matched"),
		traefikResp.ResponseHeaders.Get("X-Neg-Matched"),
		"X-Neg-Matched mismatch",
	)
	assert.Equal(s.T(), "true", traefikResp.ResponseHeaders.Get("X-Neg-Matched"))
}

func (s *SnippetSuite) TestIfNegativeRegexNoMatch() {
	// !~* "^admin" should NOT match when header starts with ADMIN (case-insensitive).
	traefikResp, nginxResp := s.requestIfNeg(map[string]string{
		"X-Check-Neg": "ADMIN-request",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Neg-Matched"),
		traefikResp.ResponseHeaders.Get("X-Neg-Matched"),
		"X-Neg-Matched mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Neg-Matched"))
}

func (s *SnippetSuite) TestIfVariableCheck() {
	// $myflag is set to "enabled" (truthy), so X-Flag-Matched should always be present.
	traefikResp, nginxResp := s.requestIfVar("/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Flag-Matched"),
		traefikResp.ResponseHeaders.Get("X-Flag-Matched"),
		"X-Flag-Matched mismatch",
	)
	assert.Equal(s.T(), "yes", traefikResp.ResponseHeaders.Get("X-Flag-Matched"))
}

func (s *SnippetSuite) TestIfRegexCaptureGroup() {
	// Regex capture groups from if conditions should be available as $1.
	traefikResp, nginxResp := s.requestIfVar("/capture/something")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Captured"),
		traefikResp.ResponseHeaders.Get("X-Captured"),
		"X-Captured mismatch",
	)
	assert.Equal(s.T(), "something", traefikResp.ResponseHeaders.Get("X-Captured"))
}

func (s *SnippetSuite) TestIfRegexCaptureGroupNoMatch() {
	// When the regex doesn't match, X-Captured should not be set.
	traefikResp, nginxResp := s.requestIfVar("/other/path")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Captured"),
		traefikResp.ResponseHeaders.Get("X-Captured"),
		"X-Captured mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Captured"))
}

// =============================================================================
// Tests from snippet_advanced_suite (location ~* case-insensitive regex)
// =============================================================================

func (s *SnippetSuite) TestLocationCaseInsensitiveRegexMatch() {
	// location ~* \.css$ should match /style/main.CSS (uppercase extension).
	traefikResp, nginxResp := s.requestLocationCI("/style/main.CSS")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Type"),
		traefikResp.ResponseHeaders.Get("X-Type"),
		"X-Type mismatch",
	)
	assert.Equal(s.T(), "css", traefikResp.ResponseHeaders.Get("X-Type"))
}

func (s *SnippetSuite) TestLocationCaseInsensitiveRegexMatchLowercase() {
	// location ~* \.css$ should also match /style/main.css (lowercase).
	traefikResp, nginxResp := s.requestLocationCI("/style/main.css")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Type"),
		traefikResp.ResponseHeaders.Get("X-Type"),
		"X-Type mismatch",
	)
	assert.Equal(s.T(), "css", traefikResp.ResponseHeaders.Get("X-Type"))
}

func (s *SnippetSuite) TestLocationCaseInsensitiveRegexNoMatch() {
	// /style/main.js should NOT match ~* \.css$ and should fall through
	// to the default ingress location (backend proxy).
	traefikResp, nginxResp := s.requestLocationCI("/style/main.js")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Both should fall through to the backend.
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "should fall through to backend")
}

// =============================================================================
// Tests from snippet_variables_suite (nginx variables)
// =============================================================================

// --- $uri / $document_uri ---

func (s *SnippetSuite) TestVarUri() {
	// $uri should return the path without the query string.
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test?q=hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Uri"],
		traefikResp.RequestHeaders["X-Var-Uri"],
		"$uri mismatch",
	)
	assert.Equal(s.T(), "/vars/test", traefikResp.RequestHeaders["X-Var-Uri"])
}

func (s *SnippetSuite) TestVarDocumentUri() {
	// $document_uri is an alias for $uri.
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test?q=hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Doc-Uri"],
		traefikResp.RequestHeaders["X-Var-Doc-Uri"],
		"$document_uri mismatch",
	)
	assert.Equal(s.T(), "/vars/test", traefikResp.RequestHeaders["X-Var-Doc-Uri"])
}

// --- $host / $server_name ---

func (s *SnippetSuite) TestVarHost() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $host should be the Host header value (without port).
	assert.Equal(s.T(), varsTraefikHost, traefikResp.RequestHeaders["X-Var-Host"],
		"traefik $host should match ingress host")
	assert.Equal(s.T(), varsNginxHost, nginxResp.RequestHeaders["X-Var-Host"],
		"nginx $host should match ingress host")
}

func (s *SnippetSuite) TestVarServerName() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $server_name should be the hostname without port.
	assert.Equal(s.T(), varsTraefikHost, traefikResp.RequestHeaders["X-Var-Server-Name"],
		"traefik $server_name should match ingress host")
	assert.Equal(s.T(), varsNginxHost, nginxResp.RequestHeaders["X-Var-Server-Name"],
		"nginx $server_name should match ingress host")
}

// --- $args / $query_string / $is_args / $arg_* ---

func (s *SnippetSuite) TestVarArgsPresent() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test?token=abc123&other=val", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Args"],
		traefikResp.RequestHeaders["X-Var-Args"],
		"$args mismatch",
	)
	assert.Contains(s.T(), traefikResp.RequestHeaders["X-Var-Args"], "token=abc123")
	assert.Contains(s.T(), traefikResp.RequestHeaders["X-Var-Args"], "other=val")
}

func (s *SnippetSuite) TestVarArgsAbsent() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Args"],
		traefikResp.RequestHeaders["X-Var-Args"],
		"$args mismatch when empty",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Args"])
}

func (s *SnippetSuite) TestVarQueryString() {
	// $query_string is an alias for $args.
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test?token=abc123&other=val", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Query-String"],
		traefikResp.RequestHeaders["X-Var-Query-String"],
		"$query_string mismatch",
	)
	assert.Contains(s.T(), traefikResp.RequestHeaders["X-Var-Query-String"], "token=abc123")
}

func (s *SnippetSuite) TestVarIsArgsPresent() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test?q=hello", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Is-Args"],
		traefikResp.RequestHeaders["X-Var-Is-Args"],
		"$is_args mismatch",
	)
	assert.Equal(s.T(), "?", traefikResp.RequestHeaders["X-Var-Is-Args"])
}

func (s *SnippetSuite) TestVarIsArgsAbsent() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Is-Args"],
		traefikResp.RequestHeaders["X-Var-Is-Args"],
		"$is_args mismatch when empty",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Is-Args"])
}

func (s *SnippetSuite) TestVarArgSpecific() {
	// $arg_token extracts the "token" query parameter.
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/vars/test?token=abc123&other=val", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Arg-Token"],
		traefikResp.RequestHeaders["X-Var-Arg-Token"],
		"$arg_token mismatch",
	)
	assert.Equal(s.T(), "abc123", traefikResp.RequestHeaders["X-Var-Arg-Token"])
}

// --- $content_type ---

func (s *SnippetSuite) TestVarContentType() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", map[string]string{
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

func (s *SnippetSuite) TestVarCookie() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", map[string]string{
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

func (s *SnippetSuite) TestVarCookieAbsent() {
	// Without the cookie, $cookie_testcookie should be empty.
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Cookie"],
		traefikResp.RequestHeaders["X-Var-Cookie"],
		"$cookie_testcookie mismatch when absent",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Cookie"])
}

// --- $http_* ---

func (s *SnippetSuite) TestVarHttpHeader() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", map[string]string{
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

func (s *SnippetSuite) TestVarHttpHeaderAbsent() {
	// Without the header, $http_x_custom_var should be empty.
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Var-Http-Custom"],
		traefikResp.RequestHeaders["X-Var-Http-Custom"],
		"$http_x_custom_var mismatch when absent",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Var-Http-Custom"])
}

// --- $server_port ---

func (s *SnippetSuite) TestVarServerPort() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

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

func (s *SnippetSuite) TestVarBestHttpHost() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// $best_http_host preserves the port in the Host header.
	// Each controller uses its own hostname, so compare individually.
	assert.Equal(s.T(), varsTraefikHost, traefikResp.RequestHeaders["X-Var-Best-Http-Host"],
		"traefik $best_http_host should match ingress host")
	assert.Equal(s.T(), varsNginxHost, nginxResp.RequestHeaders["X-Var-Best-Http-Host"],
		"nginx $best_http_host should match ingress host")
}

// --- $proxy_add_x_forwarded_for ---

func (s *SnippetSuite) TestVarProxyAddXForwardedFor() {
	traefikResp, nginxResp := s.requestVars(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Without an existing X-Forwarded-For header, $proxy_add_x_forwarded_for
	// should be the client's remote address.
	assert.NotEmpty(s.T(), traefikResp.RequestHeaders["X-Var-Proxy-Add-Xff"],
		"$proxy_add_x_forwarded_for should not be empty")
	assert.NotEmpty(s.T(), nginxResp.RequestHeaders["X-Var-Proxy-Add-Xff"],
		"$proxy_add_x_forwarded_for should not be empty")
}

// =============================================================================
// Tests from snippet_moreheaders_suite (more_set_headers, more_clear_headers, etc.)
// =============================================================================

// --- more_set_headers tests ---

func (s *SnippetSuite) TestMoreSetHeadersMultiple() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Multi-A", "X-Multi-B"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
	}
	assert.Equal(s.T(), "a-val", traefikResp.ResponseHeaders.Get("X-Multi-A"))
	assert.Equal(s.T(), "b-val", traefikResp.ResponseHeaders.Get("X-Multi-B"))
}

func (s *SnippetSuite) TestMoreSetHeadersClearingWithColon() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Colon-Clear"),
		traefikResp.ResponseHeaders.Get("X-Colon-Clear"),
		"X-Colon-Clear mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Colon-Clear"),
		"X-Colon-Clear should be cleared by config-snippet")
}

func (s *SnippetSuite) TestMoreSetHeadersClearingWithoutColon() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-NoColon-Clear"),
		traefikResp.ResponseHeaders.Get("X-NoColon-Clear"),
		"X-NoColon-Clear mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-NoColon-Clear"),
		"X-NoColon-Clear should be cleared by config-snippet")
}

// --- more_set_input_headers tests ---

func (s *SnippetSuite) TestMoreSetInputHeadersMultiple() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Input-Multi-A", "X-Input-Multi-B"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
	}
	assert.Equal(s.T(), "ia-val", traefikResp.RequestHeaders["X-Input-Multi-A"])
	assert.Equal(s.T(), "ib-val", traefikResp.RequestHeaders["X-Input-Multi-B"])
}

func (s *SnippetSuite) TestMoreSetInputHeadersClearing() {
	// Send X-Clear-Input with a value; the config-snippet should clear it.
	traefikResp, nginxResp := s.requestMoreHeaders(map[string]string{
		"X-Clear-Input": "original-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Clear-Input"],
		traefikResp.RequestHeaders["X-Clear-Input"],
		"X-Clear-Input mismatch",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Clear-Input"],
		"X-Clear-Input should be cleared")
}

// --- more_clear_headers tests ---

func (s *SnippetSuite) TestMoreClearHeadersSingle() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-To-Clear"),
		traefikResp.ResponseHeaders.Get("X-To-Clear"),
		"X-To-Clear mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-To-Clear"),
		"X-To-Clear should be cleared")
}

func (s *SnippetSuite) TestMoreClearHeadersMultiple() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Clear-One", "X-Clear-Two"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
		assert.Empty(s.T(), traefikResp.ResponseHeaders.Get(header),
			"%s should be cleared", header)
	}

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Keep-This"),
		traefikResp.ResponseHeaders.Get("X-Keep-This"),
		"X-Keep-This mismatch",
	)
	assert.Equal(s.T(), "keep-val", traefikResp.ResponseHeaders.Get("X-Keep-This"))
}

func (s *SnippetSuite) TestMoreClearHeadersWildcard() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Wild-One", "X-Wild-Two"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
		assert.Empty(s.T(), traefikResp.ResponseHeaders.Get(header),
			"%s should be cleared by wildcard", header)
	}

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Other-Resp"),
		traefikResp.ResponseHeaders.Get("X-Other-Resp"),
		"X-Other-Resp mismatch",
	)
	assert.Equal(s.T(), "other", traefikResp.ResponseHeaders.Get("X-Other-Resp"))
}

func (s *SnippetSuite) TestMoreClearHeadersCrossSnippet() {
	traefikResp, nginxResp := s.requestMoreHeaders(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Server-Cross"),
		traefikResp.ResponseHeaders.Get("X-Server-Cross"),
		"X-Server-Cross mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Server-Cross"),
		"X-Server-Cross should be cleared by config-snippet")
}

// --- more_clear_input_headers tests ---

func (s *SnippetSuite) TestMoreClearInputHeadersSingle() {
	traefikResp, nginxResp := s.requestMoreHeaders(map[string]string{
		"X-Secret": "secret-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Secret"],
		traefikResp.RequestHeaders["X-Secret"],
		"X-Secret mismatch",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Secret"],
		"X-Secret should be cleared from request")
}

func (s *SnippetSuite) TestMoreClearInputHeadersWildcard() {
	traefikResp, nginxResp := s.requestMoreHeaders(map[string]string{
		"X-Prefix-One": "val1",
		"X-Prefix-Two": "val2",
		"X-Other":      "other",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Prefix-One", "X-Prefix-Two"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
		assert.Empty(s.T(), traefikResp.RequestHeaders[header],
			"%s should be cleared by wildcard", header)
	}

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Other"],
		traefikResp.RequestHeaders["X-Other"],
		"X-Other mismatch",
	)
	assert.Equal(s.T(), "other", traefikResp.RequestHeaders["X-Other"])
}

// =============================================================================
// Tests from snippet_rewrite_suite (rewrite directive)
// =============================================================================

// --- Server-snippet rewrite tests ---

func (s *SnippetSuite) TestRewriteLastFlag() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-last/page", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest/page HTTP/1.1", "nginx backend should see rewritten URI")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest/page HTTP/1.1", "traefik backend should see rewritten URI")
}

func (s *SnippetSuite) TestRewritePermanentRedirect() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-permanent", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusMovedPermanently, traefikResp.StatusCode, "expected 301 for permanent redirect")

	assert.Contains(s.T(),
		traefikResp.ResponseHeaders.Get("Location"),
		"/rw-perm-dest",
		"traefik Location header should contain redirect target",
	)
	assert.Contains(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		"/rw-perm-dest",
		"nginx Location header should contain redirect target",
	)
}

func (s *SnippetSuite) TestRewriteRedirectFlag() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-redirect", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302 for redirect")

	assert.Contains(s.T(),
		traefikResp.ResponseHeaders.Get("Location"),
		"/rw-redir-dest",
		"traefik Location header should contain redirect target",
	)
	assert.Contains(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		"/rw-redir-dest",
		"nginx Location header should contain redirect target",
	)
}

func (s *SnippetSuite) TestRewriteMultipleCaptureGroups() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-multicap/music/media/song.flac", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest/music/mp3/song.mp3 HTTP/1.1",
		"nginx backend should see rewritten URI with multiple captures")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest/music/mp3/song.mp3 HTTP/1.1",
		"traefik backend should see rewritten URI with multiple captures")
}

func (s *SnippetSuite) TestRewriteNoFlagChain() {
	// Without a flag, rewrite continues processing the next rule.
	// /rw-chain -> /rw-step (no flag, continue) -> /rw-chain-done (last, stop).
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-chain", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-chain-done HTTP/1.1",
		"nginx backend should see chained rewrite result")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-chain-done HTTP/1.1",
		"traefik backend should see chained rewrite result")
}

func (s *SnippetSuite) TestRewriteURLRedirect() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-url-redir", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302 for URL redirect")

	assert.Contains(s.T(),
		traefikResp.ResponseHeaders.Get("Location"),
		"http://other.example.com/new",
		"traefik Location header should contain full URL",
	)
	assert.Contains(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		"http://other.example.com/new",
		"nginx Location header should contain full URL",
	)
}

func (s *SnippetSuite) TestRewriteNoMatch() {
	// A path that doesn't match any rewrite rule should pass through unchanged.
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/no-rewrite-match", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /no-rewrite-match HTTP/1.1",
		"nginx backend should see original URI")
	assert.Contains(s.T(), traefikResp.Body, "GET /no-rewrite-match HTTP/1.1",
		"traefik backend should see original URI")
}

// --- Configuration-snippet rewrite tests ---

func (s *SnippetSuite) TestRewriteBreakFlag() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-break/resource", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest/resource HTTP/1.1",
		"nginx backend should see rewritten URI (break)")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest/resource HTTP/1.1",
		"traefik backend should see rewritten URI (break)")
}

func (s *SnippetSuite) TestRewriteInConfigSnippet() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-cfg/resource", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-cfg-dest/resource HTTP/1.1",
		"nginx backend should see rewritten URI (config-snippet)")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-cfg-dest/resource HTTP/1.1",
		"traefik backend should see rewritten URI (config-snippet)")
}

func (s *SnippetSuite) TestRewritePreservesQuery() {
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-query?q=test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest?q=test HTTP/1.1",
		"nginx backend should see rewritten URI with preserved query")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest?q=test HTTP/1.1",
		"traefik backend should see rewritten URI with preserved query")
}

func (s *SnippetSuite) TestRewriteSuppressesQuery() {
	// The trailing ? in the replacement suppresses the original query string.
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-noquery?q=test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest HTTP/1.1",
		"nginx backend should see rewritten URI without query")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest HTTP/1.1",
		"traefik backend should see rewritten URI without query")
}

func (s *SnippetSuite) TestRewriteInIfBlock() {
	// The rewrite inside `if ($request_method = POST)` should only fire for POST.
	traefikResp, nginxResp := s.requestRewrite(http.MethodPost, "/rw-if/page", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "POST /rw-dest/page HTTP/1.1",
		"nginx backend should see rewritten URI (if block, POST)")
	assert.Contains(s.T(), traefikResp.Body, "POST /rw-dest/page HTTP/1.1",
		"traefik backend should see rewritten URI (if block, POST)")
}

func (s *SnippetSuite) TestRewriteInIfBlockNoMatch() {
	// GET to /rw-if/page should NOT trigger the rewrite (if condition doesn't match).
	traefikResp, nginxResp := s.requestRewrite(http.MethodGet, "/rw-if/page", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-if/page HTTP/1.1",
		"nginx backend should see original URI (if condition not met)")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-if/page HTTP/1.1",
		"traefik backend should see original URI (if condition not met)")
}
