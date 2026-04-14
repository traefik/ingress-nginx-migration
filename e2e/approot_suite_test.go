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
	appRootIngressName = "app-root-test"
	appRootTraefikHost = appRootIngressName + ".traefik.local"
	appRootNginxHost   = appRootIngressName + ".nginx.local"
)

type AppRootSuite struct {
	BaseSuite
}

func TestAppRootSuite(t *testing.T) {
	suite.Run(t, new(AppRootSuite))
}

func (s *AppRootSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	appRootAnnotations := map[string]string{
		"nginx.ingress.kubernetes.io/app-root": "/dashboard",
	}

	err := s.traefik.DeployIngress(appRootIngressName, appRootTraefikHost, appRootAnnotations)
	require.NoError(s.T(), err, "deploy app-root ingress to traefik cluster")

	err = s.nginx.DeployIngress(appRootIngressName, appRootNginxHost, appRootAnnotations)
	require.NoError(s.T(), err, "deploy app-root ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), appRootTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), appRootNginxHost, 20, 1*time.Second)
}

func (s *AppRootSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(appRootIngressName)
	_ = s.nginx.DeleteIngress(appRootIngressName)
}

func (s *AppRootSuite) request(method, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), appRootTraefikHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), appRootNginxHost, method, path, headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *AppRootSuite) TestAppRoot() {
	testCases := []struct {
		desc    string
		method  string
		path    string
		headers map[string]string
		check   func(t *testing.T, traefikResp, nginxResp *Response)
	}{
		{
			desc:   "root redirects to app-root",
			method: http.MethodGet,
			path:   "/",
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
				assert.Equal(t, http.StatusFound, traefikResp.StatusCode, "expected redirect for /")

				traefikLocation := traefikResp.ResponseHeaders.Get("Location")
				nginxLocation := nginxResp.ResponseHeaders.Get("Location")

				assert.True(t, strings.HasSuffix(traefikLocation, "/dashboard"),
					"traefik Location header should end with /dashboard, got: %s", traefikLocation)
				assert.True(t, strings.HasSuffix(nginxLocation, "/dashboard"),
					"nginx Location header should end with /dashboard, got: %s", nginxLocation)
			},
		},
		{
			desc:   "non-root path passthrough",
			method: http.MethodGet,
			path:   "/other",
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "expected 200 for non-root path /other")

				traefikLocation := traefikResp.ResponseHeaders.Get("Location")
				assert.Equal(t, "", traefikLocation, "no redirect for non-root path")
			},
		},
		{
			desc:   "non-root path with trailing slash",
			method: http.MethodGet,
			path:   "/some/path/",
			check: func(t *testing.T, traefikResp, nginxResp *Response) {
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
				assert.Equal(t, http.StatusOK, traefikResp.StatusCode, "expected 200 for /some/path/")
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.desc, func(t *testing.T) {
			traefikResp, nginxResp := s.request(tc.method, tc.path, tc.headers)
			tc.check(t, traefikResp, nginxResp)
		})
	}
}
