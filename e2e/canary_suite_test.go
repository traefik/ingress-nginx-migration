package e2e

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type canaryScenario struct {
	canaryAnnotations map[string]string
}

var canaryScenarios = map[string]canaryScenario{
	"canary-wt-all": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":        "true",
			"nginx.ingress.kubernetes.io/canary-weight": "100",
		},
	},
	"canary-wt-none": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":        "true",
			"nginx.ingress.kubernetes.io/canary-weight": "0",
		},
	},
	"canary-wt-total": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":              "true",
			"nginx.ingress.kubernetes.io/canary-weight":       "200",
			"nginx.ingress.kubernetes.io/canary-weight-total": "200",
		},
	},
	"canary-hdr": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":           "true",
			"nginx.ingress.kubernetes.io/canary-by-header": "X-Canary",
		},
	},
	"canary-hdr-val": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":                 "true",
			"nginx.ingress.kubernetes.io/canary-by-header":       "X-Canary",
			"nginx.ingress.kubernetes.io/canary-by-header-value": "route-to-canary",
		},
	},
	"canary-hdr-pat": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":                   "true",
			"nginx.ingress.kubernetes.io/canary-by-header":         "X-Canary",
			"nginx.ingress.kubernetes.io/canary-by-header-pattern": "^(lab|staging)$",
		},
	},
	"canary-cookie": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":           "true",
			"nginx.ingress.kubernetes.io/canary-by-cookie": "canary_enabled",
		},
	},
	"canary-hdr-wt": {
		canaryAnnotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary":           "true",
			"nginx.ingress.kubernetes.io/canary-by-header": "X-Canary",
			"nginx.ingress.kubernetes.io/canary-weight":    "100",
		},
	},
}

type CanarySuite struct {
	BaseSuite
}

func TestCanarySuite(t *testing.T) {
	suite.Run(t, new(CanarySuite))
}

func isCanaryBackend(resp *Response) bool {
	return strings.HasPrefix(resp.RequestHeaders["Hostname"], "canary-backend")
}

func isProductionBackend(resp *Response) bool {
	return strings.HasPrefix(resp.RequestHeaders["Hostname"], "backend")
}

func (s *CanarySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Deploy canary backend (shared across all scenarios).
	err := s.traefik.ApplyFixture("canary-backend.yaml")
	require.NoError(s.T(), err, "deploy canary-backend fixture")

	err = waitForDeployment(s.traefik, testNamespace, "canary-backend")
	require.NoError(s.T(), err, "canary-backend deployment not ready")
}

func (s *CanarySuite) TearDownSuite() {
	_ = s.traefik.Kubectl("delete", "-f", filepath.Join(fixturesDir, "canary-backend.yaml"), "-n", testNamespace, "--ignore-not-found")
}

// deployScenario deploys the production and canary ingresses for a scenario
// on both clusters and waits for the canary to be active.
func (s *CanarySuite) deployScenario(name string) {
	t := s.T()
	t.Helper()

	sc := canaryScenarios[name]
	traefikHost := name + ".traefik.local"
	nginxHost := name + ".nginx.local"

	// Production ingresses.
	err := s.traefik.DeployIngressWith(ingressTemplateData{
		Name: name + "-prod",
		Host: traefikHost,
	})
	require.NoError(t, err, "deploy prod ingress %s to traefik", name)

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name: name + "-prod",
		Host: nginxHost,
	})
	require.NoError(t, err, "deploy prod ingress %s to nginx", name)

	// Wait for production to be routable before adding canary.
	s.traefik.WaitForIngressReady(t, traefikHost, 30, 1*time.Second)
	s.nginx.WaitForIngressReady(t, nginxHost, 30, 1*time.Second)

	// Canary ingresses.
	err = s.traefik.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        traefikHost,
		Annotations: sc.canaryAnnotations,
		ServiceName: "canary-backend",
		ServicePort: 80,
	})
	require.NoError(t, err, "deploy canary ingress %s to traefik", name)

	err = s.nginx.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        nginxHost,
		Annotations: sc.canaryAnnotations,
		ServiceName: "canary-backend",
		ServicePort: 80,
	})
	require.NoError(t, err, "deploy canary ingress %s to nginx", name)
}

// teardownScenario removes the ingresses for a scenario from both clusters.
func (s *CanarySuite) teardownScenario(name string) {
	_ = s.traefik.DeleteIngress(name)
	_ = s.nginx.DeleteIngress(name)
	_ = s.traefik.DeleteIngress(name + "-prod")
	_ = s.nginx.DeleteIngress(name + "-prod")
}

// pollForBackend polls the cluster until the response matches the check
// function, or returns nil after maxRetries.
func (s *CanarySuite) pollForBackend(c *Cluster, host string, headers map[string]string, check func(*Response) bool, maxRetries int, delay time.Duration) *Response {
	t := s.T()
	t.Helper()

	var last *Response
	for i := range maxRetries {
		resp := c.MakeRequest(t, host, http.MethodGet, "/", headers, 1, 0)
		if resp != nil && resp.StatusCode == http.StatusOK && check(resp) {
			return resp
		}
		last = resp
		if i < maxRetries-1 {
			time.Sleep(delay)
		}
	}
	if last != nil {
		t.Logf("[%s] expected backend not reached for %s after %d retries (last hostname: %s)",
			c.Name, host, maxRetries, last.RequestHeaders["Hostname"])
	}
	return nil
}

// assertCanaryRouting verifies that NGINX (reference) routes to the canary
// backend, then asserts Traefik matches.
func (s *CanarySuite) assertCanaryRouting(scenario string, headers map[string]string) {
	s.T().Helper()

	nginxResp := s.pollForBackend(s.nginx, scenario+".nginx.local", headers, isCanaryBackend, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp,
		"nginx should route to canary backend for %s", scenario)

	traefikResp := s.pollForBackend(s.traefik, scenario+".traefik.local", headers, isCanaryBackend, 10, 1*time.Second)
	assert.NotNil(s.T(), traefikResp,
		"traefik should route to canary backend (matching nginx) for %s", scenario)
}

// assertProductionRouting verifies that NGINX (reference) routes to the
// production backend, then asserts Traefik matches.
func (s *CanarySuite) assertProductionRouting(scenario string, headers map[string]string) {
	s.T().Helper()

	nginxResp := s.pollForBackend(s.nginx, scenario+".nginx.local", headers, isProductionBackend, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp,
		"nginx should route to production backend for %s", scenario)

	traefikResp := s.pollForBackend(s.traefik, scenario+".traefik.local", headers, isProductionBackend, 10, 1*time.Second)
	assert.NotNil(s.T(), traefikResp,
		"traefik should route to production backend (matching nginx) for %s", scenario)
}

// --- Weight tests ---

func (s *CanarySuite) TestWeightAll() {
	s.deployScenario("canary-wt-all")
	defer s.teardownScenario("canary-wt-all")

	s.assertCanaryRouting("canary-wt-all", nil)
}

func (s *CanarySuite) TestWeightNone() {
	s.deployScenario("canary-wt-none")
	defer s.teardownScenario("canary-wt-none")

	s.assertProductionRouting("canary-wt-none", nil)
}

func (s *CanarySuite) TestWeightTotal() {
	s.deployScenario("canary-wt-total")
	defer s.teardownScenario("canary-wt-total")

	s.assertCanaryRouting("canary-wt-total", nil)
}

// --- Header tests ---

func (s *CanarySuite) TestHeader() {
	s.deployScenario("canary-hdr")
	defer s.teardownScenario("canary-hdr")

	s.Run("AlwaysRoutesToCanary", func() {
		s.assertCanaryRouting("canary-hdr", map[string]string{"X-Canary": "always"})
	})
	s.Run("NeverRoutesToProduction", func() {
		s.assertProductionRouting("canary-hdr", map[string]string{"X-Canary": "never"})
	})
	s.Run("AbsentRoutesToProduction", func() {
		s.assertProductionRouting("canary-hdr", nil)
	})
}

// --- Header value tests ---

func (s *CanarySuite) TestHeaderValue() {
	s.deployScenario("canary-hdr-val")
	defer s.teardownScenario("canary-hdr-val")

	s.Run("MatchRoutesToCanary", func() {
		s.assertCanaryRouting("canary-hdr-val", map[string]string{"X-Canary": "route-to-canary"})
	})
	s.Run("MismatchRoutesToProduction", func() {
		s.assertProductionRouting("canary-hdr-val", map[string]string{"X-Canary": "something-else"})
	})
	s.Run("AbsentRoutesToProduction", func() {
		s.assertProductionRouting("canary-hdr-val", nil)
	})
}

// --- Header pattern tests ---

func (s *CanarySuite) TestHeaderPattern() {
	s.deployScenario("canary-hdr-pat")
	defer s.teardownScenario("canary-hdr-pat")

	s.Run("LabRoutesToCanary", func() {
		s.assertCanaryRouting("canary-hdr-pat", map[string]string{"X-Canary": "lab"})
	})
	s.Run("StagingRoutesToCanary", func() {
		s.assertCanaryRouting("canary-hdr-pat", map[string]string{"X-Canary": "staging"})
	})
	s.Run("OtherRoutesToProduction", func() {
		s.assertProductionRouting("canary-hdr-pat", map[string]string{"X-Canary": "other"})
	})
}

// --- Cookie tests ---

func (s *CanarySuite) TestCookie() {
	s.deployScenario("canary-cookie")
	defer s.teardownScenario("canary-cookie")

	s.Run("AlwaysRoutesToCanary", func() {
		s.assertCanaryRouting("canary-cookie", map[string]string{"Cookie": "canary_enabled=always"})
	})
	s.Run("NeverRoutesToProduction", func() {
		s.assertProductionRouting("canary-cookie", map[string]string{"Cookie": "canary_enabled=never"})
	})
	s.Run("AbsentRoutesToProduction", func() {
		s.assertProductionRouting("canary-cookie", nil)
	})
	s.Run("MultiCookieRoutesToCanary", func() {
		s.assertCanaryRouting("canary-cookie", map[string]string{"Cookie": "other_cookie=foo; canary_enabled=always"})
	})
}

// --- Header + weight precedence tests ---

func (s *CanarySuite) TestHeaderWeight() {
	s.deployScenario("canary-hdr-wt")
	defer s.teardownScenario("canary-hdr-wt")

	s.Run("AlwaysRoutesToCanary", func() {
		s.assertCanaryRouting("canary-hdr-wt", map[string]string{"X-Canary": "always"})
	})
	s.Run("NeverRoutesToProduction", func() {
		s.assertProductionRouting("canary-hdr-wt", map[string]string{"X-Canary": "never"})
	})
	s.Run("AbsentFallsBackToWeight", func() {
		s.assertCanaryRouting("canary-hdr-wt", nil)
	})
}
