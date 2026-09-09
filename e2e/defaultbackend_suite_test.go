package e2e

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DefaultBackendSuite checks that an ingress spec.defaultBackend fallthrough
// behaves like ingress-nginx.
//
// Two concerns are covered:
//
//   - Path precedence: rule paths carry a path predicate and win; the default
//     backend is the host-only fallback for paths no rule matches. This is the
//     same routing logic regardless of any annotation, so it is tested once.
//   - Behavior inheritance: ingress-nginx applies server-wide behaviors (auth,
//     access control) to every location in the server, including the default
//     backend. Traefik should gate the fallthrough the same way it gates the
//     rule paths.
type DefaultBackendSuite struct {
	BaseSuite
}

const (
	dbDefaultBackendService = "status-backend"
	dbDefaultBackendMarker  = "status backend OK" // body served by status-backend
	dbSecretName            = "default-backend-basic-auth"

	dbCustomHeadersConfigMap = "default-backend-custom-headers"
	// X-Custom-Resp is part of the controller's globalAllowedResponseHeaders
	// allow-list (see traefik-helmchart.yaml.tmpl), so both controllers may emit it.
	dbCustomHeaderName  = "X-Custom-Resp"
	dbCustomHeaderValue = "custom-response-value"

	dbErrorBackendService = "error-backend"       // custom-http-errors error page service
	dbErrorPageMarker     = "custom error page"   // body served by error-backend
	dbTLSSecretName       = "default-backend-tls" // server cert for the ssl-redirect case
)

func TestDefaultBackendSuite(t *testing.T) {
	suite.Run(t, new(DefaultBackendSuite))
}

func (s *DefaultBackendSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	// Shared infrastructure for every case: the fallthrough service, the
	// external auth server (forward-auth gate) and the basic-auth secret.
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		require.NoError(s.T(), cluster.ApplyFixture("status-backend.yaml"),
			"deploy status-backend to %s cluster", cluster.Name)
		require.NoError(s.T(), cluster.ApplyFixture("auth-server.yaml"),
			"deploy auth-server to %s cluster", cluster.Name)
		require.NoError(s.T(), cluster.ApplyFixture("error-backend.yaml"),
			"deploy error-backend to %s cluster", cluster.Name)
	}
	for _, cluster := range []*Cluster{s.traefik, s.nginx} {
		require.NoError(s.T(), waitForDeployment(cluster, cluster.TestNamespace, "status-backend"),
			"status-backend not ready in %s cluster", cluster.Name)
		require.NoError(s.T(), waitForDeployment(cluster, cluster.TestNamespace, "auth-server"),
			"auth-server not ready in %s cluster", cluster.Name)
		require.NoError(s.T(), waitForDeployment(cluster, cluster.TestNamespace, "error-backend"),
			"error-backend not ready in %s cluster", cluster.Name)
	}

	htpasswd := htpasswdSHA(basicAuthUser, basicAuthPass)
	secret := secretTemplateData{
		Name: dbSecretName,
		Data: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(htpasswd))},
	}
	require.NoError(s.T(), s.traefik.DeploySecret(secret), "create secret in traefik cluster")
	require.NoError(s.T(), s.nginx.DeploySecret(secret), "create secret in nginx cluster")

	// Custom response headers: Traefik reads them per-ingress from the
	// custom-headers annotation; ingress-nginx applies them server-wide via the
	// controller's add-headers ConfigMap. Both must reach the default backend.
	cm := configMapTemplateData{
		Name: dbCustomHeadersConfigMap,
		Data: map[string]string{dbCustomHeaderName: dbCustomHeaderValue},
	}
	require.NoError(s.T(), s.traefik.DeployConfigMap(cm), "create custom-headers configmap in traefik cluster")
	require.NoError(s.T(), s.nginx.DeployConfigMap(cm), "create custom-headers configmap in nginx cluster")
	require.NoError(s.T(), s.nginx.Kubectl("patch", "configmap", "ingress-nginx-controller",
		"-n", s.nginx.ControllerNS,
		"--type=merge",
		"-p", fmt.Sprintf(`{"data":{"add-headers":"%s/%s"}}`, s.nginx.TestNamespace, dbCustomHeadersConfigMap),
	), "patch nginx controller configmap with add-headers")
}

func (s *DefaultBackendSuite) TearDownSuite() {
	_ = s.traefik.DeleteSecret(dbSecretName)
	_ = s.nginx.DeleteSecret(dbSecretName)
	_ = s.traefik.DeleteConfigMap(dbCustomHeadersConfigMap)
	_ = s.nginx.DeleteConfigMap(dbCustomHeadersConfigMap)
	_ = s.nginx.Kubectl("patch", "configmap", "ingress-nginx-controller",
		"-n", s.nginx.ControllerNS,
		"--type=json",
		"-p", `[{"op":"remove","path":"/data/add-headers"}]`,
	)
}

// deployPair deploys the same ingress (with a spec.defaultBackend) to both
// clusters under distinct hosts, waits for readiness, and registers cleanup.
func (s *DefaultBackendSuite) deployPair(t *testing.T, name, rulePath, pathType string, annotations map[string]string) (traefikHost, nginxHost string) {
	t.Helper()

	traefikHost = name + ".traefik.local"
	nginxHost = name + ".nginx.local"

	defaultBackend := &ingressDefaultBackend{ServiceName: dbDefaultBackendService, ServicePort: 80}

	for _, d := range []struct {
		cluster *Cluster
		host    string
	}{{s.traefik, traefikHost}, {s.nginx, nginxHost}} {
		err := d.cluster.DeployIngressWith(ingressTemplateData{
			Name:           name,
			Host:           d.host,
			Path:           rulePath,
			PathType:       pathType,
			Annotations:    annotations,
			DefaultBackend: defaultBackend,
		})
		require.NoError(t, err, "deploy ingress to %s cluster", d.cluster.Name)
	}

	t.Cleanup(func() {
		_ = s.traefik.DeleteIngress(name)
		_ = s.nginx.DeleteIngress(name)
	})

	s.traefik.WaitForIngressReady(t, traefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(t, nginxHost, 20, 1*time.Second)

	return traefikHost, nginxHost
}

// get issues the same request to both clusters and returns both responses.
func (s *DefaultBackendSuite) get(t *testing.T, traefikHost, nginxHost, path string, headers map[string]string) (traefikResp, nginxResp *Response) {
	t.Helper()

	traefikResp = s.traefik.MakeRequest(t, traefikHost, http.MethodGet, path, headers, 3, 1*time.Second)
	require.NotNil(t, traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(t, nginxHost, http.MethodGet, path, headers, 3, 1*time.Second)
	require.NotNil(t, nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func servedByDefaultBackend(resp *Response) bool {
	return strings.Contains(resp.Body, dbDefaultBackendMarker)
}

// TestPathPrecedence covers routing between the rule paths and the default
// backend, independent of any annotation. The rule backend is a whoami echo;
// the default backend is status-backend, identified by its response body.
func (s *DefaultBackendSuite) TestPathPrecedence() {
	testCases := []struct {
		desc     string
		rulePath string
		pathType string
		// path -> served by the default backend?
		expect map[string]bool
	}{
		{
			desc:     "exact rule path",
			rulePath: "/protected",
			pathType: "Exact",
			expect: map[string]bool{
				"/protected":     false, // exact match -> rule backend
				"/protected/sub": true,  // not exactly /protected -> default backend
				"/other":         true,  // unmatched -> default backend
			},
		},
		{
			desc:     "prefix rule path",
			rulePath: "/protected",
			pathType: "Prefix",
			expect: map[string]bool{
				"/protected":     false, // prefix match -> rule backend
				"/protected/sub": false, // still under the prefix -> rule backend
				"/other":         true,  // unmatched -> default backend
			},
		},
		{
			desc:     "root prefix shadows default backend",
			rulePath: "/",
			pathType: "Prefix",
			expect: map[string]bool{
				"/":       false, // catch-all rule swallows everything
				"/other":  false,
				"/deeper": false, // default backend is never reached
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.desc, func(t *testing.T) {
			name := "db-precedence-" + sanitizeName(tc.desc)
			traefikHost, nginxHost := s.deployPair(t, name, tc.rulePath, tc.pathType, nil)

			for path, wantDefaultBackend := range tc.expect {
				traefikResp, nginxResp := s.get(t, traefikHost, nginxHost, path, nil)

				assert.Equal(t, servedByDefaultBackend(nginxResp), servedByDefaultBackend(traefikResp),
					"path %q: traefik and ingress-nginx must route to the same backend", path)
				assert.Equal(t, wantDefaultBackend, servedByDefaultBackend(nginxResp),
					"path %q: unexpected ingress-nginx routing", path)
			}
		})
	}
}

// TestBehaviorInheritance checks that a server-wide behavior configured on the
// ingress gates the default-backend fallthrough exactly as it gates the rule
// path, matching ingress-nginx. A non-catch-all rule path (/protected, Prefix)
// leaves /other to fall through to the default backend.
func (s *DefaultBackendSuite) TestBehaviorInheritance() {
	testCases := []struct {
		desc        string
		annotations map[string]string
		// blocked is the status ingress-nginx returns when the gate denies a
		// request. 0 means no gate: both paths serve normally.
		blocked int
		// passHeaders, when set, satisfies the gate; the request then serves
		// the backend (used for basic auth).
		passHeaders map[string]string
	}{
		{
			desc:        "no annotation",
			annotations: nil,
			blocked:     0,
		},
		{
			desc: "basic auth",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-type":   "basic",
				"nginx.ingress.kubernetes.io/auth-secret": dbSecretName,
				"nginx.ingress.kubernetes.io/auth-realm":  basicAuthRealm,
			},
			blocked:     http.StatusUnauthorized,
			passHeaders: basicAuthHeader(basicAuthUser, basicAuthPass),
		},
		{
			desc: "forward auth",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-url": authServerServiceURL + "/deny",
			},
			blocked: http.StatusUnauthorized,
		},
		{
			desc: "ip whitelist",
			annotations: map[string]string{
				// TEST-NET-1: the test client is never in this range.
				"nginx.ingress.kubernetes.io/whitelist-source-range": "192.0.2.0/24",
			},
			blocked: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.desc, func(t *testing.T) {
			name := "db-inherit-" + sanitizeName(tc.desc)
			traefikHost, nginxHost := s.deployPair(t, name, "/protected", "Prefix", tc.annotations)

			const rulePath = "/protected"
			const defaultBackendPath = "/other"

			if tc.blocked == 0 {
				// No gate: both the rule path and the fallthrough serve.
				ruleT, ruleN := s.get(t, traefikHost, nginxHost, rulePath, nil)
				assert.Equal(t, http.StatusOK, ruleN.StatusCode, "ingress-nginx should serve the rule path")
				assert.Equal(t, ruleN.StatusCode, ruleT.StatusCode, "rule path status must match ingress-nginx")

				dbT, dbN := s.get(t, traefikHost, nginxHost, defaultBackendPath, nil)
				assert.Equal(t, http.StatusOK, dbN.StatusCode, "ingress-nginx should serve the default backend")
				assert.Equal(t, dbN.StatusCode, dbT.StatusCode, "default-backend status must match ingress-nginx")
				assert.True(t, servedByDefaultBackend(dbN), "sanity: ingress-nginx serves the default backend")
				assert.Equal(t, servedByDefaultBackend(dbN), servedByDefaultBackend(dbT),
					"traefik must also serve the default backend")
				return
			}

			// Gated: the rule path is the control, the default backend is the
			// path under test. Both must be denied like ingress-nginx.
			ruleT, ruleN := s.get(t, traefikHost, nginxHost, rulePath, nil)
			assert.Equal(t, tc.blocked, ruleN.StatusCode, "ingress-nginx should gate the rule path")
			assert.Equal(t, ruleN.StatusCode, ruleT.StatusCode, "rule path status must match ingress-nginx")

			dbT, dbN := s.get(t, traefikHost, nginxHost, defaultBackendPath, nil)
			assert.Equal(t, tc.blocked, dbN.StatusCode, "ingress-nginx should gate the default backend")
			assert.Equal(t, dbN.StatusCode, dbT.StatusCode,
				"default-backend fallthrough must be gated like ingress-nginx")

			// If the gate can be satisfied, a valid request serves the backend.
			if tc.passHeaders != nil {
				passT, passN := s.get(t, traefikHost, nginxHost, defaultBackendPath, tc.passHeaders)
				assert.Equal(t, http.StatusOK, passN.StatusCode, "ingress-nginx should serve with valid credentials")
				assert.Equal(t, passN.StatusCode, passT.StatusCode, "default-backend status with credentials must match ingress-nginx")
				assert.True(t, servedByDefaultBackend(passN), "sanity: credentials reach the default backend")
				assert.Equal(t, servedByDefaultBackend(passN), servedByDefaultBackend(passT),
					"traefik must serve the default backend with valid credentials")
			}
		})
	}
}

// TestResponseHeaderInheritance checks that server-wide custom response headers
// decorate the default-backend fallthrough, matching ingress-nginx. Traefik
// sources them from the custom-headers annotation; ingress-nginx from the global
// add-headers ConfigMap. A non-catch-all rule path (/protected, Prefix) leaves
// /other to fall through to the default backend, which must carry the header too.
func (s *DefaultBackendSuite) TestResponseHeaderInheritance() {
	t := s.T()

	const name = "db-headers"
	traefikHost := name + ".traefik.local"
	nginxHost := name + ".nginx.local"
	defaultBackend := &ingressDefaultBackend{ServiceName: dbDefaultBackendService, ServicePort: 80}

	// Traefik reads the header from the per-ingress custom-headers annotation;
	// ingress-nginx applies it server-wide from the add-headers ConfigMap patched
	// in SetupSuite, so its ingress carries no annotation (the annotation is not
	// how ingress-nginx sources response headers and would disturb its routing).
	require.NoError(t, s.traefik.DeployIngressWith(ingressTemplateData{
		Name:           name,
		Host:           traefikHost,
		Path:           "/protected",
		PathType:       "Prefix",
		Annotations:    map[string]string{"nginx.ingress.kubernetes.io/custom-headers": s.traefik.TestNamespace + "/" + dbCustomHeadersConfigMap},
		DefaultBackend: defaultBackend,
	}), "deploy ingress to traefik cluster")
	require.NoError(t, s.nginx.DeployIngressWith(ingressTemplateData{
		Name:           name,
		Host:           nginxHost,
		Path:           "/protected",
		PathType:       "Prefix",
		DefaultBackend: defaultBackend,
	}), "deploy ingress to nginx cluster")
	t.Cleanup(func() {
		_ = s.traefik.DeleteIngress(name)
		_ = s.nginx.DeleteIngress(name)
	})
	s.traefik.WaitForIngressReady(t, traefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(t, nginxHost, 20, 1*time.Second)

	// Control: the rule path carries the custom header on both controllers.
	ruleT, ruleN := s.get(t, traefikHost, nginxHost, "/protected", nil)
	assert.Equal(t, dbCustomHeaderValue, ruleN.ResponseHeaders.Get(dbCustomHeaderName),
		"sanity: ingress-nginx sets the custom header on the rule path")
	assert.Equal(t, ruleN.ResponseHeaders.Get(dbCustomHeaderName), ruleT.ResponseHeaders.Get(dbCustomHeaderName),
		"rule-path custom header must match ingress-nginx")

	// The default-backend fallthrough must carry the same header.
	dbT, dbN := s.get(t, traefikHost, nginxHost, "/other", nil)
	assert.True(t, servedByDefaultBackend(dbN), "sanity: ingress-nginx serves the default backend")
	assert.Equal(t, dbCustomHeaderValue, dbN.ResponseHeaders.Get(dbCustomHeaderName),
		"sanity: ingress-nginx sets the custom header on the default backend")
	assert.Equal(t, dbN.ResponseHeaders.Get(dbCustomHeaderName), dbT.ResponseHeaders.Get(dbCustomHeaderName),
		"default-backend fallthrough custom header must match ingress-nginx")
}

// TestErrorPageInheritance checks that custom-http-errors decorates the
// default-backend fallthrough exactly as ingress-nginx does. The rule path
// (/protected, Prefix) leaves /not-found to fall through to the default backend
// (status-backend), which returns 404; custom-http-errors then serves the
// error-backend page. Assertions compare Traefik directly against ingress-nginx.
func (s *DefaultBackendSuite) TestErrorPageInheritance() {
	t := s.T()

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/custom-http-errors": "404,503",
		"nginx.ingress.kubernetes.io/default-backend":    dbErrorBackendService,
	}
	traefikHost, nginxHost := s.deployPair(t, "db-errorpage", "/protected", "Prefix", annotations)

	traefikResp, nginxResp := s.get(t, traefikHost, nginxHost, "/not-found", nil)

	// Sanity: the scenario actually triggers the custom error page on ingress-nginx.
	assert.Contains(t, nginxResp.Body, dbErrorPageMarker,
		"sanity: ingress-nginx serves the custom error page on the default-backend fallthrough")

	// Same behavior as ingress-nginx: identical status and error-page body.
	assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode,
		"default-backend error status must match ingress-nginx")
	assert.Equal(t, strings.Contains(nginxResp.Body, dbErrorPageMarker), strings.Contains(traefikResp.Body, dbErrorPageMarker),
		"traefik must serve the custom error page on the default backend like ingress-nginx")
}

// TestSSLRedirectInheritance checks that a spec.tls section makes the
// default-backend fallthrough redirect to HTTPS, matching ingress-nginx which
// applies ssl-redirect (default true when TLS is configured) server-wide.
func (s *DefaultBackendSuite) TestSSLRedirectInheritance() {
	t := s.T()

	const name = "db-sslredirect"
	traefikHost := name + ".traefik.local"
	nginxHost := name + ".nginx.local"

	certs, err := generateAuthTLSCerts()
	require.NoError(t, err, "generate CA")
	serverCert, serverKey, err := generateServerCert(certs.caCert, certs.caKey, traefikHost, nginxHost)
	require.NoError(t, err, "generate server certificate")

	tlsSecret := secretTemplateData{
		Name: dbTLSSecretName,
		Type: "kubernetes.io/tls",
		Data: map[string]string{
			"tls.crt": base64.StdEncoding.EncodeToString(serverCert),
			"tls.key": base64.StdEncoding.EncodeToString(serverKey),
		},
	}
	require.NoError(t, s.traefik.DeploySecret(tlsSecret), "deploy tls secret to traefik cluster")
	require.NoError(t, s.nginx.DeploySecret(tlsSecret), "deploy tls secret to nginx cluster")

	defaultBackend := &ingressDefaultBackend{ServiceName: dbDefaultBackendService, ServicePort: 80}
	for _, d := range []struct {
		cluster *Cluster
		host    string
	}{{s.traefik, traefikHost}, {s.nginx, nginxHost}} {
		require.NoError(t, d.cluster.DeployIngressWith(ingressTemplateData{
			Name:           name,
			Host:           d.host,
			Path:           "/protected",
			PathType:       "Prefix",
			TLSSecret:      dbTLSSecretName,
			DefaultBackend: defaultBackend,
		}), "deploy ingress to %s cluster", d.cluster.Name)
	}
	t.Cleanup(func() {
		_ = s.traefik.DeleteIngress(name)
		_ = s.nginx.DeleteIngress(name)
		_ = s.traefik.DeleteSecret(dbTLSSecretName)
		_ = s.nginx.DeleteSecret(dbTLSSecretName)
	})
	s.traefik.WaitForIngressReady(t, traefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(t, nginxHost, 20, 1*time.Second)

	// An HTTP request to a path that falls through to the default backend must be
	// redirected to HTTPS, matching ingress-nginx.
	traefikResp, nginxResp := s.get(t, traefikHost, nginxHost, "/other", nil)
	assert.Equal(t, http.StatusPermanentRedirect, nginxResp.StatusCode,
		"sanity: ingress-nginx redirects the default-backend fallthrough to HTTPS")
	assert.Equal(t, nginxResp.StatusCode, traefikResp.StatusCode,
		"default-backend ssl-redirect must match ingress-nginx")
}
