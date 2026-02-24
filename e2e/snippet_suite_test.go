package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SnippetSuite struct {
	BaseSuite
}

func TestSnippetSuite(t *testing.T) {
	suite.Run(t, new(SnippetSuite))
}

func snippetAnnotations(serverSnippet, configSnippet string) map[string]string {
	annotations := make(map[string]string)
	if serverSnippet != "" {
		annotations["nginx.ingress.kubernetes.io/server-snippet"] = serverSnippet
	}
	if configSnippet != "" {
		annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = configSnippet
	}
	return annotations
}

func (s *SnippetSuite) TestResponseHeaders() {
	testCases := []struct {
		desc                 string
		serverSnippet        string
		configurationSnippet string
		compareHeaders       []string
	}{
		{
			desc:           "add_header server snippet adds simple header",
			serverSnippet:  `add_header X-Custom "custom-value";`,
			compareHeaders: []string{"X-Custom"},
		},
		{
			desc:           "add_header server snippet adds header without quotes",
			serverSnippet:  `add_header X-Simple simple;`,
			compareHeaders: []string{"X-Simple"},
		},
		{
			desc:                 "add_header configuration snippet adds simple header",
			configurationSnippet: `add_header X-Custom "custom-value";`,
			compareHeaders:       []string{"X-Custom"},
		},
		{
			desc:                 "add_header configuration snippet adds header without quotes",
			configurationSnippet: `add_header X-Simple simple;`,
			compareHeaders:       []string{"X-Simple"},
		},
		{
			desc:                 "add_header configuration snippet overrides server snippet",
			serverSnippet:        `add_header X-Server server-value;`,
			configurationSnippet: `add_header X-Config config-value;`,
			compareHeaders:       []string{"X-Server", "X-Config"},
		},
		{
			desc:           "more_set_headers server snippet sets header",
			serverSnippet:  `more_set_headers "X-Custom:custom-value";`,
			compareHeaders: []string{"X-Custom"},
		},
		{
			desc:                 "more_set_headers configuration snippet sets header",
			configurationSnippet: `more_set_headers "X-Custom:custom-value";`,
			compareHeaders:       []string{"X-Custom"},
		},
		{
			desc:                 "more_set_headers both snippets set headers",
			serverSnippet:        `more_set_headers "X-Server:server-value";`,
			configurationSnippet: `more_set_headers "X-Config:config-value";`,
			compareHeaders:       []string{"X-Server", "X-Config"},
		},
		{
			desc:                 "more_set_headers both snippets override same header",
			serverSnippet:        `more_set_headers "X-Header:server-value";`,
			configurationSnippet: `more_set_headers "X-Header:config-value";`,
			compareHeaders:       []string{"X-Header"},
		},
		{
			desc:                 "more_set_headers configuration snippet with space",
			configurationSnippet: `more_set_headers "X-Header: config-value";`,
			compareHeaders:       []string{"X-Header"},
		},
	}

	for _, test := range testCases {
		s.Run(test.desc, func() {
			ingressName := "snippet-test-" + sanitizeName(test.desc)

			traefikResp, nginxResp := s.makeComparisonRequests(ingressName, snippetAnnotations(test.serverSnippet, test.configurationSnippet), http.MethodGet, "/", nil)

			assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

			for _, header := range test.compareHeaders {
				assert.Equal(s.T(),
					nginxResp.ResponseHeaders.Get(header),
					traefikResp.ResponseHeaders.Get(header),
					"response header %s mismatch between controllers", header,
				)
			}
		})
	}
}

func (s *SnippetSuite) TestAllDirectives() {
	testCases := []struct {
		desc                   string
		serverSnippet          string
		configurationSnippet   string
		method                 string
		path                   string
		requestHeaders         map[string]string
		compareResponseHeaders []string
		compareRequestHeaders  []string
		compareBody            bool
	}{
		{
			desc: "add_header with variable interpolation",
			configurationSnippet: `
add_header X-Method $request_method;
add_header X-Uri $request_uri;
`,
			compareResponseHeaders: []string{"X-Method", "X-Uri"},
		},
		{
			desc: "more_set_headers directive",
			configurationSnippet: `
more_set_headers "X-Custom-Header:custom-value";
more_set_headers "X-Another:another-value";
`,
			compareResponseHeaders: []string{"X-Custom-Header", "X-Another"},
		},
		{
			desc: "proxy_set_header directive",
			configurationSnippet: `
proxy_set_header X-Custom-Method $request_method;
proxy_set_header X-Custom-Uri $request_uri;
`,
			compareRequestHeaders: []string{"X-Custom-Method", "X-Custom-Uri"},
		},
		{
			desc: "set directive creates variable",
			configurationSnippet: `
set $my_var "hello";
add_header X-My-Var $my_var;
`,
			compareResponseHeaders: []string{"X-My-Var"},
		},
		{
			desc: "set directive with variable interpolation",
			configurationSnippet: `
set $combined "$request_method-$request_uri";
add_header X-Combined $combined;
`,
			compareResponseHeaders: []string{"X-Combined"},
		},
		{
			desc: "if directive with matching condition",
			configurationSnippet: `
if ($request_method = GET) {
	add_header X-Is-Get "true";
}
`,
			method:                 http.MethodGet,
			compareResponseHeaders: []string{"X-Is-Get"},
		},
		{
			desc: "if directive with non-matching condition",
			configurationSnippet: `
if ($request_method = POST) {
	add_header X-Is-Post "true";
}
add_header X-Always "present";
`,
			method:                 http.MethodGet,
			compareResponseHeaders: []string{"X-Is-Post", "X-Always"},
		},
		{
			desc: "if directive with header check",
			configurationSnippet: `
if ($http_x_custom = "expected") {
	add_header X-Matched "yes";
}
`,
			requestHeaders: map[string]string{
				"X-Custom": "expected",
			},
			compareResponseHeaders: []string{"X-Matched"},
		},
		{
			desc: "if directive with regex match",
			configurationSnippet: `
if ($request_uri ~ "^/api") {
	add_header X-Is-Api "true";
}
`,
			path:                   "/api/users",
			compareResponseHeaders: []string{"X-Is-Api"},
		},
		{
			desc: "if directive with case-insensitive regex match - matching",
			configurationSnippet: `
if ($http_x_custom ~* "^test") {
	add_header X-Matched "yes";
}
`,
			requestHeaders: map[string]string{
				"X-Custom": "TEST-value",
			},
			compareResponseHeaders: []string{"X-Matched"},
		},
		{
			desc: "if directive with case-insensitive regex match - not matching",
			configurationSnippet: `
if ($http_x_custom ~* "^test") {
	add_header X-Matched "yes";
}
add_header X-Always "present";
`,
			requestHeaders: map[string]string{
				"X-Custom": "other-value",
			},
			compareResponseHeaders: []string{"X-Matched", "X-Always"},
		},
		{
			desc: "if directive with negative case-insensitive regex match",
			configurationSnippet: `
if ($http_x_custom !~* "^admin") {
	add_header X-Not-Admin "true";
}
`,
			requestHeaders: map[string]string{
				"X-Custom": "user-request",
			},
			compareResponseHeaders: []string{"X-Not-Admin"},
		},
		{
			desc: "if directive with negative case-insensitive regex match - should not match",
			configurationSnippet: `
if ($http_x_custom !~* "^admin") {
	add_header X-Not-Admin "true";
}
add_header X-Processed "yes";
`,
			requestHeaders: map[string]string{
				"X-Custom": "ADMIN-request",
			},
			compareResponseHeaders: []string{"X-Not-Admin", "X-Processed"},
		},
		{
			desc: "if directive with set variable check",
			configurationSnippet: `
set $flag "enabled";
if ($flag) {
	add_header X-Flag-Set "yes";
}
`,
			compareResponseHeaders: []string{"X-Flag-Set"},
		},
		{
			desc: "all directives combined",
			configurationSnippet: `
set $backend_type "api";
proxy_set_header X-Backend-Type $backend_type;
if ($request_method = GET) {
	add_header X-Read-Only "true";
	more_set_headers "X-Cache-Control:public";
}
add_header X-Powered-By "traefik";
`,
			method:                 http.MethodGet,
			compareResponseHeaders: []string{"X-Read-Only", "X-Cache-Control"},
			compareRequestHeaders:  []string{"X-Backend-Type"},
		},
		{
			desc: "server and configuration snippets interaction",
			serverSnippet: `
add_header X-Server "server-value";
set $shared "from-server";
`,
			configurationSnippet: `
add_header X-Config "config-value";
`,
			compareResponseHeaders: []string{"X-Server", "X-Config"},
		},
		{
			desc: "return directive with status code and text",
			configurationSnippet: `
return 403 "Forbidden";
`,
			compareBody: true,
		},
		{
			desc: "return directive with 200 status",
			configurationSnippet: `
return 200 "OK";
`,
			compareBody: true,
		},
		{
			desc: "return directive inside if block - condition matches",
			configurationSnippet: `
if ($request_method = POST) {
	return 405 "Method Not Allowed";
}
add_header X-Allowed "true";
`,
			method:      http.MethodPost,
			compareBody: true,
		},
		{
			desc: "return directive inside if block - condition does not match",
			configurationSnippet: `
if ($request_method = POST) {
	return 405 "Method Not Allowed";
}
add_header X-Allowed "true";
`,
			method:                 http.MethodGet,
			compareResponseHeaders: []string{"X-Allowed"},
		},
		{
			desc: "return directive doesn't stop processing headers",
			configurationSnippet: `
return 204 "";
add_header X-Should-Appear "value";
`,
			compareBody:            true,
			compareResponseHeaders: []string{"X-Should-Appear"},
		},
		{
			desc: "location without return returns 503",
			serverSnippet: `
location /api {
	add_header X-Location "api";
}
`,
			path:                   "/api/users",
			compareResponseHeaders: []string{"X-Location"},
		},
		{
			desc: "location directive with prefix match - not matching continues to next",
			serverSnippet: `
location /api {
	return 200 "OK";
}
add_header X-Always "present";
`,
			path:                   "/web/users",
			compareResponseHeaders: []string{"X-Always"},
		},
		{
			desc: "location directive with exact match and return",
			serverSnippet: `
location = /exact {
	return 200 "exact";
}
`,
			path:        "/exact",
			compareBody: true,
		},
		{
			desc: "location directive with exact match - not matching continues to next",
			serverSnippet: `
location = /exact {
	return 200 "exact";
}
add_header X-Always "present";
`,
			path:                   "/exact/more",
			compareResponseHeaders: []string{"X-Always"},
		},
		{
			desc: "location directive with regex match and return",
			serverSnippet: `
location ~ ^/api/v[0-9]+/ {
	return 200 "versioned";
}
`,
			path:        "/api/v2/users",
			compareBody: true,
		},
		{
			desc: "location directive with regex match - not matching continues to next",
			serverSnippet: `
location ~ ^/api/v[0-9]+/ {
	return 200 "versioned";
}
add_header X-Always "present";
`,
			path:                   "/api/latest/users",
			compareResponseHeaders: []string{"X-Always"},
		},
		{
			desc: "location with return applies add_header from same block",
			serverSnippet: `
location /blocked {
	add_header X-Block-Header "block-value";
	return 403 "Blocked";
}
`,
			path:                   "/blocked/path",
			compareBody:            true,
			compareResponseHeaders: []string{"X-Block-Header"},
		},
		{
			desc: "location with return applies more_set_headers from same block",
			serverSnippet: `
location /blocked {
	more_set_headers "X-More-Header:more-value";
	return 403 "Blocked";
}
`,
			path:                   "/blocked/path",
			compareBody:            true,
			compareResponseHeaders: []string{"X-More-Header"},
		},
		{
			desc: "location with return applies both add_header and more_set_headers",
			serverSnippet: `
location /api {
	add_header X-Add "add-value";
	more_set_headers "X-More:more-value";
	return 200 "OK";
}
`,
			path:                   "/api/endpoint",
			compareBody:            true,
			compareResponseHeaders: []string{"X-Add", "X-More"},
		},
		{
			desc: "add_header only applied in deepest block - location overrides root",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	return 200 "OK";
}
`,
			path:                   "/api/endpoint",
			compareBody:            true,
			compareResponseHeaders: []string{"X-Level"},
		},
		{
			desc: "add_header only applied in deepest block - nested if inside location",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	if ($request_method = GET) {
		add_header X-Level "if-block";
		return 200 "OK";
	}
}
`,
			path:                   "/api/endpoint",
			method:                 http.MethodGet,
			compareBody:            true,
			compareResponseHeaders: []string{"X-Level"},
		},
		{
			desc: "add_header from location when if condition not matched",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	if ($request_method = POST) {
		add_header X-Level "if-block";
		return 200 "POST";
	}
	return 200 "OTHER";
}
`,
			path:                   "/api/endpoint",
			method:                 http.MethodGet,
			compareBody:            true,
			compareResponseHeaders: []string{"X-Level"},
		},
		{
			desc: "root add_header applied when location not matched",
			serverSnippet: `
add_header X-Level "root";
location /api {
	add_header X-Level "location";
	return 200 "API";
}
`,
			path:                   "/web/endpoint",
			compareResponseHeaders: []string{"X-Level"},
		},
		{
			desc: "more_set_input_headers sets request header",
			configurationSnippet: `
more_set_input_headers "X-Custom-Input:input-value";
`,
			compareRequestHeaders: []string{"X-Custom-Input"},
		},
		{
			desc: "more_set_input_headers with variable interpolation",
			configurationSnippet: `
more_set_input_headers "X-Method-Input:$request_method";
`,
			compareRequestHeaders: []string{"X-Method-Input"},
		},
	}

	for _, test := range testCases {
		s.Run(test.desc, func() {
			ingressName := "directive-test-" + sanitizeName(test.desc)

			s.T().Cleanup(func() {
				if s.T().Failed() {
					s.T().Logf("Last 10 lines of traefik controller logs:\n%s", s.traefik.GetIngressControllerLogs(10))
					s.T().Logf("Last 10 lines of nginx controller logs:\n%s", s.nginx.GetIngressControllerLogs(10))
				}
			})

			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			path := test.path
			if path == "" {
				path = "/test"
			}

			traefikResp, nginxResp := s.makeComparisonRequests(ingressName, snippetAnnotations(test.serverSnippet, test.configurationSnippet), method, path, test.requestHeaders)

			// Always compare status codes.
			assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

			if test.compareBody {
				assert.Equal(s.T(), nginxResp.Body, traefikResp.Body, "body mismatch")
			}

			for _, header := range test.compareResponseHeaders {
				assert.Equal(s.T(),
					nginxResp.ResponseHeaders.Get(header),
					traefikResp.ResponseHeaders.Get(header),
					"response header %s mismatch between controllers", header,
				)
			}

			for _, header := range test.compareRequestHeaders {
				assert.Equal(s.T(),
					nginxResp.RequestHeaders[header],
					traefikResp.RequestHeaders[header],
					"request header %s mismatch between controllers", header,
				)
			}
		})
	}
}
