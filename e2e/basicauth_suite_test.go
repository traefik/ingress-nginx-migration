package e2e

import (
	"crypto/sha1"
	"encoding/base64"
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

	basicAuthUser  = "testuser"
	basicAuthPass  = "testpass"
	basicAuthRealm = "Test Realm"

	authMapIngressName = "auth-map-test"
	authMapTraefikHost = authMapIngressName + ".traefik.local"
	authMapNginxHost   = authMapIngressName + ".nginx.local"

	authMapUser = "mapuser"
	authMapPass = "mappass"
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
	basicAuthSecret := secretTemplateData{
		Name: "basic-auth",
		Data: map[string]string{
			"auth": base64.StdEncoding.EncodeToString([]byte(htpasswd)),
		},
	}

	err := s.traefik.DeploySecret(basicAuthSecret)
	require.NoError(s.T(), err, "create basic-auth secret in traefik cluster")

	err = s.nginx.DeploySecret(basicAuthSecret)
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

	// Create the auth-map secret where keys are usernames, values are password hashes.
	// The {SHA} hash format is: base64(sha1(password))
	mapHash := sha1Hash(authMapPass)
	authMapSecretData := secretTemplateData{
		Name: "auth-map-secret",
		Data: map[string]string{
			authMapUser: base64.StdEncoding.EncodeToString([]byte("{SHA}" + mapHash)),
		},
	}

	err = s.traefik.DeploySecret(authMapSecretData)
	require.NoError(s.T(), err, "create auth-map secret in traefik cluster")

	err = s.nginx.DeploySecret(authMapSecretData)
	require.NoError(s.T(), err, "create auth-map secret in nginx cluster")

	authMapAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/auth-type":        "basic",
		"nginx.ingress.kubernetes.io/auth-secret":      "auth-map-secret",
		"nginx.ingress.kubernetes.io/auth-secret-type": "auth-map",
		"nginx.ingress.kubernetes.io/auth-realm":       basicAuthRealm,
	}

	err = s.traefik.DeployIngress(authMapIngressName, authMapTraefikHost, authMapAnnotations)
	require.NoError(s.T(), err, "deploy auth-map ingress to traefik cluster")

	err = s.nginx.DeployIngress(authMapIngressName, authMapNginxHost, authMapAnnotations)
	require.NoError(s.T(), err, "deploy auth-map ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), basicAuthTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), basicAuthNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), authMapTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), authMapNginxHost, 20, 1*time.Second)
}

func (s *BasicAuthSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(basicAuthIngressName)
	_ = s.nginx.DeleteIngress(basicAuthIngressName)
	_ = s.traefik.DeleteSecret("basic-auth")
	_ = s.nginx.DeleteSecret("basic-auth")
	_ = s.traefik.DeleteIngress(authMapIngressName)
	_ = s.nginx.DeleteIngress(authMapIngressName)
	_ = s.traefik.DeleteSecret("auth-map-secret")
	_ = s.nginx.DeleteSecret("auth-map-secret")
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

// sha1Hash returns the base64-encoded SHA-1 hash of the given password.
func sha1Hash(password string) string {
	h := sha1.Sum([]byte(password))
	return base64.StdEncoding.EncodeToString(h[:])
}

// requestAuthMap makes the same HTTP request against both clusters using the auth-map ingress
// and returns both responses.
func (s *BasicAuthSuite) requestAuthMap(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), authMapTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), authMapNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *BasicAuthSuite) TestAuthMapNoCredentials() {
	traefikResp, nginxResp := s.requestAuthMap(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode, "expected 401 without credentials")
}

func (s *BasicAuthSuite) TestAuthMapCorrectCredentials() {
	traefikResp, nginxResp := s.requestAuthMap(http.MethodGet, "/", basicAuthHeader(authMapUser, authMapPass))

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with correct credentials")
}

func (s *BasicAuthSuite) TestAuthMapWrongPassword() {
	traefikResp, nginxResp := s.requestAuthMap(http.MethodGet, "/", basicAuthHeader(authMapUser, "wrongpass"))

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusUnauthorized, traefikResp.StatusCode, "expected 401 with wrong password")
}
