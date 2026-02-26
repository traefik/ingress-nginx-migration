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
	rewriteSnippetIngressName = "snippet-rewrite-test"
	rewriteSnippetTraefikHost = rewriteSnippetIngressName + ".traefik.local"
	rewriteSnippetNginxHost   = rewriteSnippetIngressName + ".nginx.local"
)

type SnippetRewriteSuite struct {
	BaseSuite
}

func TestSnippetRewriteSuite(t *testing.T) {
	suite.Run(t, new(SnippetRewriteSuite))
}

func (s *SnippetRewriteSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	annotations := map[string]string{
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

	err := s.traefik.DeployIngress(rewriteSnippetIngressName, rewriteSnippetTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy rewrite-snippet ingress to traefik cluster")

	err = s.nginx.DeployIngress(rewriteSnippetIngressName, rewriteSnippetNginxHost, annotations)
	require.NoError(s.T(), err, "deploy rewrite-snippet ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), rewriteSnippetTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), rewriteSnippetNginxHost, 20, 1*time.Second)
}

func (s *SnippetRewriteSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(rewriteSnippetIngressName)
	_ = s.nginx.DeleteIngress(rewriteSnippetIngressName)
}

func (s *SnippetRewriteSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), rewriteSnippetTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), rewriteSnippetNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- Server-snippet rewrite tests ---

func (s *SnippetRewriteSuite) TestRewriteLastFlag() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-last/page", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest/page HTTP/1.1", "nginx backend should see rewritten URI")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest/page HTTP/1.1", "traefik backend should see rewritten URI")
}

func (s *SnippetRewriteSuite) TestRewritePermanentRedirect() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-permanent", nil)

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

func (s *SnippetRewriteSuite) TestRewriteRedirectFlag() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-redirect", nil)

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

func (s *SnippetRewriteSuite) TestRewriteMultipleCaptureGroups() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-multicap/music/media/song.flac", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest/music/mp3/song.mp3 HTTP/1.1",
		"nginx backend should see rewritten URI with multiple captures")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest/music/mp3/song.mp3 HTTP/1.1",
		"traefik backend should see rewritten URI with multiple captures")
}

func (s *SnippetRewriteSuite) TestRewriteNoFlagChain() {
	// Without a flag, rewrite continues processing the next rule.
	// /rw-chain → /rw-step (no flag, continue) → /rw-chain-done (last, stop).
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-chain", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-chain-done HTTP/1.1",
		"nginx backend should see chained rewrite result")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-chain-done HTTP/1.1",
		"traefik backend should see chained rewrite result")
}

func (s *SnippetRewriteSuite) TestRewriteURLRedirect() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-url-redir", nil)

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

func (s *SnippetRewriteSuite) TestRewriteNoMatch() {
	// A path that doesn't match any rewrite rule should pass through unchanged.
	traefikResp, nginxResp := s.request(http.MethodGet, "/no-rewrite-match", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /no-rewrite-match HTTP/1.1",
		"nginx backend should see original URI")
	assert.Contains(s.T(), traefikResp.Body, "GET /no-rewrite-match HTTP/1.1",
		"traefik backend should see original URI")
}

// --- Configuration-snippet rewrite tests ---

func (s *SnippetRewriteSuite) TestRewriteBreakFlag() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-break/resource", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest/resource HTTP/1.1",
		"nginx backend should see rewritten URI (break)")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest/resource HTTP/1.1",
		"traefik backend should see rewritten URI (break)")
}

func (s *SnippetRewriteSuite) TestRewriteInConfigSnippet() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-cfg/resource", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-cfg-dest/resource HTTP/1.1",
		"nginx backend should see rewritten URI (config-snippet)")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-cfg-dest/resource HTTP/1.1",
		"traefik backend should see rewritten URI (config-snippet)")
}

func (s *SnippetRewriteSuite) TestRewritePreservesQuery() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-query?q=test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest?q=test HTTP/1.1",
		"nginx backend should see rewritten URI with preserved query")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest?q=test HTTP/1.1",
		"traefik backend should see rewritten URI with preserved query")
}

func (s *SnippetRewriteSuite) TestRewriteSuppressesQuery() {
	// The trailing ? in the replacement suppresses the original query string.
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-noquery?q=test", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-dest HTTP/1.1",
		"nginx backend should see rewritten URI without query")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-dest HTTP/1.1",
		"traefik backend should see rewritten URI without query")
}

func (s *SnippetRewriteSuite) TestRewriteInIfBlock() {
	// The rewrite inside `if ($request_method = POST)` should only fire for POST.
	traefikResp, nginxResp := s.request(http.MethodPost, "/rw-if/page", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "POST /rw-dest/page HTTP/1.1",
		"nginx backend should see rewritten URI (if block, POST)")
	assert.Contains(s.T(), traefikResp.Body, "POST /rw-dest/page HTTP/1.1",
		"traefik backend should see rewritten URI (if block, POST)")
}

func (s *SnippetRewriteSuite) TestRewriteInIfBlockNoMatch() {
	// GET to /rw-if/page should NOT trigger the rewrite (if condition doesn't match).
	traefikResp, nginxResp := s.request(http.MethodGet, "/rw-if/page", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Contains(s.T(), nginxResp.Body, "GET /rw-if/page HTTP/1.1",
		"nginx backend should see original URI (if condition not met)")
	assert.Contains(s.T(), traefikResp.Body, "GET /rw-if/page HTTP/1.1",
		"traefik backend should see original URI (if condition not met)")
}
