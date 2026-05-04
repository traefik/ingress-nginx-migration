package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// UnsupportedAnnotationsSuite validates the runtime behavior of annotations
// the migration tool flags as either known-unsupported or unknown.
//
//   - denylist-source-range is a known-unsupported annotation: NGINX enforces it,
//     Traefik ignores it, and the migration tool surfaces the gap.
//   - A made-up annotation is unknown to both controllers and is ignored.
type UnsupportedAnnotationsSuite struct {
	BaseSuite
}

func TestUnsupportedAnnotationsSuite(t *testing.T) {
	suite.Run(t, new(UnsupportedAnnotationsSuite))
}

const (
	// denylistIngressName uses denylist-source-range: 0.0.0.0/0 (deny all IPs).
	// NGINX returns 403 for every request; Traefik ignores the annotation and returns 200.
	denylistIngressName = "denylist-all-test"
	denylistTraefikHost = denylistIngressName + ".traefik.local"
	denylistNginxHost   = denylistIngressName + ".nginx.local"

	// unknownAnnotationIngressName carries an annotation not present in any known list.
	// Both controllers ignore it and route normally (200).
	unknownAnnotationIngressName = "unknown-annotation-test"
	unknownAnnotationTraefikHost = unknownAnnotationIngressName + ".traefik.local"
	unknownAnnotationNginxHost   = unknownAnnotationIngressName + ".nginx.local"
)

func (s *UnsupportedAnnotationsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	denyAllAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/denylist-source-range": "0.0.0.0/0",
	}

	err := s.traefik.DeployIngress(denylistIngressName, denylistTraefikHost, denyAllAnnotations)
	require.NoError(s.T(), err, "deploy denylist-all ingress to traefik cluster")

	err = s.nginx.DeployIngress(denylistIngressName, denylistNginxHost, denyAllAnnotations)
	require.NoError(s.T(), err, "deploy denylist-all ingress to nginx cluster")

	unknownAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/does-not-exist-in-any-list": "true",
	}

	err = s.traefik.DeployIngress(unknownAnnotationIngressName, unknownAnnotationTraefikHost, unknownAnnotations)
	require.NoError(s.T(), err, "deploy unknown-annotation ingress to traefik cluster")

	err = s.nginx.DeployIngress(unknownAnnotationIngressName, unknownAnnotationNginxHost, unknownAnnotations)
	require.NoError(s.T(), err, "deploy unknown-annotation ingress to nginx cluster")

	// The denylist ingress returns 403 on NGINX, which is not a 404/502,
	// so WaitForIngressReady will consider it ready.
	s.traefik.WaitForIngressReady(s.T(), denylistTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), denylistNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), unknownAnnotationTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), unknownAnnotationNginxHost, 20, 1*time.Second)
}

func (s *UnsupportedAnnotationsSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(denylistIngressName)
	_ = s.nginx.DeleteIngress(denylistIngressName)
	_ = s.traefik.DeleteIngress(unknownAnnotationIngressName)
	_ = s.nginx.DeleteIngress(unknownAnnotationIngressName)
}

// TestKnownUnsupportedAnnotation_DenylistBehaviorDiffers verifies that Traefik
// ignores the denylist-source-range annotation and routes traffic normally,
// while NGINX enforces the deny rule and returns 403 — demonstrating the
// behavioral gap users must close manually (e.g. via Traefik's IPAllowList
// middleware).
func (s *UnsupportedAnnotationsSuite) TestKnownUnsupportedAnnotation_DenylistBehaviorDiffers() {
	traefikResp := s.traefik.MakeRequest(s.T(), denylistTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), denylistNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"traefik should ignore denylist-source-range and return 200")
	assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode,
		"nginx should enforce denylist-source-range: 0.0.0.0/0 and return 403")
}

// TestUnknownAnnotation_BothControllersIgnore verifies that an annotation
// not present in any known list is silently ignored by both controllers
// and traffic is routed normally.
func (s *UnsupportedAnnotationsSuite) TestUnknownAnnotation_BothControllersIgnore() {
	traefikResp := s.traefik.MakeRequest(s.T(), unknownAnnotationTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), unknownAnnotationNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"traefik should ignore unknown annotation and route normally")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"nginx should ignore unknown annotation and route normally")
}
