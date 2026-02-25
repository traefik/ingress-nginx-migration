package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	customHeadersIngressName    = "custom-headers-test"
	customHeadersTraefikHost    = customHeadersIngressName + ".traefik.local"
	customHeadersNginxHost      = customHeadersIngressName + ".nginx.local"
	customHeadersConfigMapName  = "custom-headers"
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

	s.traefik.WaitForIngressReady(s.T(), customHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), customHeadersNginxHost, 20, 1*time.Second)
}

func (s *CustomHeadersSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(customHeadersIngressName)
	_ = s.nginx.DeleteIngress(customHeadersIngressName)
	_ = s.traefik.DeleteConfigMap(customHeadersConfigMapName)
	_ = s.nginx.DeleteConfigMap(customHeadersConfigMapName)

	// Remove the add-headers key from the nginx controller ConfigMap.
	_ = s.nginx.Kubectl("patch", "configmap", "ingress-nginx-controller",
		"-n", s.nginx.ControllerNS,
		"--type=json",
		"-p", `[{"op":"remove","path":"/data/add-headers"}]`,
	)
}

func (s *CustomHeadersSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), customHeadersTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), customHeadersNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// Custom response headers — verified via HTTP response headers.

func (s *CustomHeadersSuite) TestCustomResponseHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Custom-Resp"),
		traefikResp.ResponseHeaders.Get("X-Custom-Resp"),
		"custom response header mismatch",
	)
}

func (s *CustomHeadersSuite) TestSecurityResponseHeader() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Frame-Options"),
		traefikResp.ResponseHeaders.Get("X-Frame-Options"),
		"X-Frame-Options mismatch",
	)
}

func (s *CustomHeadersSuite) TestMoreSetResponseHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-More-Resp"),
		traefikResp.ResponseHeaders.Get("X-More-Resp"),
		"more_set_headers response mismatch",
	)
}

// Client-originated headers.

func (s *CustomHeadersSuite) TestClientHeaderPassthrough() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", map[string]string{
		"X-Client-Custom": "from-client",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Client-Custom"],
		traefikResp.RequestHeaders["X-Client-Custom"],
		"client header passthrough mismatch",
	)
}

// Combined verification.

func (s *CustomHeadersSuite) TestAllResponseHeaders() {
	traefikResp, nginxResp := s.request(http.MethodGet, "/", nil)

	for _, header := range []string{"X-Custom-Resp", "X-Frame-Options", "X-More-Resp"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
	}
}
