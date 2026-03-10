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

type AffinityCanarySuite struct {
	BaseSuite
}

func TestAffinityCanarySuite(t *testing.T) {
	suite.Run(t, new(AffinityCanarySuite))
}

func (s *AffinityCanarySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Deploy canary backend on both clusters.
	err := s.traefik.ApplyFixture("canary-backend.yaml")
	require.NoError(s.T(), err, "deploy canary-backend fixture to traefik")

	err = waitForDeployment(s.traefik, testNamespace, "canary-backend")
	require.NoError(s.T(), err, "canary-backend deployment not ready on traefik")

	err = s.nginx.ApplyFixture("canary-backend.yaml")
	require.NoError(s.T(), err, "deploy canary-backend fixture to nginx")

	err = waitForDeployment(s.nginx, testNamespace, "canary-backend")
	require.NoError(s.T(), err, "canary-backend deployment not ready on nginx")
}

func (s *AffinityCanarySuite) TearDownSuite() {
	_ = s.traefik.Kubectl("delete", "-f", filepath.Join(fixturesDir, "canary-backend.yaml"), "-n", testNamespace, "--ignore-not-found")
	_ = s.nginx.Kubectl("delete", "-f", filepath.Join(fixturesDir, "canary-backend.yaml"), "-n", testNamespace, "--ignore-not-found")
}

// deployAffinityCanary deploys a production ingress with session affinity and a
// canary ingress with the given affinity-canary-behavior value on a single cluster.
// If affinityCanaryBehavior is empty, the annotation is omitted (testing default behavior).
func (s *AffinityCanarySuite) deployAffinityCanary(cluster *Cluster, name, host, affinityCanaryBehavior string) {
	t := s.T()
	t.Helper()

	// Production ingress with session affinity.
	prodAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/affinity":            "cookie",
		"nginx.ingress.kubernetes.io/session-cookie-name": "route",
	}

	err := cluster.DeployIngressWith(ingressTemplateData{
		Name:        name + "-prod",
		Host:        host,
		Annotations: prodAnnotations,
	})
	require.NoError(t, err, "deploy prod ingress %s to %s", name, cluster.Name)

	cluster.WaitForIngressReady(t, host, 30, 1*time.Second)

	// Canary ingress.
	canaryAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/canary":        "true",
		"nginx.ingress.kubernetes.io/canary-weight": "50",
	}
	if affinityCanaryBehavior != "" {
		canaryAnnotations["nginx.ingress.kubernetes.io/affinity-canary-behavior"] = affinityCanaryBehavior
	}

	err = cluster.DeployIngressWith(ingressTemplateData{
		Name:        name,
		Host:        host,
		Annotations: canaryAnnotations,
		ServiceName: "canary-backend",
		ServicePort: 80,
	})
	require.NoError(t, err, "deploy canary ingress %s to %s", name, cluster.Name)
}

// deployScenario deploys the production + canary ingresses on both clusters.
func (s *AffinityCanarySuite) deployScenario(name, affinityCanaryBehavior string) {
	t := s.T()
	t.Helper()

	traefikHost := name + ".traefik.local"
	nginxHost := name + ".nginx.local"

	s.deployAffinityCanary(s.traefik, name, traefikHost, affinityCanaryBehavior)
	s.deployAffinityCanary(s.nginx, name, nginxHost, affinityCanaryBehavior)

	// Allow time for the canary to become active.
	time.Sleep(2 * time.Second)
}

// teardownScenario removes the ingresses for a scenario from both clusters.
func (s *AffinityCanarySuite) teardownScenario(name string) {
	_ = s.traefik.DeleteIngress(name)
	_ = s.nginx.DeleteIngress(name)
	_ = s.traefik.DeleteIngress(name + "-prod")
	_ = s.nginx.DeleteIngress(name + "-prod")
}

func (s *AffinityCanarySuite) TestAffinityCanaryStickyReturnsOK() {
	const scenario = "affinity-canary-sticky"
	s.deployScenario(scenario, "sticky")
	defer s.teardownScenario(scenario)

	traefikResp := s.traefik.MakeRequest(s.T(), scenario+".traefik.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik should return 200")

	nginxResp := s.nginx.MakeRequest(s.T(), scenario+".nginx.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx should return 200")

	// Should receive a Set-Cookie header with the affinity cookie.
	traefikCookie := findCookie(traefikResp.ResponseHeaders, "route")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "route")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set route cookie")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set route cookie")
}

func (s *AffinityCanarySuite) TestAffinityCanaryStickyPreservesRouting() {
	s.T().Skip("affinity-canary sticky routing not yet implemented")

	const scenario = "affinity-canary-preserve"
	s.deployScenario(scenario, "sticky")
	defer s.teardownScenario(scenario)

	// First request to get the affinity cookie.
	traefikResp1 := s.traefik.MakeRequest(s.T(), scenario+".traefik.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), traefikResp1, "traefik first response should not be nil")
	require.Equal(s.T(), http.StatusOK, traefikResp1.StatusCode, "traefik first request should return 200")

	traefikCookie := findCookie(traefikResp1.ResponseHeaders, "route")
	require.NotEmpty(s.T(), traefikCookie, "traefik should set route cookie on first request")

	traefikBackend1 := traefikResp1.RequestHeaders["Hostname"]
	require.NotEmpty(s.T(), traefikBackend1, "traefik first response should have a hostname")

	// Extract cookie value (name=value portion).
	traefikCookieValue := strings.SplitN(traefikCookie, ";", 2)[0]

	// Second request with cookie should reach the same backend.
	traefikResp2 := s.traefik.MakeRequest(s.T(), scenario+".traefik.local", http.MethodGet, "/",
		map[string]string{"Cookie": traefikCookieValue}, 10, 1*time.Second)
	require.NotNil(s.T(), traefikResp2, "traefik second response should not be nil")
	assert.Equal(s.T(), http.StatusOK, traefikResp2.StatusCode, "traefik second request should return 200")

	traefikBackend2 := traefikResp2.RequestHeaders["Hostname"]
	assert.Equal(s.T(), traefikBackend1, traefikBackend2, "traefik should route to the same backend with cookie")

	// Same test for nginx.
	nginxResp1 := s.nginx.MakeRequest(s.T(), scenario+".nginx.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp1, "nginx first response should not be nil")
	require.Equal(s.T(), http.StatusOK, nginxResp1.StatusCode, "nginx first request should return 200")

	nginxCookie := findCookie(nginxResp1.ResponseHeaders, "route")
	require.NotEmpty(s.T(), nginxCookie, "nginx should set route cookie on first request")

	nginxBackend1 := nginxResp1.RequestHeaders["Hostname"]
	require.NotEmpty(s.T(), nginxBackend1, "nginx first response should have a hostname")

	nginxCookieValue := strings.SplitN(nginxCookie, ";", 2)[0]

	nginxResp2 := s.nginx.MakeRequest(s.T(), scenario+".nginx.local", http.MethodGet, "/",
		map[string]string{"Cookie": nginxCookieValue}, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp2, "nginx second response should not be nil")
	assert.Equal(s.T(), http.StatusOK, nginxResp2.StatusCode, "nginx second request should return 200")

	nginxBackend2 := nginxResp2.RequestHeaders["Hostname"]
	assert.Equal(s.T(), nginxBackend1, nginxBackend2, "nginx should route to the same backend with cookie")
}

func (s *AffinityCanarySuite) TestAffinityCanaryLegacyReturnsOK() {
	const scenario = "affinity-canary-legacy"
	s.deployScenario(scenario, "legacy")
	defer s.teardownScenario(scenario)

	traefikResp := s.traefik.MakeRequest(s.T(), scenario+".traefik.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik should return 200")

	nginxResp := s.nginx.MakeRequest(s.T(), scenario+".nginx.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx should return 200")
}

func (s *AffinityCanarySuite) TestAffinityCanaryDefaultReturnsOK() {
	const scenario = "affinity-canary-default"
	// Empty string means no affinity-canary-behavior annotation; should behave like sticky.
	s.deployScenario(scenario, "")
	defer s.teardownScenario(scenario)

	traefikResp := s.traefik.MakeRequest(s.T(), scenario+".traefik.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik should return 200")

	nginxResp := s.nginx.MakeRequest(s.T(), scenario+".nginx.local", http.MethodGet, "/", nil, 10, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx should return 200")

	// Should behave like sticky: Set-Cookie header should be present.
	traefikCookie := findCookie(traefikResp.ResponseHeaders, "route")
	nginxCookie := findCookie(nginxResp.ResponseHeaders, "route")

	assert.NotEmpty(s.T(), traefikCookie, "traefik should set route cookie (default sticky behavior)")
	assert.NotEmpty(s.T(), nginxCookie, "nginx should set route cookie (default sticky behavior)")
}
