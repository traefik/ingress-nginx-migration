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
	newDirIngressName = "snippet-newdir-test"
	newDirTraefikHost = newDirIngressName + ".traefik.local"
	newDirNginxHost   = newDirIngressName + ".nginx.local"

	denyAllIngressName = "snippet-deny-all-test"
	denyAllTraefikHost = denyAllIngressName + ".traefik.local"
	denyAllNginxHost   = denyAllIngressName + ".nginx.local"
)

type SnippetNewDirectivesSuite struct {
	BaseSuite
}

func TestSnippetNewDirectivesSuite(t *testing.T) {
	suite.Run(t, new(SnippetNewDirectivesSuite))
}

func (s *SnippetNewDirectivesSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress 1: combined tests for add_header always, expires, more_set_headers flags.
	annotations := map[string]string{
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

	err := s.traefik.DeployIngress(newDirIngressName, newDirTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy newdir ingress to traefik cluster")

	err = s.nginx.DeployIngress(newDirIngressName, newDirNginxHost, annotations)
	require.NoError(s.T(), err, "deploy newdir ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), newDirTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), newDirNginxHost, 20, 1*time.Second)

	// Ingress 2: deny all (blocks every request with 403).
	denyAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/configuration-snippet": `
deny all;
`,
	}

	err = s.traefik.DeployIngress(denyAllIngressName, denyAllTraefikHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy deny-all ingress to traefik cluster")

	err = s.nginx.DeployIngress(denyAllIngressName, denyAllNginxHost, denyAnnotations)
	require.NoError(s.T(), err, "deploy deny-all ingress to nginx cluster")

	// WaitForIngressReady expects non-404/non-502; a 403 satisfies this.
	s.traefik.WaitForIngressReady(s.T(), denyAllTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), denyAllNginxHost, 20, 1*time.Second)
}

func (s *SnippetNewDirectivesSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(newDirIngressName)
	_ = s.nginx.DeleteIngress(newDirIngressName)
	_ = s.traefik.DeleteIngress(denyAllIngressName)
	_ = s.nginx.DeleteIngress(denyAllIngressName)
}

func (s *SnippetNewDirectivesSuite) requestNewDir(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), newDirTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), newDirNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetNewDirectivesSuite) requestDenyAll() (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), denyAllTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), denyAllNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- add_header always tests ---

func (s *SnippetNewDirectivesSuite) TestAddHeaderAlwaysOnError() {
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

func (s *SnippetNewDirectivesSuite) TestAddHeaderNoAlwaysOnError() {
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

func (s *SnippetNewDirectivesSuite) TestAddHeaderAlwaysOnSuccess() {
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

func (s *SnippetNewDirectivesSuite) TestAddHeaderNoAlwaysOnSuccess() {
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

func (s *SnippetNewDirectivesSuite) TestExpiresEpoch() {
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

func (s *SnippetNewDirectivesSuite) TestExpiresMax() {
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

func (s *SnippetNewDirectivesSuite) TestExpiresOff() {
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

func (s *SnippetNewDirectivesSuite) TestDenyAll() {
	traefikResp, nginxResp := s.requestDenyAll()

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "deny all should return 403")
}

// --- more_set_headers -a append flag ---

func (s *SnippetNewDirectivesSuite) TestMoreSetHeadersAppend() {
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

func (s *SnippetNewDirectivesSuite) TestMoreSetInputHeadersRestrictExisting() {
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

func (s *SnippetNewDirectivesSuite) TestMoreSetInputHeadersRestrictMissing() {
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
