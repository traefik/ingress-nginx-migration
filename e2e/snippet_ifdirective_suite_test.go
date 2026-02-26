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
	ifDirectiveIngressName = "snippet-if-directive-test"
	ifDirectiveTraefikHost = ifDirectiveIngressName + ".traefik.local"
	ifDirectiveNginxHost   = ifDirectiveIngressName + ".nginx.local"
)

type SnippetIfDirectiveSuite struct {
	BaseSuite
}

func TestSnippetIfDirectiveSuite(t *testing.T) {
	suite.Run(t, new(SnippetIfDirectiveSuite))
}

func (s *SnippetIfDirectiveSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Use more_set_headers inside if blocks (additive) to avoid nginx's
	// add_header deepest-block-wins interaction between multiple if blocks.
	annotations := map[string]string{
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
if ($http_x_check_neg !~* "^admin") {
    more_set_headers "X-Neg-Matched: true";
}
set $myflag "enabled";
if ($myflag) {
    more_set_headers "X-Flag-Matched: yes";
}
`,
	}

	err := s.traefik.DeployIngress(ifDirectiveIngressName, ifDirectiveTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy if-directive ingress to traefik cluster")

	err = s.nginx.DeployIngress(ifDirectiveIngressName, ifDirectiveNginxHost, annotations)
	require.NoError(s.T(), err, "deploy if-directive ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), ifDirectiveTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), ifDirectiveNginxHost, 20, 1*time.Second)
}

func (s *SnippetIfDirectiveSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(ifDirectiveIngressName)
	_ = s.nginx.DeleteIngress(ifDirectiveIngressName)
}

func (s *SnippetIfDirectiveSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), ifDirectiveTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), ifDirectiveNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *SnippetIfDirectiveSuite) TestIfHeaderEqualMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
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
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
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
	traefikResp, nginxResp := s.request(http.MethodGet, "/api-check/test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Regex-Matched"),
		traefikResp.ResponseHeaders.Get("X-Regex-Matched"),
		"X-Regex-Matched mismatch",
	)
	assert.Equal(s.T(), "true", traefikResp.ResponseHeaders.Get("X-Regex-Matched"))
}

func (s *SnippetIfDirectiveSuite) TestIfRegexNoMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/other", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Regex-Matched"),
		traefikResp.ResponseHeaders.Get("X-Regex-Matched"),
		"X-Regex-Matched mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Regex-Matched"))
}

func (s *SnippetIfDirectiveSuite) TestIfCaseInsensitiveRegexMatch() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
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
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
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
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
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
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
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
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Flag-Matched"),
		traefikResp.ResponseHeaders.Get("X-Flag-Matched"),
		"X-Flag-Matched mismatch",
	)
	assert.Equal(s.T(), "yes", traefikResp.ResponseHeaders.Get("X-Flag-Matched"))
}
