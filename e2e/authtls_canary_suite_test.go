package e2e

import (
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// The production Ingress of the host declares no auth-tls annotation, its canary Ingress requires a client certificate.
	authTLSCanaryAuthHost = "mtls-canary-auth"

	// The production Ingress of the host declares no auth-tls annotation, its canary Ingress declares an auth-tls-secret that
	// does not exist.
	authTLSCanaryMissingHost = "mtls-canary-missing"

	// Only a canary Ingress declares this host, with an auth-tls-secret that does not exist.
	// ingress-nginx creates no server for such a host, and reports the error on its default server.
	authTLSCanaryOnlyHost    = "mtls-canary-only"
	authTLSCanaryUnknownHost = "mtls-canary-unknown"

	// No Ingress of the host declares an auth-tls annotation.
	authTLSCanaryPlainHost = "mtls-canary-plain"

	authTLSCanaryCASecretName      = "mtls-canary-ca"
	authTLSCanaryTLSSecretName     = "mtls-canary-tls"
	authTLSCanaryMissingSecretName = "mtls-canary-missing-secret"
)

// authTLSCanaryMode is a way for a canary Ingress to receive requests.
type authTLSCanaryMode struct {
	// annotations are the canary annotations of the mode, besides the one enabling the canary.
	annotations map[string]string
	// headers are the request headers routing a request to the canary backend, if the mode needs any.
	headers map[string]string
}

// authTLSCanaryModes holds the canary modes checked besides the canary receiving no request, by name.
// Each of them gets a host whose canary Ingress requires a client certificate, mtls-canary-<name>-auth, and a host whose
// canary Ingress declares an auth-tls-secret that does not exist, mtls-canary-<name>-missing.
var authTLSCanaryModes = map[string]authTLSCanaryMode{
	"wt-all": {
		annotations: map[string]string{"nginx.ingress.kubernetes.io/canary-weight": "100"},
	},
	"hdr": {
		annotations: map[string]string{"nginx.ingress.kubernetes.io/canary-by-header": "X-Canary"},
		headers:     map[string]string{"X-Canary": "always"},
	},
	"hdr-val": {
		annotations: map[string]string{
			"nginx.ingress.kubernetes.io/canary-by-header":       "X-Canary",
			"nginx.ingress.kubernetes.io/canary-by-header-value": "route-to-canary",
		},
		headers: map[string]string{"X-Canary": "route-to-canary"},
	},
	"cookie": {
		annotations: map[string]string{"nginx.ingress.kubernetes.io/canary-by-cookie": "canary_enabled"},
		headers:     map[string]string{"Cookie": "canary_enabled=always"},
	},
}

// authTLSCanaryModeHosts returns the hosts of the canary modes.
func authTLSCanaryModeHosts() []string {
	var hosts []string
	for name := range authTLSCanaryModes {
		hosts = append(hosts, "mtls-canary-"+name+"-auth", "mtls-canary-"+name+"-missing")
	}

	return hosts
}

// AuthTLSCanarySuite checks the auth-tls annotations of canary Ingresses, which ingress-nginx reads like the ones of any
// other Ingress when it configures the client certificate authentication of a server.
type AuthTLSCanarySuite struct {
	BaseSuite

	certs authTLSCerts
}

func TestAuthTLSCanarySuite(t *testing.T) {
	suite.Run(t, new(AuthTLSCanarySuite))
}

func (s *AuthTLSCanarySuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// TearDownSuite does not run when SetupSuite fails, and a remaining Ingress would break the suites running afterward.
	s.T().Cleanup(s.cleanup)

	err := s.traefik.ApplyFixture("canary-backend.yaml")
	require.NoError(s.T(), err, "deploy canary-backend fixture")
	require.NoError(s.T(), waitForDeployment(s.traefik, testNamespace, "canary-backend"), "canary-backend deployment not ready")

	s.certs, err = generateAuthTLSCerts()
	require.NoError(s.T(), err, "generate mTLS certificates")

	prodHosts := append([]string{authTLSCanaryAuthHost, authTLSCanaryMissingHost, authTLSCanaryPlainHost}, authTLSCanaryModeHosts()...)

	var certHosts []string
	for _, host := range prodHosts {
		certHosts = append(certHosts, host+".traefik.local", host+".nginx.local")
	}
	serverCert, serverKey, err := generateServerCert(s.certs.caCert, s.certs.caKey, certHosts...)
	require.NoError(s.T(), err, "generate server certificate")

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err = c.DeploySecret(secretTemplateData{
			Name: authTLSCanaryCASecretName,
			Type: "Opaque",
			Data: map[string]string{"ca.crt": base64.StdEncoding.EncodeToString(s.certs.caCertPEM)},
		})
		require.NoError(s.T(), err, "deploy CA secret to %s cluster", c.Name)

		err = c.DeploySecret(secretTemplateData{
			Name: authTLSCanaryTLSSecretName,
			Type: "kubernetes.io/tls",
			Data: map[string]string{
				"tls.crt": base64.StdEncoding.EncodeToString(serverCert),
				"tls.key": base64.StdEncoding.EncodeToString(serverKey),
			},
		})
		require.NoError(s.T(), err, "deploy server TLS secret to %s cluster", c.Name)
	}

	for _, host := range prodHosts {
		s.deployIngress(host+"-prod", host, authTLSCanaryTLSSecretName, "", nil)
	}
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		for _, host := range prodHosts {
			c.WaitForIngressReady(s.T(), host+"."+c.Name+".local", 30, time.Second)
		}
	}

	for name, mode := range authTLSCanaryModes {
		for suffix, secret := range map[string]string{"auth": authTLSCanaryCASecretName, "missing": authTLSCanaryMissingSecretName} {
			annotations := map[string]string{
				"nginx.ingress.kubernetes.io/canary":                 "true",
				"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + secret,
				"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
			}
			for key, value := range mode.annotations {
				annotations[key] = value
			}

			host := "mtls-canary-" + name + "-" + suffix
			s.deployIngress(host+"-canary", host, authTLSCanaryTLSSecretName, "canary-backend", annotations)
		}
	}

	s.deployIngress(authTLSCanaryAuthHost+"-canary", authTLSCanaryAuthHost, authTLSCanaryTLSSecretName, "canary-backend", map[string]string{
		"nginx.ingress.kubernetes.io/canary":                 "true",
		"nginx.ingress.kubernetes.io/canary-weight":          "0",
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSCanaryCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSCanaryMissingHost+"-canary", authTLSCanaryMissingHost, authTLSCanaryTLSSecretName, "canary-backend", map[string]string{
		"nginx.ingress.kubernetes.io/canary":                 "true",
		"nginx.ingress.kubernetes.io/canary-weight":          "0",
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSCanaryMissingSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSCanaryOnlyHost+"-canary", authTLSCanaryOnlyHost, "", "canary-backend", map[string]string{
		"nginx.ingress.kubernetes.io/canary":                 "true",
		"nginx.ingress.kubernetes.io/canary-weight":          "0",
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSCanaryMissingSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})

	// The controllers give no signal once they loaded the canary Ingresses, whose effect is what the tests look for.
	time.Sleep(20 * time.Second)
}

// cleanup deletes the resources of the suite, and waits until the hosts no Ingress declares stop answering 403.
func (s *AuthTLSCanarySuite) cleanup() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(200))
		s.T().Log(s.nginx.GetIngressControllerLogs(200))
	}

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		for _, host := range append([]string{authTLSCanaryAuthHost, authTLSCanaryMissingHost, authTLSCanaryPlainHost, authTLSCanaryOnlyHost}, authTLSCanaryModeHosts()...) {
			_ = c.DeleteIngress(host + "-prod")
			_ = c.DeleteIngress(host + "-canary")
		}
		_ = c.DeleteSecret(authTLSCanaryCASecretName)
		_ = c.DeleteSecret(authTLSCanaryTLSSecretName)
	}
	_ = s.traefik.Kubectl("delete", "-f", fixturesDir+"/canary-backend.yaml", "-n", testNamespace, "--ignore-not-found")

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		for range 30 {
			resp := c.MakeRequest(s.T(), authTLSCanaryUnknownHost+"."+c.Name+".local", http.MethodGet, "/", nil, 1, time.Second)
			if resp != nil && resp.StatusCode != http.StatusForbidden {
				break
			}
			time.Sleep(time.Second)
		}
	}
}

// deployIngress deploys an Ingress declaring the path / on <host>.<cluster>.local to both clusters.
func (s *AuthTLSCanarySuite) deployIngress(name, host, tlsSecret, service string, annotations map[string]string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngressWith(ingressTemplateData{
			Name:        name,
			Host:        host + "." + c.Name + ".local",
			Annotations: annotations,
			TLSSecret:   tlsSecret,
			ServiceName: service,
		})
		require.NoError(s.T(), err, "deploy %s ingress to %s cluster", name, c.Name)
	}
}

// statusOf returns the status code of a response, 0 standing for a request that got none, like a refused handshake.
func statusOf(resp *Response) int {
	if resp == nil {
		return 0
	}

	return resp.StatusCode
}

// TestCanaryClientAuthAppliesToHost checks that the client certificate authentication a canary Ingress declares applies to
// its host, whose production Ingress declares none.
func (s *AuthTLSCanarySuite) TestCanaryClientAuthAppliesToHost() {
	traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSCanaryAuthHost+".traefik.local", http.MethodGet, "/", nil, nil, 3, time.Second)
	nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSCanaryAuthHost+".nginx.local", http.MethodGet, "/", nil, nil, 3, time.Second)

	s.T().Logf("without client certificate: nginx=%d traefik=%d", statusOf(nginxResp), statusOf(traefikResp))
	assert.Contains(s.T(), []int{0, http.StatusBadRequest}, statusOf(nginxResp), "nginx: without client certificate")
	assert.Contains(s.T(), []int{0, http.StatusBadRequest}, statusOf(traefikResp), "traefik: without client certificate")

	traefikResp = s.traefik.MakeTLSRequest(s.T(), authTLSCanaryAuthHost+".traefik.local", http.MethodGet, "/", nil, &s.certs.clientCert, 3, time.Second)
	nginxResp = s.nginx.MakeTLSRequest(s.T(), authTLSCanaryAuthHost+".nginx.local", http.MethodGet, "/", nil, &s.certs.clientCert, 3, time.Second)

	s.T().Logf("with client certificate: nginx=%d traefik=%d", statusOf(nginxResp), statusOf(traefikResp))
	assert.Equal(s.T(), http.StatusOK, statusOf(nginxResp), "nginx: with client certificate")
	assert.Equal(s.T(), http.StatusOK, statusOf(traefikResp), "traefik: with client certificate")
}

// TestCanaryClientAuthOverwritesClientSentHeaders checks that the Ssl-Client-* headers a client sends do not reach the
// backend of a host whose client certificate authentication comes from a canary Ingress.
func (s *AuthTLSCanarySuite) TestCanaryClientAuthOverwritesClientSentHeaders() {
	forged := forgedSSLClientHeaders()
	delete(forged, "Ssl-Client-Cert")

	for _, clientCert := range []*tls.Certificate{nil, &s.certs.clientCert} {
		traefikResp := s.traefik.MakeTLSRequest(s.T(), authTLSCanaryAuthHost+".traefik.local", http.MethodGet, "/", forged, clientCert, 3, time.Second)
		nginxResp := s.nginx.MakeTLSRequest(s.T(), authTLSCanaryAuthHost+".nginx.local", http.MethodGet, "/", forged, clientCert, 3, time.Second)

		var traefikHeaders, nginxHeaders map[string]string
		if traefikResp != nil && traefikResp.StatusCode == http.StatusOK {
			traefikHeaders = sslClientHeadersReaching(traefikResp.RequestHeaders)
		}
		if nginxResp != nil && nginxResp.StatusCode == http.StatusOK {
			nginxHeaders = sslClientHeadersReaching(nginxResp.RequestHeaders)
		}

		s.T().Logf("client certificate=%v: nginx=%d %v traefik=%d %v", clientCert != nil, statusOf(nginxResp), nginxHeaders, statusOf(traefikResp), traefikHeaders)
		for name, value := range forged {
			if name == "Ssl-Client-Verify" && clientCert != nil {
				continue
			}
			assert.NotEqual(s.T(), value, nginxHeaders[name], "nginx: client-sent %s reached the backend (client certificate: %v)", name, clientCert != nil)
			assert.NotEqual(s.T(), value, traefikHeaders[name], "traefik: client-sent %s reached the backend (client certificate: %v)", name, clientCert != nil)
		}
	}
}

// TestCanaryMissingSecretForbidsWholeHost checks that the auth-tls-secret a canary Ingress declares, and that does not
// exist, makes its host answer 403.
func (s *AuthTLSCanarySuite) TestCanaryMissingSecretForbidsWholeHost() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		httpResp := c.MakeRequest(s.T(), authTLSCanaryMissingHost+"."+c.Name+".local", http.MethodGet, "/", nil, 3, time.Second)
		httpsResp := c.MakeTLSRequest(s.T(), authTLSCanaryMissingHost+"."+c.Name+".local", http.MethodGet, "/", nil, nil, 3, time.Second)

		s.T().Logf("%s: HTTP=%d HTTPS=%d", c.Name, statusOf(httpResp), statusOf(httpsResp))
		assert.Equal(s.T(), []int{http.StatusForbidden, http.StatusForbidden}, []int{statusOf(httpResp), statusOf(httpsResp)}, c.Name)
	}
}

// TestCanaryModes checks that the auth-tls annotations of a canary Ingress apply to its host whichever way the canary
// receives requests, for the requests routed to the canary backend as well as for the other ones.
func (s *AuthTLSCanarySuite) TestCanaryModes() {
	for name, mode := range authTLSCanaryModes {
		headerSets := []map[string]string{nil}
		if mode.headers != nil {
			headerSets = append(headerSets, mode.headers)
		}

		for _, headers := range headerSets {
			for _, c := range []*Cluster{s.traefik, s.nginx} {
				authHost := "mtls-canary-" + name + "-auth." + c.Name + ".local"
				missingHost := "mtls-canary-" + name + "-missing." + c.Name + ".local"

				withoutCert := c.MakeTLSRequest(s.T(), authHost, http.MethodGet, "/", headers, nil, 3, time.Second)
				withCert := c.MakeTLSRequest(s.T(), authHost, http.MethodGet, "/", headers, &s.certs.clientCert, 3, time.Second)
				missingHTTP := c.MakeRequest(s.T(), missingHost, http.MethodGet, "/", headers, 3, time.Second)
				missingHTTPS := c.MakeTLSRequest(s.T(), missingHost, http.MethodGet, "/", headers, nil, 3, time.Second)

				var backend string
				if withCert != nil {
					backend = withCert.RequestHeaders["Hostname"]
				}
				s.T().Logf("%s: %s: canary headers=%v: without certificate=%d with certificate=%d (backend %s) missing secret HTTP=%d HTTPS=%d",
					name, c.Name, headers != nil, statusOf(withoutCert), statusOf(withCert), backend, statusOf(missingHTTP), statusOf(missingHTTPS))

				assert.Contains(s.T(), []int{0, http.StatusBadRequest}, statusOf(withoutCert), "%s: %s: without client certificate", name, c.Name)
				assert.Equal(s.T(), http.StatusOK, statusOf(withCert), "%s: %s: with client certificate", name, c.Name)
				assert.Equal(s.T(), []int{http.StatusForbidden, http.StatusForbidden}, []int{statusOf(missingHTTP), statusOf(missingHTTPS)}, "%s: %s: missing secret", name, c.Name)
			}
		}
	}
}

// TestCanaryOnlyHostMissingSecretForbidsDefaultServer checks that the auth-tls-secret that does not exist, declared by the
// canary Ingress of a host no other Ingress declares, makes the default server answer 403.
func (s *AuthTLSCanarySuite) TestCanaryOnlyHostMissingSecretForbidsDefaultServer() {
	for _, host := range []string{authTLSCanaryOnlyHost, authTLSCanaryUnknownHost} {
		for _, c := range []*Cluster{s.traefik, s.nginx} {
			httpResp := c.MakeRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, "/", nil, 3, time.Second)
			httpsResp := c.MakeTLSRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, "/", nil, nil, 3, time.Second)

			s.T().Logf("%s: %s: HTTP=%d HTTPS=%d", c.Name, host, statusOf(httpResp), statusOf(httpsResp))
			assert.Equal(s.T(), []int{http.StatusForbidden, http.StatusForbidden}, []int{statusOf(httpResp), statusOf(httpsResp)}, "%s: %s", c.Name, host)
		}
	}
}

// TestKnownDifference_HostWithoutClientAuthForwardsClientSentHeaders checks that both controllers forward the Ssl-Client-*
// headers a client sends to a host without client certificate authentication.
// Nginx drops a request header whose name contains an underscore or a dot on every server, so only Traefik forwards the
// aliasing spellings, which carry nothing more than the canonical ones both controllers forward.
func (s *AuthTLSCanarySuite) TestKnownDifference_HostWithoutClientAuthForwardsClientSentHeaders() {
	headers := forgedSSLClientHeaders()
	for name, value := range forgedAliasingSSLClientHeaders() {
		headers[name] = value
	}

	traefikHeaders, nginxHeaders := requestSSLClientHeaders(s.T(), s.traefik, s.nginx,
		authTLSCanaryPlainHost+".traefik.local", authTLSCanaryPlainHost+".nginx.local", "/", headers, nil)

	assert.Equal(s.T(), forgedSSLClientHeaders(), nginxHeaders, "nginx")
	for name, value := range forgedSSLClientHeaders() {
		assert.Equal(s.T(), value, traefikHeaders[name], "traefik: %s", name)
	}
	assert.Len(s.T(), traefikHeaders, len(headers), "traefik")
}
