package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// The production Ingresses of the three hosts route to the same service.
	// The canary Ingress of the first host receives every request.
	canarySharedServiceAllHost = "canary-shared-all"
	// The second host has no canary Ingress.
	canarySharedServiceNoCanaryHost = "canary-shared-no-canary"
	// The canary Ingress of the third host receives no request. It is created last.
	canarySharedServiceNoneHost = "canary-shared-none"
)

// CanarySharedServiceSuite checks which hosts a canary Ingress applies to
// when the production Ingresses of several hosts route to the same service.
type CanarySharedServiceSuite struct {
	BaseSuite
}

func TestCanarySharedServiceSuite(t *testing.T) {
	suite.Run(t, new(CanarySharedServiceSuite))
}

func (s *CanarySharedServiceSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.T().Cleanup(s.cleanup)

	err := s.traefik.ApplyFixture("canary-backend.yaml")
	require.NoError(s.T(), err, "deploy canary-backend fixture")
	require.NoError(s.T(), waitForDeployment(s.traefik, testNamespace, "canary-backend"), "canary-backend deployment not ready")

	hosts := []string{canarySharedServiceAllHost, canarySharedServiceNoCanaryHost, canarySharedServiceNoneHost}
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		for _, host := range hosts {
			err = c.DeployIngressWith(ingressTemplateData{Name: host + "-prod", Host: host + "." + c.Name + ".local"})
			require.NoError(s.T(), err, "deploy %s-prod ingress to %s cluster", host, c.Name)
		}
		for _, host := range hosts {
			c.WaitForIngressReady(s.T(), host+"."+c.Name+".local", 30, time.Second)
		}
	}
}

func (s *CanarySharedServiceSuite) cleanup() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		for _, host := range []string{canarySharedServiceAllHost, canarySharedServiceNoCanaryHost, canarySharedServiceNoneHost} {
			_ = c.DeleteIngress(host + "-prod")
			_ = c.DeleteIngress(host + "-canary")
		}
	}
	_ = s.traefik.Kubectl("delete", "-f", fixturesDir+"/canary-backend.yaml", "-n", testNamespace, "--ignore-not-found")
}

// deployCanary deploys the canary Ingress of a host to both clusters, with the given weight.
func (s *CanarySharedServiceSuite) deployCanary(host, weight string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngressWith(ingressTemplateData{
			Name: host + "-canary",
			Host: host + "." + c.Name + ".local",
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/canary":        "true",
				"nginx.ingress.kubernetes.io/canary-weight": weight,
			},
			ServiceName: "canary-backend",
		})
		require.NoError(s.T(), err, "deploy %s-canary ingress to %s cluster", host, c.Name)
	}
}

// backends sends 15 requests to a host, and returns how many of them the production and the canary backends served.
func (s *CanarySharedServiceSuite) backends(c *Cluster, host string) map[string]int {
	s.T().Helper()

	served := map[string]int{}
	for range 15 {
		resp := c.MakeRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, "/", nil, 3, time.Second)
		require.NotNil(s.T(), resp, "%s: %s", c.Name, host)

		switch {
		case isCanaryBackend(resp):
			served["canary"]++
		case isProductionBackend(resp):
			served["production"]++
		default:
			served[strings.TrimSpace(resp.RequestHeaders["Hostname"])]++
		}
	}

	return served
}

// TestCanaryAppliesToEveryHostOfItsService checks that a canary Ingress applies to every host whose production Ingress
// routes to the same service, including a host without canary Ingress: ingress-nginx attaches the canary to the upstream.
// It runs before TestKnownDifference_FirstCanaryOfServiceWins, which adds a second canary Ingress.
func (s *CanarySharedServiceSuite) TestCanaryAppliesToEveryHostOfItsService() {
	s.deployCanary(canarySharedServiceAllHost, "100")
	// The controllers give no signal once they loaded a canary Ingress.
	time.Sleep(15 * time.Second)

	for _, c := range []*Cluster{s.nginx, s.traefik} {
		for _, host := range []string{canarySharedServiceAllHost, canarySharedServiceNoCanaryHost} {
			assert.Equal(s.T(), map[string]int{"canary": 15}, s.backends(c, host), "%s: %s", c.Name, host)
		}
	}
}

// TestKnownDifference_FirstCanaryOfServiceWins checks which canary Ingress applies when two of them, on different hosts,
// compete for the same production service: ingress-nginx keeps the first one, Traefik the last one.
func (s *CanarySharedServiceSuite) TestKnownDifference_FirstCanaryOfServiceWins() {
	// Ingresses are sorted by creation timestamp, whose resolution is the second.
	time.Sleep(2 * time.Second)
	s.deployCanary(canarySharedServiceNoneHost, "0")
	time.Sleep(15 * time.Second)

	for _, host := range []string{canarySharedServiceAllHost, canarySharedServiceNoneHost} {
		assert.Equal(s.T(), map[string]int{"canary": 15}, s.backends(s.nginx, host), "nginx: %s", host)
		assert.Equal(s.T(), map[string]int{"production": 15}, s.backends(s.traefik, host), "traefik: %s", host)
	}
}
