package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// auth-url denies the request, so the client is redirected to auth-signin.
	vhostSigninIngress = "upstream-vhost-signin"

	// auth-url allows the request, so it reaches the backend.
	vhostAuthAllowIngress = "upstream-vhost-auth-allow"
)

// UpstreamVhostAuthSuite checks that upstream-vhost only applies to the request forwarded to the backend
// when external authentication is configured, as ingress-nginx renders it on the proxy_pass location only:
// the auth-signin redirect keeps the host requested by the client.
type UpstreamVhostAuthSuite struct {
	BaseSuite
}

func TestUpstreamVhostAuthSuite(t *testing.T) {
	suite.Run(t, new(UpstreamVhostAuthSuite))
}

func (s *UpstreamVhostAuthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.ApplyFixture("auth-server.yaml")
		require.NoError(s.T(), err, "deploy auth-server to %s cluster", c.Name)

		err = waitForDeployment(c, c.TestNamespace, "auth-server")
		require.NoError(s.T(), err, "auth-server not ready in %s cluster", c.Name)
	}

	s.deployIngress(vhostSigninIngress, map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":       authServerServiceURL + "/deny",
		"nginx.ingress.kubernetes.io/auth-signin":    "https://$host/oauth2/start",
		"nginx.ingress.kubernetes.io/upstream-vhost": customUpstreamHost,
	})
	s.deployIngress(vhostAuthAllowIngress, map[string]string{
		"nginx.ingress.kubernetes.io/auth-url":       authServerServiceURL + "/",
		"nginx.ingress.kubernetes.io/upstream-vhost": customUpstreamHost,
	})

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		s.waitForPath(c, vhostSigninIngress, "/")
		s.waitForPath(c, vhostAuthAllowIngress, "/")
	}
}

func (s *UpstreamVhostAuthSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		_ = c.DeleteIngress(vhostSigninIngress)
		_ = c.DeleteIngress(vhostAuthAllowIngress)
		_ = c.Kubectl("delete", "-f", fmt.Sprintf("%s/auth-server.yaml", fixturesDir), "-n", c.TestNamespace, "--ignore-not-found")
	}
}

// deployIngress deploys an Ingress on <name>.<cluster>.local to both clusters.
func (s *UpstreamVhostAuthSuite) deployIngress(name string, annotations map[string]string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngress(name, name+"."+c.Name+".local", annotations)
		require.NoError(s.T(), err, "deploy %s ingress to %s cluster", name, c.Name)
	}
}

// waitForPath waits until the controller routes the given path.
// Like WaitForIngressReady, it fails the suite when the path never starts routing, as the assertions would then prove nothing.
func (s *UpstreamVhostAuthSuite) waitForPath(c *Cluster, name, path string) {
	s.T().Helper()

	for range 30 {
		resp := c.MakeRequest(s.T(), name+"."+c.Name+".local", http.MethodGet, path, nil, 1, time.Second)
		if resp != nil && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadGateway {
			return
		}
		time.Sleep(time.Second)
	}

	s.T().Fatalf("[%s] path %s on ingress %s not ready after 30 retries", c.Name, path, name)
}

// TestAuthSigninKeepsClientHost checks that the auth-signin redirect, and the rd parameter appended to it,
// use the host requested by the client and not the upstream-vhost value.
func (s *UpstreamVhostAuthSuite) TestAuthSigninKeepsClientHost() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		host := vhostSigninIngress + "." + c.Name + ".local"

		resp := c.MakeRequest(s.T(), host, http.MethodGet, "/", nil, 3, time.Second)
		require.NotNil(s.T(), resp, "%s response should not be nil", c.Name)
		require.Equal(s.T(), http.StatusFound, resp.StatusCode, "%s: expected 302 to auth-signin", c.Name)

		location, err := url.Parse(resp.ResponseHeaders.Get("Location"))
		require.NoError(s.T(), err, "%s: parse Location", c.Name)
		assert.Equal(s.T(), host, location.Host, "%s: auth-signin should redirect to the client host", c.Name)

		rd, err := url.Parse(location.Query().Get("rd"))
		require.NoError(s.T(), err, "%s: parse rd", c.Name)
		assert.Equal(s.T(), host, rd.Host, "%s: rd should point back to the client host", c.Name)
	}
}

// TestAuthenticatedRequestUsesUpstreamVhost checks that a request allowed by auth-url still reaches the backend
// with the upstream-vhost value as Host.
func (s *UpstreamVhostAuthSuite) TestAuthenticatedRequestUsesUpstreamVhost() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		resp := c.MakeRequest(s.T(), vhostAuthAllowIngress+"."+c.Name+".local", http.MethodGet, "/", nil, 3, time.Second)
		require.NotNil(s.T(), resp, "%s response should not be nil", c.Name)
		require.Equal(s.T(), http.StatusOK, resp.StatusCode, "%s: expected 200 once authenticated", c.Name)

		assert.Equal(s.T(), customUpstreamHost, resp.RequestHeaders["Host"], "%s: backend should see Host: %s", c.Name, customUpstreamHost)
	}
}
