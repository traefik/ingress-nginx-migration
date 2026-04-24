package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DenylistSuite validates the behavior of the denylist-source-range annotation,
// which is a known-unsupported annotation in Traefik v3.7.
//
// NGINX applies the annotation and denies matching source IPs.
// Traefik ignores it and routes all traffic normally.
// This behavioral difference is expected and documented by the migration tool.
type DenylistSuite struct {
	BaseSuite
}

func TestDenylistSuite(t *testing.T) {
	suite.Run(t, new(DenylistSuite))
}

const (
	// denyAllIngressName uses denylist-source-range: 0.0.0.0/0 (deny all IPs).
	// NGINX returns 403 for every request; Traefik ignores the annotation and returns 200.
	denyAllIngressName    = "denylist-all-test"
	denyAllTraefikHost    = denyAllIngressName + ".traefik.local"
	denyAllNginxHost      = denyAllIngressName + ".nginx.local"

	// unknownAnnotationIngressName carries an annotation not present in any known list.
	// Both controllers ignore it and route normally (200).
	unknownAnnotationIngressName = "unknown-annotation-test"
	unknownAnnotationTraefikHost = unknownAnnotationIngressName + ".traefik.local"
	unknownAnnotationNginxHost   = unknownAnnotationIngressName + ".nginx.local"
)

func (s *DenylistSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress with denylist-source-range covering all IPs.
	denyAllAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/denylist-source-range": "0.0.0.0/0",
	}

	err := s.traefik.DeployIngress(denyAllIngressName, denyAllTraefikHost, denyAllAnnotations)
	require.NoError(s.T(), err, "deploy denylist-all ingress to traefik cluster")

	err = s.nginx.DeployIngress(denyAllIngressName, denyAllNginxHost, denyAllAnnotations)
	require.NoError(s.T(), err, "deploy denylist-all ingress to nginx cluster")

	// Ingress with a completely unknown annotation (not in supported or known-unsupported lists).
	unknownAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/does-not-exist-in-any-list": "true",
	}

	err = s.traefik.DeployIngress(unknownAnnotationIngressName, unknownAnnotationTraefikHost, unknownAnnotations)
	require.NoError(s.T(), err, "deploy unknown-annotation ingress to traefik cluster")

	err = s.nginx.DeployIngress(unknownAnnotationIngressName, unknownAnnotationNginxHost, unknownAnnotations)
	require.NoError(s.T(), err, "deploy unknown-annotation ingress to nginx cluster")

	// The denylist ingress returns 403 on NGINX, which is not a 404/502,
	// so WaitForIngressReady will consider it ready.
	s.traefik.WaitForIngressReady(s.T(), denyAllTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), denyAllNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), unknownAnnotationTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), unknownAnnotationNginxHost, 20, 1*time.Second)
}

func (s *DenylistSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(denyAllIngressName)
	_ = s.nginx.DeleteIngress(denyAllIngressName)
	_ = s.traefik.DeleteIngress(unknownAnnotationIngressName)
	_ = s.nginx.DeleteIngress(unknownAnnotationIngressName)
}

// TestKnownUnsupportedAnnotation_DenylistTraefikIgnores verifies that Traefik
// ignores the denylist-source-range annotation and routes traffic normally,
// while NGINX enforces the deny rule and returns 403.
//
// This demonstrates the behavioral gap for a known-unsupported annotation:
// users who rely on NGINX's deny behavior must migrate to Traefik's
// IPAllowList middleware manually.
func (s *DenylistSuite) TestKnownUnsupportedAnnotation_DenylistTraefikIgnores() {
	traefikResp := s.traefik.MakeRequest(s.T(), denyAllTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), denyAllNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	// Traefik ignores the annotation: traffic is allowed.
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"traefik should ignore denylist-source-range and return 200")

	// NGINX enforces the annotation: all IPs are denied.
	assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode,
		"nginx should enforce denylist-source-range: 0.0.0.0/0 and return 403")
}

// TestKnownUnsupportedAnnotation_DenylistBehaviorDiffers documents the
// behavioral difference between the two controllers for denylist-source-range.
func (s *DenylistSuite) TestKnownUnsupportedAnnotation_DenylistBehaviorDiffers() {
	traefikResp := s.traefik.MakeRequest(s.T(), denyAllTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp)

	nginxResp := s.nginx.MakeRequest(s.T(), denyAllNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	assert.NotEqual(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"status codes should differ: traefik ignores the annotation, nginx enforces it")
}

// TestUnknownAnnotation_BothControllersIgnore verifies that an annotation
// not present in any known list is silently ignored by both controllers,
// and traffic is routed normally.
//
// This shows that unknown annotations do not break routing in either controller.
func (s *DenylistSuite) TestUnknownAnnotation_BothControllersIgnore() {
	traefikResp := s.traefik.MakeRequest(s.T(), unknownAnnotationTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), unknownAnnotationNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"traefik should ignore unknown annotation and route normally")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"nginx should ignore unknown annotation and route normally")
}

// TestUnknownAnnotation_StatusCodesMatch verifies that both controllers
// produce the same response when an unknown annotation is present.
func (s *DenylistSuite) TestUnknownAnnotation_StatusCodesMatch() {
	traefikResp := s.traefik.MakeRequest(s.T(), unknownAnnotationTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp)

	nginxResp := s.nginx.MakeRequest(s.T(), unknownAnnotationNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"both controllers should produce the same status code for an unknown annotation")
}
