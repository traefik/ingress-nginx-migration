package e2e

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// Ingress with all three timeout annotations set to reasonable values.
	timeoutAllIngressName = "timeout-all-test"
	timeoutAllTraefikHost = timeoutAllIngressName + ".traefik.local"
	timeoutAllNginxHost   = timeoutAllIngressName + ".nginx.local"

	// Ingress with a very short read timeout (1 second) to trigger 504 on slow backends.
	timeoutShortReadIngressName = "timeout-short-read-test"
	timeoutShortReadTraefikHost = timeoutShortReadIngressName + ".traefik.local"
	timeoutShortReadNginxHost   = timeoutShortReadIngressName + ".nginx.local"

	// Ingress with only proxy-connect-timeout set.
	timeoutConnectOnlyIngressName = "timeout-connect-only-test"
	timeoutConnectOnlyTraefikHost = timeoutConnectOnlyIngressName + ".traefik.local"
	timeoutConnectOnlyNginxHost   = timeoutConnectOnlyIngressName + ".nginx.local"

	// Ingress with only proxy-send-timeout set.
	timeoutSendOnlyIngressName = "timeout-send-only-test"
	timeoutSendOnlyTraefikHost = timeoutSendOnlyIngressName + ".traefik.local"
	timeoutSendOnlyNginxHost   = timeoutSendOnlyIngressName + ".nginx.local"
)

type ProxyTimeoutSuite struct {
	BaseSuite
}

func TestProxyTimeoutSuite(t *testing.T) {
	suite.Run(t, new(ProxyTimeoutSuite))
}

func (s *ProxyTimeoutSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Ingress with all three proxy timeout annotations.
	allAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-connect-timeout": "30",
		"nginx.ingress.kubernetes.io/proxy-read-timeout":    "30",
		"nginx.ingress.kubernetes.io/proxy-send-timeout":    "30",
	}

	err := s.traefik.DeployIngress(timeoutAllIngressName, timeoutAllTraefikHost, allAnnotations)
	require.NoError(s.T(), err, "deploy timeout-all ingress to traefik cluster")

	err = s.nginx.DeployIngress(timeoutAllIngressName, timeoutAllNginxHost, allAnnotations)
	require.NoError(s.T(), err, "deploy timeout-all ingress to nginx cluster")

	// Ingress with a very short read timeout to test 504 behavior.
	shortReadAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-connect-timeout": "5",
		"nginx.ingress.kubernetes.io/proxy-read-timeout":    "1",
		"nginx.ingress.kubernetes.io/proxy-send-timeout":    "5",
	}

	err = s.traefik.DeployIngress(timeoutShortReadIngressName, timeoutShortReadTraefikHost, shortReadAnnotations)
	require.NoError(s.T(), err, "deploy timeout-short-read ingress to traefik cluster")

	err = s.nginx.DeployIngress(timeoutShortReadIngressName, timeoutShortReadNginxHost, shortReadAnnotations)
	require.NoError(s.T(), err, "deploy timeout-short-read ingress to nginx cluster")

	// Ingress with only proxy-connect-timeout set.
	connectOnlyAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-connect-timeout": "10",
	}

	err = s.traefik.DeployIngress(timeoutConnectOnlyIngressName, timeoutConnectOnlyTraefikHost, connectOnlyAnnotations)
	require.NoError(s.T(), err, "deploy timeout-connect-only ingress to traefik cluster")

	err = s.nginx.DeployIngress(timeoutConnectOnlyIngressName, timeoutConnectOnlyNginxHost, connectOnlyAnnotations)
	require.NoError(s.T(), err, "deploy timeout-connect-only ingress to nginx cluster")

	// Ingress with only proxy-send-timeout set.
	sendOnlyAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-send-timeout": "10",
	}

	err = s.traefik.DeployIngress(timeoutSendOnlyIngressName, timeoutSendOnlyTraefikHost, sendOnlyAnnotations)
	require.NoError(s.T(), err, "deploy timeout-send-only ingress to traefik cluster")

	err = s.nginx.DeployIngress(timeoutSendOnlyIngressName, timeoutSendOnlyNginxHost, sendOnlyAnnotations)
	require.NoError(s.T(), err, "deploy timeout-send-only ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), timeoutAllTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), timeoutAllNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), timeoutShortReadTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), timeoutShortReadNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), timeoutConnectOnlyTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), timeoutConnectOnlyNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), timeoutSendOnlyTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), timeoutSendOnlyNginxHost, 20, 1*time.Second)
}

func (s *ProxyTimeoutSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(timeoutAllIngressName)
	_ = s.nginx.DeleteIngress(timeoutAllIngressName)
	_ = s.traefik.DeleteIngress(timeoutShortReadIngressName)
	_ = s.nginx.DeleteIngress(timeoutShortReadIngressName)
	_ = s.traefik.DeleteIngress(timeoutConnectOnlyIngressName)
	_ = s.nginx.DeleteIngress(timeoutConnectOnlyIngressName)
	_ = s.traefik.DeleteIngress(timeoutSendOnlyIngressName)
	_ = s.nginx.DeleteIngress(timeoutSendOnlyIngressName)
}

// makeRequestWithClientTimeout makes an HTTP request with a custom client timeout.
// This is needed for timeout tests where the proxy may take longer than the default
// 5-second client timeout to respond (e.g., when waiting for a 504 from a slow backend).
func (s *ProxyTimeoutSuite) makeRequestWithClientTimeout(c *Cluster, host, method, path string, clientTimeout time.Duration) *Response {
	s.T().Helper()

	url := fmt.Sprintf("http://%s:%s%s", c.Host, c.Port, path)
	client := &http.Client{
		Timeout:       clientTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	req, err := http.NewRequest(method, url, nil)
	require.NoError(s.T(), err)
	req.Host = host

	resp, err := client.Do(req)
	if err != nil {
		s.T().Logf("[%s] request error: %v", c.Name, err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.T().Logf("[%s] body read error: %v", c.Name, err)
		return nil
	}

	return &Response{
		StatusCode:      resp.StatusCode,
		Body:            string(body),
		ResponseHeaders: resp.Header,
		RequestHeaders:  parseWhoamiHeaders(string(body)),
	}
}

// requestAll makes the same HTTP request against both clusters using the all-timeouts ingress.
func (s *ProxyTimeoutSuite) requestAll(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), timeoutAllTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), timeoutAllNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestConnectOnly makes the same HTTP request against both clusters using the connect-only ingress.
func (s *ProxyTimeoutSuite) requestConnectOnly(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), timeoutConnectOnlyTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), timeoutConnectOnlyNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// requestSendOnly makes the same HTTP request against both clusters using the send-only ingress.
func (s *ProxyTimeoutSuite) requestSendOnly(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), timeoutSendOnlyTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), timeoutSendOnlyNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *ProxyTimeoutSuite) TestAllTimeoutsNormalRequest() {
	traefikResp, nginxResp := s.requestAll(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for normal request with all timeouts set")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for normal request with all timeouts set")
}

func (s *ProxyTimeoutSuite) TestAllTimeoutsWithSubpath() {
	traefikResp, nginxResp := s.requestAll(http.MethodGet, "/some/path", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for subpath")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for subpath request with all timeouts set")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for subpath request with all timeouts set")
}

func (s *ProxyTimeoutSuite) TestShortReadTimeoutFastRequest() {
	traefikResp := s.makeRequestWithClientTimeout(s.traefik, timeoutShortReadTraefikHost, http.MethodGet, "/", 10*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil for fast request")

	nginxResp := s.makeRequestWithClientTimeout(s.nginx, timeoutShortReadNginxHost, http.MethodGet, "/", 10*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil for fast request")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for fast request with short read timeout")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for fast request with short read timeout")
}

func (s *ProxyTimeoutSuite) TestShortReadTimeoutSlowBackend() {
	traefikResp := s.makeRequestWithClientTimeout(s.traefik, timeoutShortReadTraefikHost, http.MethodGet, "/?wait=5s", 15*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.makeRequestWithClientTimeout(s.nginx, timeoutShortReadNginxHost, http.MethodGet, "/?wait=5s", 15*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusGatewayTimeout, traefikResp.StatusCode,
		"traefik should return 504 when backend exceeds read timeout")
	assert.Equal(s.T(), http.StatusGatewayTimeout, nginxResp.StatusCode,
		"nginx should return 504 when backend exceeds read timeout")
}

func (s *ProxyTimeoutSuite) TestAllTimeoutsSlowBackendWithinLimit() {
	traefikResp := s.makeRequestWithClientTimeout(s.traefik, timeoutAllTraefikHost, http.MethodGet, "/?wait=2s", 15*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.makeRequestWithClientTimeout(s.nginx, timeoutAllNginxHost, http.MethodGet, "/?wait=2s", 15*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"traefik should return 200 when backend responds within timeout")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"nginx should return 200 when backend responds within timeout")
}

func (s *ProxyTimeoutSuite) TestConnectOnlyNormalRequest() {
	traefikResp, nginxResp := s.requestConnectOnly(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with connect-only timeout")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 with connect-only timeout")
}

func (s *ProxyTimeoutSuite) TestSendOnlyNormalRequest() {
	traefikResp, nginxResp := s.requestSendOnly(http.MethodGet, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with send-only timeout")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 with send-only timeout")
}

func (s *ProxyTimeoutSuite) TestAllTimeoutsPostRequest() {
	traefikResp, nginxResp := s.requestAll(http.MethodPost, "/", nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for POST")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for POST with all timeouts set")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for POST with all timeouts set")
}

func (s *ProxyTimeoutSuite) TestConnectOnlySlowBackendWithinDefaultReadTimeout() {
	traefikResp := s.makeRequestWithClientTimeout(s.traefik, timeoutConnectOnlyTraefikHost, http.MethodGet, "/?wait=2s", 15*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.makeRequestWithClientTimeout(s.nginx, timeoutConnectOnlyNginxHost, http.MethodGet, "/?wait=2s", 15*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"traefik should return 200 when backend is within default read timeout")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode,
		"nginx should return 200 when backend is within default read timeout")
}
