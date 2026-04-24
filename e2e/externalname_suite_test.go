package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ExternalNameSuite struct {
	BaseSuite
}

func TestExternalNameSuite(t *testing.T) {
	suite.Run(t, new(ExternalNameSuite))
}

func (s *ExternalNameSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// The ExternalName service is shared by both controllers (same k3s cluster).
	err := s.traefik.ApplyFixture("external-name-service.yaml")
	require.NoError(s.T(), err, "deploy external-name service")
}

func (s *ExternalNameSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}
}

func (s *ExternalNameSuite) TestExternalName() {
	testCases := []struct {
		desc            string
		servicePort     int
		servicePortName string
		annotations     map[string]string
	}{
		{
			desc:        "matching numeric port",
			servicePort: 80,
		},
		{
			desc:            "matching named port",
			servicePortName: "http",
		},
		{
			desc:        "non-matching numeric port",
			servicePort: 3000,
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-connect-timeout": "1",
			},
		},
		{
			desc:            "non-matching named port",
			servicePortName: "foo",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/proxy-connect-timeout": "1",
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			prefix := sanitizeName(tc.desc)
			hostTraefik := prefix + ".traefik.local"
			hostNginx := prefix + ".nginx.local"

			ingressData := ingressTemplateData{
				ServiceName:     "external-backend",
				ServicePort:     tc.servicePort,
				ServicePortName: tc.servicePortName,
				Annotations:     tc.annotations,
			}

			traefikData := ingressData
			traefikData.Name = prefix
			traefikData.Host = hostTraefik
			err := s.traefik.DeployIngressWith(traefikData)
			require.NoError(t, err, "deploy ingress to traefik cluster")

			nginxData := ingressData
			nginxData.Name = prefix
			nginxData.Host = hostNginx
			err = s.nginx.DeployIngressWith(nginxData)
			require.NoError(t, err, "deploy ingress to nginx cluster")

			t.Cleanup(func() {
				_ = s.traefik.DeleteIngress(prefix)
				_ = s.nginx.DeleteIngress(prefix)
			})

			s.traefik.WaitForIngressReady(t, hostTraefik, 20, 1*time.Second)
			s.nginx.WaitForIngressReady(t, hostNginx, 20, 1*time.Second)

			traefikResp := s.traefik.MakeRequest(t, hostTraefik, http.MethodGet, "/", nil, 3, 1*time.Second)
			require.NotNil(t, traefikResp, "traefik response should not be nil")

			nginxResp := s.nginx.MakeRequest(t, hostNginx, http.MethodGet, "/", nil, 3, 1*time.Second)
			require.NotNil(t, nginxResp, "nginx response should not be nil")

			assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch between traefik and nginx")
		})
	}
}