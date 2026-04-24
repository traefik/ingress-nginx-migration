package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TLSSuite struct {
	BaseSuite
}

func TestTLSSuite(t *testing.T) {
	suite.Run(t, new(TLSSuite))
}

func (s *TLSSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
}

func (s *TLSSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}
}

func (s *TLSSuite) TestTLS() {
	testCases := []struct {
		desc           string
		annotations    map[string]string
		tlsSecret      string
		defaultBackend *ingressDefaultBackend
		test           func(t *testing.T, hostTraefik, hostNginx string)
	}{
		{
			desc:        "no .spec.tls section",
			annotations: map[string]string{},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeTLSRequest(t, hostTraefik, http.MethodGet, "/resource", nil, nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeTLSRequest(t, hostNginx, http.MethodGet, "/resource", nil, nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "traefik should match nginx behavior")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "expected 200 — no ssl-redirect annotation")
			},
		},
		{
			desc:        "invalid tls secret",
			annotations: map[string]string{},
			tlsSecret:   "invalid-secret",
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeTLSRequest(t, hostTraefik, http.MethodGet, "/resource", nil, nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeTLSRequest(t, hostNginx, http.MethodGet, "/resource", nil, nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "traefik should match nginx behavior")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "expected 200 — TLS served with default cert when secret is invalid")
			},
		},
		{
			desc: "invalid tls secret - with force-ssl-redirect",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/force-ssl-redirect": "true",
			},
			tlsSecret: "invalid-secret",
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				traefikResp := s.traefik.MakeTLSRequest(t, hostTraefik, http.MethodGet, "/resource", nil, nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeTLSRequest(t, hostNginx, http.MethodGet, "/resource", nil, nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "traefik should match nginx behavior")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "expected 200 — TLS served normally even with invalid secret")

				traefikResp = s.traefik.MakeRequest(t, hostTraefik, http.MethodGet, "/resource", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp = s.nginx.MakeRequest(t, hostNginx, http.MethodGet, "/resource", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				// force-ssl-redirect: true + spec.tls present → HTTP requests are permanently redirected to HTTPS.
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "traefik should match nginx behavior")
				assert.Equal(t, http.StatusPermanentRedirect, nginxResp.StatusCode, "expected 308 — force-ssl-redirect always redirects HTTP to HTTPS")
			},
		},
		{
			desc:        "default backend",
			annotations: map[string]string{},
			defaultBackend: &ingressDefaultBackend{
				ServiceName: "status-backend",
				ServicePort: 80,
			},
			test: func(t *testing.T, hostTraefik, hostNginx string) {
				t.Helper()

				// HTTP
				traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodGet, "/other", nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodGet, "/other", nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "traefik should match nginx behavior")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "expected 200 — default backend serves unmatched paths")

				// TLS
				traefikResp = s.traefik.MakeTLSRequest(t, hostTraefik, http.MethodGet, "/", nil, nil, 3, 1*time.Second)
				require.NotNil(t, traefikResp, "traefik response should not be nil")

				nginxResp = s.nginx.MakeTLSRequest(t, hostNginx, http.MethodGet, "/", nil, nil, 3, 1*time.Second)
				require.NotNil(t, nginxResp, "nginx response should not be nil")

				// No TLS section and no ssl-redirect annotation → HTTPS requests are served normally.
				assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "traefik should match nginx behavior")
				assert.Equal(t, http.StatusOK, nginxResp.StatusCode, "expected 200 — no ssl-redirect annotation")
			},
		},
	}

	for _, test := range testCases {
		s.T().Run(test.desc, func(t *testing.T) {
			t.Parallel()
			prefix := sanitizeName(test.desc)
			hostTraefik := prefix + ".traefik.local"
			hostNginx := prefix + ".nginx.local"

			if test.defaultBackend != nil {
				// Deploy status-backend and error-backend to both clusters.
				for _, cluster := range []*Cluster{s.traefik, s.nginx} {
					err := cluster.ApplyFixture("status-backend.yaml")
					require.NoError(t, err, "deploy status-backend to %s cluster", cluster.Name)
				}

				// Wait for backends to be ready.
				for _, cluster := range []*Cluster{s.traefik, s.nginx} {
					err := waitForDeployment(cluster, cluster.TestNamespace, "status-backend")
					require.NoError(t, err, "status-backend not ready in %s cluster", cluster.Name)
				}
			}

			err := s.traefik.DeployIngressWith(ingressTemplateData{
				Name:           prefix,
				Host:           hostTraefik,
				Annotations:    test.annotations,
				TLSSecret:      test.tlsSecret,
				DefaultBackend: test.defaultBackend,
			})
			require.NoError(t, err, "deploy %s ingress to traefik cluster", prefix)

			err = s.nginx.DeployIngressWith(ingressTemplateData{
				Name:           prefix,
				Host:           hostNginx,
				Annotations:    test.annotations,
				TLSSecret:      test.tlsSecret,
				DefaultBackend: test.defaultBackend,
			})
			require.NoError(t, err, "deploy %s ingress to nginx cluster", prefix)

			t.Cleanup(func() {
				_ = s.traefik.DeleteIngress(prefix)
				_ = s.nginx.DeleteIngress(prefix)
			})

			s.traefik.WaitForIngressReady(t, hostTraefik, 20, 1*time.Second)
			s.nginx.WaitForIngressReady(t, hostNginx, 20, 1*time.Second)

			test.test(t, hostTraefik, hostNginx)
		})
	}
}
