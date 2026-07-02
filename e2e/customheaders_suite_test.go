package e2e

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	customHeadersIngressName   = "custom-headers-test"
	customHeadersTraefikHost   = customHeadersIngressName + ".traefik.local"
	customHeadersNginxHost     = customHeadersIngressName + ".nginx.local"
	customHeadersGatewayHost   = customHeadersIngressName + ".gateway.local"
	customHeadersConfigMapName = "custom-headers"
)

type CustomHeadersSuite struct {
	BaseSuite
}

func TestCustomHeadersSuite(t *testing.T) {
	suite.Run(t, new(CustomHeadersSuite))
}

func (s *CustomHeadersSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Create the ConfigMap with custom response headers on both clusters.
	cmData := configMapTemplateData{
		Name: customHeadersConfigMapName,
		Data: map[string]string{
			"X-Custom-Resp":   "custom-response-value",
			"X-Frame-Options": "DENY",
			"X-More-Resp":     "more-response-value",
		},
	}

	err := s.traefik.DeployConfigMap(cmData)
	require.NoError(s.T(), err, "create custom-headers configmap in traefik cluster")

	err = s.nginx.DeployConfigMap(cmData)
	require.NoError(s.T(), err, "create custom-headers configmap in nginx cluster")

	// Traefik: uses the per-ingress annotation to reference the ConfigMap.
	traefikAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/custom-headers": fmt.Sprintf("%s/%s", s.traefik.TestNamespace, customHeadersConfigMapName),
	}

	err = s.traefik.DeployIngress(customHeadersIngressName, customHeadersTraefikHost, traefikAnnotations)
	require.NoError(s.T(), err, "deploy custom-headers ingress to traefik cluster")

	// nginx: uses the controller's ConfigMap "add-headers" key to reference
	// the custom headers ConfigMap (response headers are global, not per-ingress).
	err = s.nginx.Kubectl("patch", "configmap", "ingress-nginx-controller",
		"-n", s.nginx.ControllerNS,
		"--type=merge",
		"-p", fmt.Sprintf(`{"data":{"add-headers":"%s/%s"}}`, s.nginx.TestNamespace, customHeadersConfigMapName),
	)
	require.NoError(s.T(), err, "patch nginx controller configmap with add-headers")

	err = s.nginx.DeployIngress(customHeadersIngressName, customHeadersNginxHost, nil)
	require.NoError(s.T(), err, "deploy custom-headers ingress to nginx cluster")

	// Deploy Gateway API equivalent (custom-headers annotation → ResponseHeaderModifier filter).
	err = s.gateway.DeployGatewayFixture(filepath.Join(fixturesDir, "gateway", "customheaders", "headers.yaml"))
	require.NoError(s.T(), err, "deploy custom-headers gateway fixture")

	s.traefik.WaitForIngressReady(s.T(), customHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), customHeadersNginxHost, 20, 1*time.Second)
	// Gateway API routes need more time — CRD provider must publish middleware config first.
	s.gateway.WaitForIngressReady(s.T(), customHeadersGatewayHost, 60, 1*time.Second)
}

func (s *CustomHeadersSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(customHeadersIngressName)
	_ = s.nginx.DeleteIngress(customHeadersIngressName)
	_ = s.traefik.DeleteConfigMap(customHeadersConfigMapName)
	_ = s.nginx.DeleteConfigMap(customHeadersConfigMapName)
	_ = s.gateway.DeleteGatewayFixture(filepath.Join(fixturesDir, "gateway", "customheaders", "headers.yaml"))

	// Remove the add-headers key from the nginx controller ConfigMap.
	_ = s.nginx.Kubectl("patch", "configmap", "ingress-nginx-controller",
		"-n", s.nginx.ControllerNS,
		"--type=json",
		"-p", `[{"op":"remove","path":"/data/add-headers"}]`,
	)
}

func (s *CustomHeadersSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp, gatewayResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), customHeadersTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), customHeadersNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	gatewayResp = s.gateway.MakeRequest(s.T(), customHeadersGatewayHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), gatewayResp, "gateway response should not be nil")

	return traefikResp, nginxResp, gatewayResp
}

func (s *CustomHeadersSuite) TestCustomHeaders() {
	testCases := []struct {
		desc        string
		method      string
		path        string
		headers     map[string]string
		check       func(t *testing.T, traefikResp, nginxResp *Response)
		checkGW     func(t *testing.T, traefikResp, gatewayResp *Response)
	}{
		{
			desc:   "X-Custom-Resp header",
			method: http.MethodGet,
			path:   "/",
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
				assert.Equal(t,
					nginxResp.ResponseHeaders.Get("X-Custom-Resp"),
					traefikResp.ResponseHeaders.Get("X-Custom-Resp"),
					"X-Custom-Resp mismatch",
				)
			},
			checkGW: func(t *testing.T, traefikResp, gatewayResp *Response) {
				assert.Equal(t, "custom-response-value", gatewayResp.ResponseHeaders.Get("X-Custom-Resp"),
					"gateway should set X-Custom-Resp response header")
			},
		},
		{
			desc:   "X-Frame-Options header",
			method: http.MethodGet,
			path:   "/",
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t,
					nginxResp.ResponseHeaders.Get("X-Frame-Options"),
					traefikResp.ResponseHeaders.Get("X-Frame-Options"),
					"X-Frame-Options mismatch",
				)
			},
			checkGW: func(t *testing.T, traefikResp, gatewayResp *Response) {
				assert.Equal(t, "DENY", gatewayResp.ResponseHeaders.Get("X-Frame-Options"),
					"gateway should set X-Frame-Options response header")
			},
		},
		{
			desc:   "X-More-Resp header",
			method: http.MethodGet,
			path:   "/",
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t,
					nginxResp.ResponseHeaders.Get("X-More-Resp"),
					traefikResp.ResponseHeaders.Get("X-More-Resp"),
					"X-More-Resp mismatch",
				)
			},
			checkGW: func(t *testing.T, traefikResp, gatewayResp *Response) {
				assert.Equal(t, "more-response-value", gatewayResp.ResponseHeaders.Get("X-More-Resp"),
					"gateway should set X-More-Resp response header")
			},
		},
		{
			desc:    "client header passthrough",
			method:  http.MethodGet,
			path:    "/",
			headers: map[string]string{"X-Client-Custom": "from-client"},
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
				assert.Equal(t,
					nginxResp.RequestHeaders["X-Client-Custom"],
					traefikResp.RequestHeaders["X-Client-Custom"],
					"client header passthrough mismatch",
				)
			},
			checkGW: func(t *testing.T, traefikResp, gatewayResp *Response) {
				assert.Equal(t, traefikResp.StatusCode, gatewayResp.StatusCode, "gateway migration: status code mismatch")
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			traefikResp, nginxResp, gatewayResp := s.request(tc.method, tc.path, tc.headers)
			tc.check(t, traefikResp, nginxResp)
			if tc.checkGW != nil {
				tc.checkGW(t, traefikResp, gatewayResp)
			}
		})
	}
}

func (s *CustomHeadersSuite) TestWrongConfigMap() {
	wrongCMIngressName := "custom-headers-wrong-cm"
	wrongCMHost := wrongCMIngressName + ".traefik.local"
	wrongCMNginxHost := wrongCMIngressName + ".nginx.local"

	wrongCMAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/custom-headers": fmt.Sprintf("%s/%s", s.traefik.TestNamespace, "non-existent-configmap"),
	}

	err := s.traefik.DeployIngress(wrongCMIngressName, wrongCMHost, wrongCMAnnotations)
	require.NoError(s.T(), err, "deploy wrong-cm ingress to traefik cluster")

	err = s.nginx.DeployIngress(wrongCMIngressName, wrongCMNginxHost, wrongCMAnnotations)
	require.NoError(s.T(), err, "deploy wrong-cm ingress to nginx cluster")

	s.T().Cleanup(func() {
		_ = s.traefik.DeleteIngress(wrongCMIngressName)
		_ = s.nginx.DeleteIngress(wrongCMIngressName)
	})

	s.traefik.WaitForIngressReady(s.T(), wrongCMHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), wrongCMNginxHost, 20, 1*time.Second)

	traefikResp := s.traefik.MakeRequest(s.T(), wrongCMHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), wrongCMNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch with wrong ConfigMap")
}
