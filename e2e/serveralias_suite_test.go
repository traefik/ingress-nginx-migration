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
	serverAliasIngressName = "server-alias-test"
	serverAliasTraefikHost = serverAliasIngressName + ".traefik.local"
	serverAliasNginxHost   = serverAliasIngressName + ".nginx.local"

	serverAliasAltTraefikHost = "alias-alt.traefik.local"
	serverAliasAltNginxHost   = "alias-alt.nginx.local"

	multiAliasIngressName = "multi-alias-test"
	multiAliasTraefikHost = multiAliasIngressName + ".traefik.local"
	multiAliasNginxHost   = multiAliasIngressName + ".nginx.local"

	multiAlias1TraefikHost = "alias1.traefik.local"
	multiAlias1NginxHost   = "alias1.nginx.local"
	multiAlias2TraefikHost = "alias2.traefik.local"
	multiAlias2NginxHost   = "alias2.nginx.local"
)

type ServerAliasSuite struct {
	BaseSuite
}

func TestServerAliasSuite(t *testing.T) {
	suite.Run(t, new(ServerAliasSuite))
}

func (s *ServerAliasSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Single alias ingress.
	traefikAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-alias": serverAliasAltTraefikHost,
	}
	err := s.traefik.DeployIngress(serverAliasIngressName, serverAliasTraefikHost, traefikAnnotations)
	require.NoError(s.T(), err, "deploy server-alias ingress to traefik cluster")

	nginxAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-alias": serverAliasAltNginxHost,
	}
	err = s.nginx.DeployIngress(serverAliasIngressName, serverAliasNginxHost, nginxAnnotations)
	require.NoError(s.T(), err, "deploy server-alias ingress to nginx cluster")

	// Multiple aliases ingress.
	multiTraefikAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-alias": multiAlias1TraefikHost + "," + multiAlias2TraefikHost,
	}
	err = s.traefik.DeployIngress(multiAliasIngressName, multiAliasTraefikHost, multiTraefikAnnotations)
	require.NoError(s.T(), err, "deploy multi-alias ingress to traefik cluster")

	multiNginxAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-alias": multiAlias1NginxHost + "," + multiAlias2NginxHost,
	}
	err = s.nginx.DeployIngress(multiAliasIngressName, multiAliasNginxHost, multiNginxAnnotations)
	require.NoError(s.T(), err, "deploy multi-alias ingress to nginx cluster")

	// Wait for primary hosts to be ready.
	s.traefik.WaitForIngressReady(s.T(), serverAliasTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), serverAliasNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), multiAliasTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), multiAliasNginxHost, 20, 1*time.Second)

	// Wait for alias hosts to be ready.
	s.traefik.WaitForIngressReady(s.T(), serverAliasAltTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), serverAliasAltNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), multiAlias1TraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), multiAlias1NginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), multiAlias2TraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), multiAlias2NginxHost, 20, 1*time.Second)
}

func (s *ServerAliasSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(serverAliasIngressName)
	_ = s.nginx.DeleteIngress(serverAliasIngressName)
	_ = s.traefik.DeleteIngress(multiAliasIngressName)
	_ = s.nginx.DeleteIngress(multiAliasIngressName)
}

func (s *ServerAliasSuite) TestPrimaryHostServesNormally() {
	traefikResp := s.traefik.MakeRequest(s.T(), serverAliasTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), serverAliasNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for primary host")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for primary host")
}

func (s *ServerAliasSuite) TestAliasHostServesContent() {
	traefikResp := s.traefik.MakeRequest(s.T(), serverAliasAltTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), serverAliasAltNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for alias host")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for alias host")
}

func (s *ServerAliasSuite) TestAliasHostOnSubpath() {
	traefikResp := s.traefik.MakeRequest(s.T(), serverAliasAltTraefikHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), serverAliasAltNginxHost, http.MethodGet, "/some/path", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for alias host on subpath")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for alias host on subpath")
}

func (s *ServerAliasSuite) TestNonAliasHostReturns404() {
	unknownHost := "unknown-alias.traefik.local"
	traefikResp := s.traefik.MakeRequest(s.T(), unknownHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	unknownNginxHost := "unknown-alias.nginx.local"
	nginxResp := s.nginx.MakeRequest(s.T(), unknownNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), http.StatusNotFound, traefikResp.StatusCode, "traefik should return 404 for non-alias host")
	assert.Equal(s.T(), http.StatusNotFound, nginxResp.StatusCode, "nginx should return 404 for non-alias host")
}

func (s *ServerAliasSuite) TestMultipleAliases() {
	// First alias.
	traefikResp1 := s.traefik.MakeRequest(s.T(), multiAlias1TraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp1, "traefik response should not be nil")

	nginxResp1 := s.nginx.MakeRequest(s.T(), multiAlias1NginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp1, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp1.StatusCode, traefikResp1.StatusCode, "status code mismatch for first alias")
	assert.Equal(s.T(), http.StatusOK, traefikResp1.StatusCode, "expected 200 for first alias on traefik")
	assert.Equal(s.T(), http.StatusOK, nginxResp1.StatusCode, "expected 200 for first alias on nginx")

	// Second alias.
	traefikResp2 := s.traefik.MakeRequest(s.T(), multiAlias2TraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp2, "traefik response should not be nil")

	nginxResp2 := s.nginx.MakeRequest(s.T(), multiAlias2NginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp2, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp2.StatusCode, traefikResp2.StatusCode, "status code mismatch for second alias")
	assert.Equal(s.T(), http.StatusOK, traefikResp2.StatusCode, "expected 200 for second alias on traefik")
	assert.Equal(s.T(), http.StatusOK, nginxResp2.StatusCode, "expected 200 for second alias on nginx")
}
