package e2e

import (
	"bytes"
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
	bodyLimitIngressName   = "body-limit-test"
	bodyNoLimitIngressName = "body-no-limit-test"
	bodyLimitTraefikHost   = bodyLimitIngressName + ".traefik.local"
	bodyLimitNginxHost     = bodyLimitIngressName + ".nginx.local"
	bodyNoLimitTraefikHost = bodyNoLimitIngressName + ".traefik.local"
	bodyNoLimitNginxHost   = bodyNoLimitIngressName + ".nginx.local"

	bufferingOffIngressName = "buffering-off-test"
	bufferingOffTraefikHost = bufferingOffIngressName + ".traefik.local"
	bufferingOffNginxHost   = bufferingOffIngressName + ".nginx.local"

	bodyLimitMBIngressName = "body-limit-mb-test"
	bodyLimitMBTraefikHost = bodyLimitMBIngressName + ".traefik.local"
	bodyLimitMBNginxHost   = bodyLimitMBIngressName + ".nginx.local"

	bufferConfigIngressName = "buffer-config-test"
	bufferConfigTraefikHost = bufferConfigIngressName + ".traefik.local"
	bufferConfigNginxHost   = bufferConfigIngressName + ".nginx.local"
)

type ProxyBodySizeSuite struct {
	BaseSuite
}

func TestProxyBodySizeSuite(t *testing.T) {
	suite.Run(t, new(ProxyBodySizeSuite))
}

func (s *ProxyBodySizeSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	limitAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "1k",
	}

	noLimitAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "0",
	}

	// Deploy limited ingress to both clusters.
	err := s.traefik.DeployIngress(bodyLimitIngressName, bodyLimitTraefikHost, limitAnnotations)
	require.NoError(s.T(), err, "deploy body-limit ingress to traefik cluster")

	err = s.nginx.DeployIngress(bodyLimitIngressName, bodyLimitNginxHost, limitAnnotations)
	require.NoError(s.T(), err, "deploy body-limit ingress to nginx cluster")

	// Deploy unlimited ingress to both clusters.
	err = s.traefik.DeployIngress(bodyNoLimitIngressName, bodyNoLimitTraefikHost, noLimitAnnotations)
	require.NoError(s.T(), err, "deploy body-no-limit ingress to traefik cluster")

	err = s.nginx.DeployIngress(bodyNoLimitIngressName, bodyNoLimitNginxHost, noLimitAnnotations)
	require.NoError(s.T(), err, "deploy body-no-limit ingress to nginx cluster")

	// Deploy buffering-off ingress to both clusters.
	bufferingAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-buffering":         "off",
		"nginx.ingress.kubernetes.io/proxy-request-buffering": "off",
	}

	err = s.traefik.DeployIngress(bufferingOffIngressName, bufferingOffTraefikHost, bufferingAnnotations)
	require.NoError(s.T(), err, "deploy buffering-off ingress to traefik cluster")

	err = s.nginx.DeployIngress(bufferingOffIngressName, bufferingOffNginxHost, bufferingAnnotations)
	require.NoError(s.T(), err, "deploy buffering-off ingress to nginx cluster")

	// Deploy body-limit-mb ingress to both clusters.
	limitMBAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-body-size": "1m",
	}

	err = s.traefik.DeployIngress(bodyLimitMBIngressName, bodyLimitMBTraefikHost, limitMBAnnotations)
	require.NoError(s.T(), err, "deploy body-limit-mb ingress to traefik cluster")

	err = s.nginx.DeployIngress(bodyLimitMBIngressName, bodyLimitMBNginxHost, limitMBAnnotations)
	require.NoError(s.T(), err, "deploy body-limit-mb ingress to nginx cluster")

	// Deploy buffer config ingress with all buffering annotations.
	bufferConfigAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/client-body-buffer-size":  "16k",
		"nginx.ingress.kubernetes.io/proxy-buffer-size":        "8k",
		"nginx.ingress.kubernetes.io/proxy-buffers-number":     "8",
		"nginx.ingress.kubernetes.io/proxy-max-temp-file-size": "1024m",
	}

	err = s.traefik.DeployIngress(bufferConfigIngressName, bufferConfigTraefikHost, bufferConfigAnnotations)
	require.NoError(s.T(), err, "deploy buffer-config ingress to traefik cluster")

	err = s.nginx.DeployIngress(bufferConfigIngressName, bufferConfigNginxHost, bufferConfigAnnotations)
	require.NoError(s.T(), err, "deploy buffer-config ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), bodyLimitTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bodyLimitNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), bodyNoLimitTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bodyNoLimitNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), bufferingOffTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bufferingOffNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), bodyLimitMBTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bodyLimitMBNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), bufferConfigTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bufferConfigNginxHost, 20, 1*time.Second)
}

func (s *ProxyBodySizeSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(bodyLimitIngressName)
	_ = s.nginx.DeleteIngress(bodyLimitIngressName)
	_ = s.traefik.DeleteIngress(bodyNoLimitIngressName)
	_ = s.nginx.DeleteIngress(bodyNoLimitIngressName)
	_ = s.traefik.DeleteIngress(bufferingOffIngressName)
	_ = s.nginx.DeleteIngress(bufferingOffIngressName)
	_ = s.traefik.DeleteIngress(bodyLimitMBIngressName)
	_ = s.nginx.DeleteIngress(bodyLimitMBIngressName)
	_ = s.traefik.DeleteIngress(bufferConfigIngressName)
	_ = s.nginx.DeleteIngress(bufferConfigIngressName)
}

// makeRequestWithBody makes an HTTP request with a body to the given cluster and returns the response.
func (s *ProxyBodySizeSuite) makeRequestWithBody(c *Cluster, host, method, path string, body []byte) *Response {
	s.T().Helper()

	url := fmt.Sprintf("http://%s:%s%s", c.Host, c.Port, path)
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	require.NoError(s.T(), err)

	req.Host = host
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	require.NoError(s.T(), err)
	defer resp.Body.Close()

	// Tolerate read errors (e.g. unexpected EOF) when the server rejects
	// a large body — it may close the connection after sending the status.
	respBody, _ := io.ReadAll(resp.Body)

	return &Response{
		StatusCode:      resp.StatusCode,
		Body:            string(respBody),
		ResponseHeaders: resp.Header,
	}
}

func (s *ProxyBodySizeSuite) TestSmallBodyWithinLimit() {
	body := bytes.Repeat([]byte("A"), 500)

	traefikResp := s.makeRequestWithBody(s.traefik, bodyLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bodyLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for body within limit")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for body within limit")
}

func (s *ProxyBodySizeSuite) TestLargeBodyExceedsLimit() {
	body := bytes.Repeat([]byte("A"), 2*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bodyLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bodyLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	// nginx returns 413 for bodies exceeding the limit.
	assert.Equal(s.T(), http.StatusRequestEntityTooLarge, nginxResp.StatusCode, "nginx should return 413 for body exceeding limit")
}

func (s *ProxyBodySizeSuite) TestLargeBodyUnlimited() {
	body := bytes.Repeat([]byte("A"), 2*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bodyNoLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bodyNoLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for unlimited body size")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for unlimited body size")
}

func (s *ProxyBodySizeSuite) TestVeryLargeBodyUnlimited() {
	body := bytes.Repeat([]byte("A"), 100*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bodyNoLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bodyNoLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for unlimited body size")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for unlimited body size")
}

func (s *ProxyBodySizeSuite) TestBufferingOffSmallBody() {
	body := bytes.Repeat([]byte("B"), 500)

	traefikResp := s.makeRequestWithBody(s.traefik, bufferingOffTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bufferingOffNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for small body with buffering off")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for small body with buffering off")
}

func (s *ProxyBodySizeSuite) TestBufferingOffLargeBody() {
	body := bytes.Repeat([]byte("B"), 100*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bufferingOffTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bufferingOffNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for large body with buffering off")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for large body with buffering off")
}

func (s *ProxyBodySizeSuite) TestExactBoundaryBody() {
	body := bytes.Repeat([]byte("A"), 1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bodyLimitTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bodyLimitNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode,
		"exact boundary (1024 bytes against 1k limit) should be handled the same way")
}

func (s *ProxyBodySizeSuite) TestBodyLimitMBSuffix() {
	// 500KB body should be accepted under a 1m limit.
	smallBody := bytes.Repeat([]byte("M"), 500*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bodyLimitMBTraefikHost, http.MethodPost, "/", smallBody)
	nginxResp := s.makeRequestWithBody(s.nginx, bodyLimitMBNginxHost, http.MethodPost, "/", smallBody)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for 500KB body")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for 500KB body under 1m limit")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for 500KB body under 1m limit")

	// 2MB body should exceed the 1m limit.
	largeBody := bytes.Repeat([]byte("M"), 2*1024*1024)

	traefikResp = s.makeRequestWithBody(s.traefik, bodyLimitMBTraefikHost, http.MethodPost, "/", largeBody)
	nginxResp = s.makeRequestWithBody(s.nginx, bodyLimitMBNginxHost, http.MethodPost, "/", largeBody)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch for 2MB body")
	assert.Equal(s.T(), http.StatusRequestEntityTooLarge, nginxResp.StatusCode, "nginx should return 413 for body exceeding 1m limit")
}

func (s *ProxyBodySizeSuite) TestBufferConfigNormalRequest() {
	traefikResp := s.traefik.MakeRequest(s.T(), bufferConfigTraefikHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp := s.nginx.MakeRequest(s.T(), bufferConfigNginxHost, http.MethodGet, "/", nil, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 with buffer config annotations")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 with buffer config annotations")
}

func (s *ProxyBodySizeSuite) TestBufferConfigSmallBody() {
	body := bytes.Repeat([]byte("C"), 500)

	traefikResp := s.makeRequestWithBody(s.traefik, bufferConfigTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bufferConfigNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for small body with buffer config")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for small body with buffer config")
}

func (s *ProxyBodySizeSuite) TestBufferConfigLargeBody() {
	body := bytes.Repeat([]byte("C"), 100*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bufferConfigTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bufferConfigNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "expected 200 for large body with buffer config")
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "expected 200 for large body with buffer config")
}

func (s *ProxyBodySizeSuite) TestClientBodyBufferWithinBuffer() {
	// client-body-buffer-size is 16k. A body smaller than 16k should be buffered in memory.
	body := bytes.Repeat([]byte("D"), 8*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bufferConfigTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bufferConfigNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 for body within client-body-buffer-size")
}

func (s *ProxyBodySizeSuite) TestClientBodyBufferExceedsBuffer() {
	// client-body-buffer-size is 16k. A body larger than 16k spills to a temp file.
	// With proxy-max-temp-file-size=1024m, this should still succeed.
	body := bytes.Repeat([]byte("D"), 32*1024)

	traefikResp := s.makeRequestWithBody(s.traefik, bufferConfigTraefikHost, http.MethodPost, "/", body)
	nginxResp := s.makeRequestWithBody(s.nginx, bufferConfigNginxHost, http.MethodPost, "/", body)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode,
		"expected 200 for body exceeding client-body-buffer-size but within temp file limit")
}
