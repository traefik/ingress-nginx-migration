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
	// Ingress 1: Header-based conditions (mutually exclusive).
	ifHeaderIngressName = "snippet-if-header-test"
	ifHeaderTraefikHost = ifHeaderIngressName + ".traefik.local"
	ifHeaderNginxHost   = ifHeaderIngressName + ".nginx.local"

	// Ingress 2: Negative regex (standalone).
	ifNegIngressName = "snippet-if-neg-test"
	ifNegTraefikHost = ifNegIngressName + ".traefik.local"
	ifNegNginxHost   = ifNegIngressName + ".nginx.local"

	// Ingress 3: Variable check + capture group.
	ifVarIngressName = "snippet-if-var-test"
	ifVarTraefikHost = ifVarIngressName + ".traefik.local"
	ifVarNginxHost   = ifVarIngressName + ".nginx.local"
)

type SnippetIfDirectiveSuite struct {
	BaseSuite
}

func TestSnippetIfDirectiveSuite(t *testing.T) {
	suite.Run(t, new(SnippetIfDirectiveSuite))
}

func (s *SnippetIfDirectiveSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress 1: Mutually exclusive header-based if conditions.
	// Each test sends only the header needed for its condition,
	// ensuring only one if block matches per request (avoids nginx's
	// "last matching if wins" behavior).
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

	err := s.traefik.DeployIngress(ifHeaderIngressName, ifHeaderTraefikHost, headerAnnotations)
	require.NoError(s.T(), err, "deploy if-header ingress to traefik cluster")

	err = s.nginx.DeployIngress(ifHeaderIngressName, ifHeaderNginxHost, headerAnnotations)
	require.NoError(s.T(), err, "deploy if-header ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), ifHeaderTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), ifHeaderNginxHost, 20, 1*time.Second)

	// Ingress 2: Negative regex (standalone to avoid interference).
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

	// Ingress 3: Variable check + regex capture group.
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
}

func (s *SnippetIfDirectiveSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(ifHeaderIngressName)
	_ = s.nginx.DeleteIngress(ifHeaderIngressName)
	_ = s.traefik.DeleteIngress(ifNegIngressName)
	_ = s.nginx.DeleteIngress(ifNegIngressName)
	_ = s.traefik.DeleteIngress(ifVarIngressName)
	_ = s.nginx.DeleteIngress(ifVarIngressName)
}

func (s *SnippetIfDirectiveSuite) requestHeader(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifHeaderTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifHeaderNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetIfDirectiveSuite) requestNeg(headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifNegTraefikHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifNegNginxHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetIfDirectiveSuite) requestVar(path string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifVarTraefikHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifVarNginxHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetIfDirectiveSuite) TestIfHeaderEqualMatch() {
	traefikResp, nginxResp := s.requestHeader(http.MethodGet, "/", map[string]string{
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

func (s *SnippetIfDirectiveSuite) TestIfHeaderEqualNoMatch() {
	traefikResp, nginxResp := s.requestHeader(http.MethodGet, "/", map[string]string{
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

func (s *SnippetIfDirectiveSuite) TestIfRegexMatch() {
	traefikResp, nginxResp := s.requestHeader(http.MethodGet, "/api-check/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Regex-Matched"),
		traefikResp.ResponseHeaders.Get("X-Regex-Matched"),
		"X-Regex-Matched mismatch",
	)
	assert.Equal(s.T(), "true", traefikResp.ResponseHeaders.Get("X-Regex-Matched"))
}

func (s *SnippetIfDirectiveSuite) TestIfRegexNoMatch() {
	traefikResp, nginxResp := s.requestHeader(http.MethodGet, "/other", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Regex-Matched"),
		traefikResp.ResponseHeaders.Get("X-Regex-Matched"),
		"X-Regex-Matched mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Regex-Matched"))
}

func (s *SnippetIfDirectiveSuite) TestIfCaseInsensitiveRegexMatch() {
	traefikResp, nginxResp := s.requestHeader(http.MethodGet, "/", map[string]string{
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

func (s *SnippetIfDirectiveSuite) TestIfCaseInsensitiveRegexNoMatch() {
	traefikResp, nginxResp := s.requestHeader(http.MethodGet, "/", map[string]string{
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

func (s *SnippetIfDirectiveSuite) TestIfNegativeRegexMatch() {
	// !~* "^admin" should match when header does NOT start with admin.
	traefikResp, nginxResp := s.requestNeg(map[string]string{
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

func (s *SnippetIfDirectiveSuite) TestIfNegativeRegexNoMatch() {
	// !~* "^admin" should NOT match when header starts with ADMIN (case-insensitive).
	traefikResp, nginxResp := s.requestNeg(map[string]string{
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

func (s *SnippetIfDirectiveSuite) TestIfVariableCheck() {
	// $myflag is set to "enabled" (truthy), so X-Flag-Matched should always be present.
	traefikResp, nginxResp := s.requestVar("/")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Flag-Matched"),
		traefikResp.ResponseHeaders.Get("X-Flag-Matched"),
		"X-Flag-Matched mismatch",
	)
	assert.Equal(s.T(), "yes", traefikResp.ResponseHeaders.Get("X-Flag-Matched"))
}

func (s *SnippetIfDirectiveSuite) TestIfRegexCaptureGroup() {
	// Regex capture groups from if conditions should be available as $1.
	traefikResp, nginxResp := s.requestVar("/capture/something")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Captured"),
		traefikResp.ResponseHeaders.Get("X-Captured"),
		"X-Captured mismatch",
	)
	assert.Equal(s.T(), "something", traefikResp.ResponseHeaders.Get("X-Captured"))
}

func (s *SnippetIfDirectiveSuite) TestIfRegexCaptureGroupNoMatch() {
	// When the regex doesn't match, X-Captured should not be set.
	traefikResp, nginxResp := s.requestVar("/other/path")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Captured"),
		traefikResp.ResponseHeaders.Get("X-Captured"),
		"X-Captured mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Captured"))
}
