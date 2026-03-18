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
	permRedirectIngressName    = "perm-redirect-test"
	tempRedirectIngressName    = "temp-redirect-test"
	permRedirect308IngressName = "perm-redirect-308-test"
	tempRedirect307IngressName = "temp-redirect-307-test"

	permRedirectTraefikHost    = permRedirectIngressName + ".traefik.local"
	permRedirectNginxHost      = permRedirectIngressName + ".nginx.local"
	tempRedirectTraefikHost    = tempRedirectIngressName + ".traefik.local"
	tempRedirectNginxHost      = tempRedirectIngressName + ".nginx.local"
	permRedirect308TraefikHost = permRedirect308IngressName + ".traefik.local"
	permRedirect308NginxHost   = permRedirect308IngressName + ".nginx.local"
	tempRedirect307TraefikHost = tempRedirect307IngressName + ".traefik.local"
	tempRedirect307NginxHost   = tempRedirect307IngressName + ".nginx.local"

	bothRedirectIngressName = "both-redirect-test"
	bothRedirectTraefikHost = bothRedirectIngressName + ".traefik.local"
	bothRedirectNginxHost   = bothRedirectIngressName + ".nginx.local"
)

type RedirectSuite struct {
	BaseSuite
}

func TestRedirectSuite(t *testing.T) {
	suite.Run(t, new(RedirectSuite))
}

func (s *RedirectSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// 1. Permanent redirect (301).
	permAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/permanent-redirect": "https://example.com/new-home",
	}

	err := s.traefik.DeployIngress(permRedirectIngressName, permRedirectTraefikHost, permAnnotations)
	require.NoError(s.T(), err, "deploy permanent-redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(permRedirectIngressName, permRedirectNginxHost, permAnnotations)
	require.NoError(s.T(), err, "deploy permanent-redirect ingress to nginx cluster")

	// 2. Temporal redirect (302).
	tempAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/temporal-redirect": "https://example.com/temp",
	}

	err = s.traefik.DeployIngress(tempRedirectIngressName, tempRedirectTraefikHost, tempAnnotations)
	require.NoError(s.T(), err, "deploy temporal-redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(tempRedirectIngressName, tempRedirectNginxHost, tempAnnotations)
	require.NoError(s.T(), err, "deploy temporal-redirect ingress to nginx cluster")

	// 3. Permanent redirect with custom code 308.
	perm308Annotations := map[string]string{
		"nginx.ingress.kubernetes.io/permanent-redirect":      "https://example.com/new-home",
		"nginx.ingress.kubernetes.io/permanent-redirect-code": "308",
	}

	err = s.traefik.DeployIngress(permRedirect308IngressName, permRedirect308TraefikHost, perm308Annotations)
	require.NoError(s.T(), err, "deploy permanent-redirect-308 ingress to traefik cluster")

	err = s.nginx.DeployIngress(permRedirect308IngressName, permRedirect308NginxHost, perm308Annotations)
	require.NoError(s.T(), err, "deploy permanent-redirect-308 ingress to nginx cluster")

	// 4. Temporal redirect with custom code 307.
	temp307Annotations := map[string]string{
		"nginx.ingress.kubernetes.io/temporal-redirect":      "https://example.com/temp-307",
		"nginx.ingress.kubernetes.io/temporal-redirect-code": "307",
	}

	err = s.traefik.DeployIngress(tempRedirect307IngressName, tempRedirect307TraefikHost, temp307Annotations)
	require.NoError(s.T(), err, "deploy temporal-redirect-307 ingress to traefik cluster")

	err = s.nginx.DeployIngress(tempRedirect307IngressName, tempRedirect307NginxHost, temp307Annotations)
	require.NoError(s.T(), err, "deploy temporal-redirect-307 ingress to nginx cluster")

	// 5. Both temporal and permanent redirect (temporal takes precedence).
	bothAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/permanent-redirect": "https://example.com/permanent",
		"nginx.ingress.kubernetes.io/temporal-redirect":  "https://example.com/temporal",
	}

	err = s.traefik.DeployIngress(bothRedirectIngressName, bothRedirectTraefikHost, bothAnnotations)
	require.NoError(s.T(), err, "deploy both-redirect ingress to traefik cluster")

	err = s.nginx.DeployIngress(bothRedirectIngressName, bothRedirectNginxHost, bothAnnotations)
	require.NoError(s.T(), err, "deploy both-redirect ingress to nginx cluster")

	// Wait for all ingresses to be ready.
	s.traefik.WaitForIngressReady(s.T(), permRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), permRedirectNginxHost, 20, 1*time.Second)

	s.traefik.WaitForIngressReady(s.T(), tempRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), tempRedirectNginxHost, 20, 1*time.Second)

	s.traefik.WaitForIngressReady(s.T(), permRedirect308TraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), permRedirect308NginxHost, 20, 1*time.Second)

	s.traefik.WaitForIngressReady(s.T(), tempRedirect307TraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), tempRedirect307NginxHost, 20, 1*time.Second)

	s.traefik.WaitForIngressReady(s.T(), bothRedirectTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bothRedirectNginxHost, 20, 1*time.Second)
}

func (s *RedirectSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(permRedirectIngressName)
	_ = s.nginx.DeleteIngress(permRedirectIngressName)

	_ = s.traefik.DeleteIngress(tempRedirectIngressName)
	_ = s.nginx.DeleteIngress(tempRedirectIngressName)

	_ = s.traefik.DeleteIngress(permRedirect308IngressName)
	_ = s.nginx.DeleteIngress(permRedirect308IngressName)

	_ = s.traefik.DeleteIngress(tempRedirect307IngressName)
	_ = s.nginx.DeleteIngress(tempRedirect307IngressName)

	_ = s.traefik.DeleteIngress(bothRedirectIngressName)
	_ = s.nginx.DeleteIngress(bothRedirectIngressName)
}

// requestTo makes the same HTTP request against both clusters for a given host pair and returns both responses.
func (s *RedirectSuite) requestTo(traefikHost, nginxHost, method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), traefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), nginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *RedirectSuite) TestPermanentRedirectStatus() {
	traefikResp, nginxResp := s.requestTo(permRedirectTraefikHost, permRedirectNginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusMovedPermanently, traefikResp.StatusCode, "expected 301 for permanent redirect")
}

func (s *RedirectSuite) TestPermanentRedirectLocation() {
	traefikResp, nginxResp := s.requestTo(permRedirectTraefikHost, permRedirectNginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		traefikResp.ResponseHeaders.Get("Location"),
		"Location header mismatch",
	)
	assert.Equal(s.T(), "https://example.com/new-home", traefikResp.ResponseHeaders.Get("Location"),
		"expected Location header to be https://example.com/new-home",
	)
}

func (s *RedirectSuite) TestTemporalRedirectStatus() {
	traefikResp, nginxResp := s.requestTo(tempRedirectTraefikHost, tempRedirectNginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302 for temporal redirect")
}

func (s *RedirectSuite) TestTemporalRedirectLocation() {
	traefikResp, nginxResp := s.requestTo(tempRedirectTraefikHost, tempRedirectNginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		traefikResp.ResponseHeaders.Get("Location"),
		"Location header mismatch",
	)
	assert.Equal(s.T(), "https://example.com/temp", traefikResp.ResponseHeaders.Get("Location"),
		"expected Location header to be https://example.com/temp",
	)
}

func (s *RedirectSuite) TestPermanentRedirectCustomCode() {
	traefikResp, nginxResp := s.requestTo(permRedirect308TraefikHost, permRedirect308NginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusPermanentRedirect, traefikResp.StatusCode, "expected 308 for permanent redirect with custom code")
}

func (s *RedirectSuite) TestTemporalRedirectCustomCode() {
	traefikResp, nginxResp := s.requestTo(tempRedirect307TraefikHost, tempRedirect307NginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusTemporaryRedirect, traefikResp.StatusCode, "expected 307 for temporal redirect with custom code")
}

func (s *RedirectSuite) TestTemporalRedirectCustomCodeLocation() {
	traefikResp, nginxResp := s.requestTo(tempRedirect307TraefikHost, tempRedirect307NginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		traefikResp.ResponseHeaders.Get("Location"),
		"Location header mismatch",
	)
	assert.Equal(s.T(), "https://example.com/temp-307", traefikResp.ResponseHeaders.Get("Location"),
		"expected Location header to be https://example.com/temp-307",
	)
}

func (s *RedirectSuite) TestPermanentRedirectPreservesMethod() {
	traefikResp, nginxResp := s.requestTo(permRedirectTraefikHost, permRedirectNginxHost, http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for POST request")
	assert.Equal(s.T(), http.StatusMovedPermanently, traefikResp.StatusCode, "expected 301 for POST to permanent redirect")
}

func (s *RedirectSuite) TestTemporalPrecedenceOverPermanent() {
	traefikResp, nginxResp := s.requestTo(bothRedirectTraefikHost, bothRedirectNginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusFound, traefikResp.StatusCode, "expected 302 when both temporal and permanent redirects are set (temporal takes precedence)")

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		traefikResp.ResponseHeaders.Get("Location"),
		"Location header mismatch",
	)
	assert.Equal(s.T(), "https://example.com/temporal", traefikResp.ResponseHeaders.Get("Location"),
		"expected Location header to be https://example.com/temporal (temporal takes precedence)",
	)
}

func (s *RedirectSuite) TestPermanentRedirectCustomCodeLocation() {
	traefikResp, nginxResp := s.requestTo(permRedirect308TraefikHost, permRedirect308NginxHost, http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("Location"),
		traefikResp.ResponseHeaders.Get("Location"),
		"Location header mismatch",
	)
	assert.Equal(s.T(), "https://example.com/new-home", traefikResp.ResponseHeaders.Get("Location"),
		"expected Location header to be https://example.com/new-home",
	)
}
