package e2e

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	serverAliasAuthSecretName = "server-alias-auth" // #nosec G101 -- Kubernetes Secret resource name, not a credential.
	serverAliasAuthPath       = "/admin/report"
	serverAliasAuthUser       = "aliasuser"
	serverAliasAuthPass       = "aliaspass"
	serverAliasAuthRealm      = "Server alias"
)

// serverAliasAuthScenario deploys, on one host, a basic-auth-protected Exact
// path and a public Prefix path whose Ingress declares the server-alias.
type serverAliasAuthScenario struct {
	host       string
	alias      string
	prefixPath string
}

var serverAliasAuthScenarios = []serverAliasAuthScenario{
	// The Exact rule is longer than the Prefix rule, so only the alias scope matters.
	{host: "server-alias-auth", alias: "server-alias-auth-alt", prefixPath: "/"},
	// The Prefix rule is longer than the Exact rule, so Exact precedence matters too.
	{host: "server-alias-nested-auth", alias: "server-alias-nested-auth-alt", prefixPath: "/admin"},
}

func (sc serverAliasAuthScenario) exactIngressName() string  { return sc.host + "-exact" }
func (sc serverAliasAuthScenario) publicIngressName() string { return sc.host + "-public" }

// ServerAliasAuthSuite checks that a server-alias declared by one Ingress also
// serves the paths of the other Ingresses of the host, so an alias cannot be
// used to reach a protected path through another Ingress's location.
type ServerAliasAuthSuite struct {
	BaseSuite
}

func TestServerAliasAuthSuite(t *testing.T) { //nolint:paralleltest // Suites share cluster resources and must run sequentially.
	suite.Run(t, new(ServerAliasAuthSuite))
}

func (s *ServerAliasAuthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeploySecret(secretTemplateData{
			Name: serverAliasAuthSecretName,
			Data: map[string]string{
				"auth": base64.StdEncoding.EncodeToString([]byte(htpasswdSHA(serverAliasAuthUser, serverAliasAuthPass))),
			},
		})
		s.Require().NoError(err, "deploy auth secret to %s cluster", c.Name)

		for _, sc := range serverAliasAuthScenarios {
			host := sc.host + "." + c.Name + ".local"
			alias := sc.alias + "." + c.Name + ".local"

			err = c.DeployIngressWith(ingressTemplateData{
				Name:     sc.exactIngressName(),
				Host:     host,
				Path:     serverAliasAuthPath,
				PathType: "Exact",
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/auth-type":   "basic",
					"nginx.ingress.kubernetes.io/auth-secret": serverAliasAuthSecretName,
					"nginx.ingress.kubernetes.io/auth-realm":  serverAliasAuthRealm,
				},
			})
			s.Require().NoError(err, "deploy Exact ingress %s to %s cluster", sc.host, c.Name)

			err = c.DeployIngressWith(ingressTemplateData{
				Name:     sc.publicIngressName(),
				Host:     host,
				Path:     sc.prefixPath,
				PathType: "Prefix",
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/server-alias": alias,
				},
			})
			s.Require().NoError(err, "deploy aliased Prefix ingress %s to %s cluster", sc.host, c.Name)

			// The alias only exists through the Prefix ingress; wait for it before testing the Exact path.
			s.Require().Eventually(func() bool {
				resp := c.MakeRequest(s.T(), alias, http.MethodGet, "/admin/other", nil, 1, time.Second)
				return resp != nil && resp.StatusCode == http.StatusOK && backendReached(resp)
			}, 30*time.Second, time.Second, "[%s] server-alias %s did not become ready", c.Name, alias)
		}
	}
}

func (s *ServerAliasAuthSuite) TearDownSuite() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		if c == nil {
			continue
		}
		if s.T().Failed() {
			s.T().Log(c.GetIngressControllerLogs(500))
		}
		for _, sc := range serverAliasAuthScenarios {
			_ = c.DeleteIngress(sc.exactIngressName())
			_ = c.DeleteIngress(sc.publicIngressName())
		}
		_ = c.DeleteSecret(serverAliasAuthSecretName)
	}
}

func (s *ServerAliasAuthSuite) TestNoCredentials() {
	s.assertResponse(serverAliasAuthPath, nil, http.StatusUnauthorized)
}

func (s *ServerAliasAuthSuite) TestWrongCredentials() {
	s.assertResponse(serverAliasAuthPath, basicAuthHeader(serverAliasAuthUser, "wrong"), http.StatusUnauthorized)
}

func (s *ServerAliasAuthSuite) TestCorrectCredentials() {
	s.assertResponse(serverAliasAuthPath, basicAuthHeader(serverAliasAuthUser, serverAliasAuthPass), http.StatusOK)
}

func (s *ServerAliasAuthSuite) TestOtherPathsRemainPublic() {
	for _, path := range []string{"/admin", "/admin/other", serverAliasAuthPath + "/child"} {
		s.Run(path, func() {
			s.assertResponse(path, nil, http.StatusOK)
		})
	}
}

// assertResponse checks the path on the host and the alias of every scenario, which must behave the same.
func (s *ServerAliasAuthSuite) assertResponse(path string, headers map[string]string, status int) {
	s.T().Helper()

	for _, sc := range serverAliasAuthScenarios {
		for _, name := range []string{sc.host, sc.alias} {
			s.Run(name, func() {
				for _, scheme := range []string{"http", "https"} {
					s.Run(scheme, func() {
						for _, c := range []*Cluster{s.nginx, s.traefik} {
							s.Run(c.Name, func() {
								host := name + "." + c.Name + ".local"
								var resp *Response
								if scheme == "https" {
									resp = c.MakeTLSRequest(s.T(), host, http.MethodGet, path, headers, nil, 3, time.Second)
								} else {
									resp = c.MakeRequest(s.T(), host, http.MethodGet, path, headers, 3, time.Second)
								}
								s.Require().NotNil(resp, "request to %s%s returned no response", host, path)
								s.Equal(status, resp.StatusCode, "unexpected status for %s%s", host, path)
								if status == http.StatusUnauthorized {
									s.Equal(`Basic realm="`+serverAliasAuthRealm+`"`, resp.ResponseHeaders.Get("WWW-Authenticate"))
									s.False(backendReached(resp), "protected backend served without valid credentials")
								} else {
									s.True(backendReached(resp), "request did not reach the backend")
								}
							})
						}
					})
				}
			})
		}
	}
}
