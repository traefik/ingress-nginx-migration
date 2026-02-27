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
	locationCIIngressName = "snippet-location-ci-test"
	locationCITraefikHost = locationCIIngressName + ".traefik.local"
	locationCINginxHost   = locationCIIngressName + ".nginx.local"
)

type SnippetAdvancedSuite struct {
	BaseSuite
}

func TestSnippetAdvancedSuite(t *testing.T) {
	suite.Run(t, new(SnippetAdvancedSuite))
}

func (s *SnippetAdvancedSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Location case-insensitive regex + return.
	locAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
location ~* \.css$ {
    add_header X-Type "css" always;
    return 200 "CSS";
}
`,
	}

	err := s.traefik.DeployIngress(locationCIIngressName, locationCITraefikHost, locAnnotations)
	require.NoError(s.T(), err, "deploy location-ci ingress to traefik cluster")

	err = s.nginx.DeployIngress(locationCIIngressName, locationCINginxHost, locAnnotations)
	require.NoError(s.T(), err, "deploy location-ci ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), locationCITraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), locationCINginxHost, 20, 1*time.Second)
}

func (s *SnippetAdvancedSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(locationCIIngressName)
	_ = s.nginx.DeleteIngress(locationCIIngressName)
}

func (s *SnippetAdvancedSuite) requestLocationCI(path string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), locationCITraefikHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), locationCINginxHost, http.MethodGet, path, nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- Location ~* case-insensitive regex ---

func (s *SnippetAdvancedSuite) TestLocationCaseInsensitiveRegexMatch() {
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

func (s *SnippetAdvancedSuite) TestLocationCaseInsensitiveRegexMatchLowercase() {
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

func (s *SnippetAdvancedSuite) TestLocationCaseInsensitiveRegexNoMatch() {
	// /style/main.js should NOT match ~* \.css$ and should fall through
	// to the default ingress location (backend proxy).
	traefikResp, nginxResp := s.requestLocationCI("/style/main.js")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	// Both should fall through to the backend.
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "should fall through to backend")
}
