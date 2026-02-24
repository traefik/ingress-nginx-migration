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

	s.traefik.WaitForIngressReady(s.T(), bodyLimitTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bodyLimitNginxHost, 20, 1*time.Second)
	s.traefik.WaitForIngressReady(s.T(), bodyNoLimitTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), bodyNoLimitNginxHost, 20, 1*time.Second)
}

func (s *ProxyBodySizeSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(bodyLimitIngressName)
	_ = s.nginx.DeleteIngress(bodyLimitIngressName)
	_ = s.traefik.DeleteIngress(bodyNoLimitIngressName)
	_ = s.nginx.DeleteIngress(bodyNoLimitIngressName)
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

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(s.T(), err)

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
