package e2e

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	basicAuthIngressName = "basic-auth-test"
	basicAuthTraefikHost = basicAuthIngressName + ".traefik.local"
	basicAuthNginxHost   = basicAuthIngressName + ".nginx.local"

	basicAuthUser = "testuser"
	basicAuthPass = "testpass"
	basicAuthRealm = "Test Realm"
)

type BasicAuthSuite struct {
	BaseSuite
}

func TestBasicAuthSuite(t *testing.T) {
	suite.Run(t, new(BasicAuthSuite))
}

func (s *BasicAuthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Create the htpasswd secret on both clusters.
	htpasswd := htpasswdSHA(basicAuthUser, basicAuthPass)
	secretManifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: basic-auth
type: Opaque
data:
  auth: %s
`, base64.StdEncoding.EncodeToString([]byte(htpasswd)))

	err := s.traefik.ApplyManifest(secretManifest)
	require.NoError(s.T(), err, "create basic-auth secret in traefik cluster")

	err = s.nginx.ApplyManifest(secretManifest)
	require.NoError(s.T(), err, "create basic-auth secret in nginx cluster")

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-type":   "basic",
		"nginx.ingress.kubernetes.io/auth-secret": "basic-auth",
		"nginx.ingress.kubernetes.io/auth-realm":  basicAuthRealm,
	}

	err = s.traefik.DeployIngress(basicAuthIngressName, basicAuthTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy basic-auth ingress to traefik cluster")

	err = s.nginx.DeployIngress(basicAuthIngressName, basicAuthNginxHost, annotations)
	require.NoError(s.T(), err, "deploy basic-auth ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), basicAuthTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), basicAuthNginxHost, 20, 1*time.Second)
}

func (s *BasicAuthSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(basicAuthIngressName)
	_ = s.nginx.DeleteIngress(basicAuthIngressName)
	_ = s.traefik.Kubectl("delete", "secret", "basic-auth", "-n", s.traefik.TestNamespace, "--ignore-not-found")
	_ = s.nginx.Kubectl("delete", "secret", "basic-auth", "-n", s.nginx.TestNamespace, "--ignore-not-found")
}

func basicAuthHeader(user, pass string) map[string]string {
	creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return map[string]string{
		"Authorization": "Basic " + creds,
	}
}

// request makes the same HTTP request against both clusters and returns both responses.
func (s *BasicAuthSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), basicAuthTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), basicAuthNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *BasicAuthSuite) TestNoCredentials() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode, "expected 401 without credentials")
}

func (s *BasicAuthSuite) TestCorrectCredentials() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", basicAuthHeader(basicAuthUser, basicAuthPass))

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with correct credentials")
}

func (s *BasicAuthSuite) TestWrongPassword() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", basicAuthHeader(basicAuthUser, "wrongpass"))

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode, "expected 401 with wrong password")
}

func (s *BasicAuthSuite) TestWrongUsername() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", basicAuthHeader("wronguser", basicAuthPass))

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode, "expected 401 with wrong username")
}

func (s *BasicAuthSuite) TestAuthRealm() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("WWW-Authenticate"),
		traefikResp.ResponseHeaders.Get("WWW-Authenticate"),
		"WWW-Authenticate header mismatch",
	)
}
