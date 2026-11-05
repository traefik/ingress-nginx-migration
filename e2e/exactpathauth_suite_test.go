package e2e

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	exactPathAuthHost          = "exact-path-auth"
	exactPathAuthIngressName   = "exact-path-auth-exact"
	exactPathPublicIngressName = "exact-path-auth-prefix"
	exactPathAuthSecretName    = "exact-path-auth" // #nosec G101 -- Kubernetes Secret resource name, not a credential.
	exactPathAuthPath          = "/admin/report"
	exactPathAuthUser          = "exactuser"
	exactPathAuthPass          = "exactpass"
	exactPathAuthRealm         = "Exact path"
)

// exactPathAuthVariant is one host on which the Exact and Prefix ingresses are deployed.
type exactPathAuthVariant struct {
	name              string
	suffix            string
	prefixAnnotations map[string]string
	// protectedPaths require credentials; publicPaths must reach the backend without them.
	protectedPaths []string
	publicPaths    []string
}

var exactPathAuthVariants = []exactPathAuthVariant{
	{
		name:           "literal",
		protectedPaths: []string{exactPathAuthPath},
		publicPaths:    []string{"/admin", "/admin/other", exactPathAuthPath + "/child"},
	},
	{
		// use-regex applies to every path of the host. ingress-nginx then renders every
		// location, the Exact one included, as a case-insensitive `~* "^<path>"` regex,
		// so the Exact path also protects what lies below it.
		name:              "use-regex",
		suffix:            "-regex",
		prefixAnnotations: map[string]string{"nginx.ingress.kubernetes.io/use-regex": "true"},
		protectedPaths:    []string{exactPathAuthPath, exactPathAuthPath + "/child"},
		publicPaths:       []string{"/admin", "/admin/other"},
	},
}

func (v exactPathAuthVariant) host(c *Cluster) string {
	return exactPathAuthHost + v.suffix + "." + c.Name + ".local"
}

// ExactPathAuthSuite checks that an enclosing public Prefix path cannot bypass
// the authentication configured on an Exact path on the same host.
type ExactPathAuthSuite struct {
	BaseSuite
}

func TestExactPathAuthSuite(t *testing.T) { //nolint:paralleltest // Suites share cluster resources and must run sequentially.
	suite.Run(t, new(ExactPathAuthSuite))
}

func (s *ExactPathAuthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeploySecret(secretTemplateData{
			Name: exactPathAuthSecretName,
			Data: map[string]string{
				"auth": base64.StdEncoding.EncodeToString([]byte(htpasswdSHA(exactPathAuthUser, exactPathAuthPass))),
			},
		})
		s.Require().NoError(err, "deploy auth secret to %s cluster", c.Name)

		for _, v := range exactPathAuthVariants {
			host := v.host(c)
			err = c.DeployIngressWith(ingressTemplateData{
				Name:     exactPathAuthIngressName + v.suffix,
				Host:     host,
				Path:     exactPathAuthPath,
				PathType: "Exact",
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/auth-type":   "basic",
					"nginx.ingress.kubernetes.io/auth-secret": exactPathAuthSecretName,
					"nginx.ingress.kubernetes.io/auth-realm":  exactPathAuthRealm,
				},
			})
			s.Require().NoError(err, "deploy %s Exact ingress to %s cluster", v.name, c.Name)

			// Observe authentication before adding the competing Prefix, so a missing
			// Exact ingress cannot be mistaken for a routing precedence failure.
			s.Require().Eventually(func() bool {
				resp := c.MakeRequest(s.T(), host, http.MethodGet, exactPathAuthPath, nil, 1, time.Second)
				return resp != nil && resp.StatusCode == http.StatusUnauthorized
			}, 30*time.Second, time.Second, "[%s] %s Exact path authentication did not become ready", c.Name, v.name)

			err = c.DeployIngressWith(ingressTemplateData{
				Name:        exactPathPublicIngressName + v.suffix,
				Host:        host,
				Path:        "/admin",
				PathType:    "Prefix",
				Annotations: v.prefixAnnotations,
			})
			s.Require().NoError(err, "deploy %s Prefix ingress to %s cluster", v.name, c.Name)

			// This path only matches the Prefix; wait for it before testing overlap.
			s.Require().Eventually(func() bool {
				resp := c.MakeRequest(s.T(), host, http.MethodGet, "/admin/other", nil, 1, time.Second)
				return resp != nil && resp.StatusCode == http.StatusOK && backendReached(resp)
			}, 30*time.Second, time.Second, "[%s] %s public Prefix path did not become ready", c.Name, v.name)
		}
	}
}

func (s *ExactPathAuthSuite) TearDownSuite() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		if c == nil {
			continue
		}
		if s.T().Failed() {
			s.T().Log(c.GetIngressControllerLogs(500))
		}
		for _, v := range exactPathAuthVariants {
			_ = c.DeleteIngress(exactPathAuthIngressName + v.suffix)
			_ = c.DeleteIngress(exactPathPublicIngressName + v.suffix)
		}
		_ = c.DeleteSecret(exactPathAuthSecretName)
	}
}

func (s *ExactPathAuthSuite) TestNoCredentials() {
	s.forEachPath(func(v exactPathAuthVariant) []string { return v.protectedPaths }, nil, http.StatusUnauthorized)
}

func (s *ExactPathAuthSuite) TestWrongCredentials() {
	s.forEachPath(func(v exactPathAuthVariant) []string { return v.protectedPaths }, basicAuthHeader(exactPathAuthUser, "wrong"), http.StatusUnauthorized)
}

func (s *ExactPathAuthSuite) TestCorrectCredentials() {
	s.forEachPath(func(v exactPathAuthVariant) []string { return v.protectedPaths }, basicAuthHeader(exactPathAuthUser, exactPathAuthPass), http.StatusOK)
}

func (s *ExactPathAuthSuite) TestPrefixRemainsPublic() {
	s.forEachPath(func(v exactPathAuthVariant) []string { return v.publicPaths }, nil, http.StatusOK)
}

func (s *ExactPathAuthSuite) forEachPath(paths func(exactPathAuthVariant) []string, headers map[string]string, status int) {
	s.T().Helper()

	for _, v := range exactPathAuthVariants {
		s.Run(v.name, func() {
			for _, path := range paths(v) {
				s.Run(path, func() {
					s.assertPathResponse(v, path, headers, status)
				})
			}
		})
	}
}

func (s *ExactPathAuthSuite) assertPathResponse(v exactPathAuthVariant, path string, headers map[string]string, status int) {
	s.T().Helper()

	for _, scheme := range []string{"http", "https"} {
		s.Run(scheme, func() {
			for _, c := range []*Cluster{s.nginx, s.traefik} {
				s.Run(c.Name, func() {
					host := v.host(c)
					var resp *Response
					if scheme == "https" {
						resp = c.MakeTLSRequest(s.T(), host, http.MethodGet, path, headers, nil, 3, time.Second)
					} else {
						resp = c.MakeRequest(s.T(), host, http.MethodGet, path, headers, 3, time.Second)
					}
					s.Require().NotNil(resp, "request to %s returned no response", path)
					s.Equal(status, resp.StatusCode, "unexpected status for %s", path)
					if status == http.StatusUnauthorized {
						s.Equal(`Basic realm="`+exactPathAuthRealm+`"`, resp.ResponseHeaders.Get("WWW-Authenticate"))
						s.False(backendReached(resp), "protected backend served without valid credentials")
					} else {
						s.True(backendReached(resp), "request did not reach the backend")
					}
				})
			}
		})
	}
}
