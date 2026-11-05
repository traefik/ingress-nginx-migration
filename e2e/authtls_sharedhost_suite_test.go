package e2e

import (
	"crypto/tls"
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

const (
	// One Ingress requires a client certificate, another one declares a path on the same host without any auth-tls annotation.
	authTLSMixedHost                 = "mtls-mixed-host"
	authTLSMixedHostProtectedIngress = "mtls-mixed-host-protected"
	authTLSMixedHostPublicIngress    = "mtls-mixed-host-public"
	// Another Ingress without any auth-tls annotation disables the HTTPS redirection, so that its path is served over plaintext HTTP.
	authTLSMixedHostPlaintextIngress = "mtls-mixed-host-plaintext"

	// Two Ingresses on the same host declare different auth-tls-verify-client values.
	// The "required" one is processed first by both controllers: ingresses are sorted by creation timestamp,
	// ties being broken by descending namespace/name, and it is both created first and named after "optional".
	// Only the "required" one passes the certificate to the upstream.
	authTLSMixedModesHost            = "mtls-mixed-modes"
	authTLSMixedModesRequiredIngress = "mtls-mixed-modes-required"
	authTLSMixedModesOptionalIngress = "mtls-mixed-modes-optional"

	// The first Ingress of the host requests a client certificate without verifying it during the handshake (optional_no_ca),
	// the second one requires a verified client certificate and passes it to the upstream.
	// Names are ordered like the mixed modes ones, so that the "owner" one is processed first.
	authTLSNoCAHost              = "mtls-no-ca-host"
	authTLSNoCAHostOwnerIngress  = "mtls-no-ca-host-owner"
	authTLSNoCAHostMemberIngress = "mtls-no-ca-host-member"

	// Two Ingresses on the same host declare different CA secrets, the "owner" one being processed first.
	authTLSOtherCAHost              = "mtls-other-ca-host"
	authTLSOtherCAHostOwnerIngress  = "mtls-other-ca-host-owner"
	authTLSOtherCAHostMemberIngress = "mtls-other-ca-host-member"

	// An Ingress declares an auth-tls-secret on a rule without paths. ingress-nginx skips such a rule before reading its
	// auth-tls annotations, so it does not configure client certificate authentication for the path of another Ingress.
	authTLSNoPathsHost            = "mtls-no-paths-host"
	authTLSNoPathsHostRuleIngress = "mtls-no-paths-host-rule"
	authTLSNoPathsHostPathIngress = "mtls-no-paths-host-path"

	// An Ingress declares an auth-tls-secret that does not exist. ingress-nginx then answers 403 to every request for the host,
	// including the paths of the other Ingresses of the host.
	authTLSMissingSecretHost              = "mtls-missing-secret-host"
	authTLSMissingSecretHostBrokenIngress = "mtls-missing-secret-host-broken"
	authTLSMissingSecretHostPublicIngress = "mtls-missing-secret-host-public"
	authTLSMissingSecretName              = "mtls-missing-secret"

	// The first Ingress of the host declares an auth-tls-secret without CA certificate, and passes the certificate to the upstream.
	// ingress-nginx disables client certificate authentication for that Ingress, so the second one configures it for the host.
	// Names are ordered like the mixed modes ones, so that the "without-ca" one is processed first.
	authTLSWithoutCAHost                 = "mtls-without-ca-host"
	authTLSWithoutCAHostWithoutCAIngress = "mtls-without-ca-host-without-ca"
	authTLSWithoutCAHostOwnerIngress     = "mtls-without-ca-host-owner"

	// A single Ingress declares an auth-tls-secret without CA certificate: ingress-nginx serves it without client certificate authentication.
	authTLSWithoutCAOnlyHost    = "mtls-without-ca-only"
	authTLSWithoutCAOnlyIngress = "mtls-without-ca-only"

	authTLSSharedHostCASecretName      = "mtls-shared-host-ca"
	authTLSSharedHostOtherCASecretName = "mtls-shared-host-other-ca"
	authTLSSharedHostTLSSecretName     = "mtls-shared-host-tls"
)

// AuthTLSSharedHostSuite checks that client certificate authentication applies to every path of a host,
// as ingress-nginx configures it on the server block from the first Ingress declaring it for the host.
type AuthTLSSharedHostSuite struct {
	BaseSuite

	certs authTLSCerts
	// otherCerts holds a CA declared by a second Ingress of a host, and a client certificate it signed.
	otherCerts authTLSCerts
	// selfSignedClientCert is a client certificate signed by its own key.
	selfSignedClientCert tls.Certificate
}

func TestAuthTLSSharedHostSuite(t *testing.T) {
	suite.Run(t, new(AuthTLSSharedHostSuite))
}

func (s *AuthTLSSharedHostSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	certs, err := generateAuthTLSCerts()
	require.NoError(s.T(), err, "generate mTLS certificates")
	s.certs = certs

	s.otherCerts, err = generateAuthTLSCertsWithCAName("Test mTLS Other CA")
	require.NoError(s.T(), err, "generate other mTLS certificates")

	s.selfSignedClientCert, err = generateSelfSignedClientCert("Self-Signed Client")
	require.NoError(s.T(), err, "generate self-signed client certificate")

	serverCert, serverKey, err := generateServerCert(s.certs.caCert, s.certs.caKey,
		authTLSMixedHost+".traefik.local", authTLSMixedHost+".nginx.local",
		authTLSMixedModesHost+".traefik.local", authTLSMixedModesHost+".nginx.local",
		authTLSNoCAHost+".traefik.local", authTLSNoCAHost+".nginx.local",
		authTLSOtherCAHost+".traefik.local", authTLSOtherCAHost+".nginx.local",
		authTLSNoPathsHost+".traefik.local", authTLSNoPathsHost+".nginx.local",
		authTLSMissingSecretHost+".traefik.local", authTLSMissingSecretHost+".nginx.local",
		authTLSWithoutCAHost+".traefik.local", authTLSWithoutCAHost+".nginx.local",
		authTLSWithoutCAOnlyHost+".traefik.local", authTLSWithoutCAOnlyHost+".nginx.local",
	)
	require.NoError(s.T(), err, "generate server certificate")

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err = c.DeploySecret(secretTemplateData{
			Name: authTLSSharedHostCASecretName,
			Type: "Opaque",
			Data: map[string]string{
				"ca.crt": base64.StdEncoding.EncodeToString(s.certs.caCertPEM),
			},
		})
		require.NoError(s.T(), err, "deploy CA secret to %s cluster", c.Name)

		err = c.DeploySecret(secretTemplateData{
			Name: authTLSSharedHostOtherCASecretName,
			Type: "Opaque",
			Data: map[string]string{
				"ca.crt": base64.StdEncoding.EncodeToString(s.otherCerts.caCertPEM),
			},
		})
		require.NoError(s.T(), err, "deploy other CA secret to %s cluster", c.Name)

		err = c.DeploySecret(secretTemplateData{
			Name: authTLSSharedHostTLSSecretName,
			Type: "kubernetes.io/tls",
			Data: map[string]string{
				"tls.crt": base64.StdEncoding.EncodeToString(serverCert),
				"tls.key": base64.StdEncoding.EncodeToString(serverKey),
			},
		})
		require.NoError(s.T(), err, "deploy server TLS secret to %s cluster", c.Name)
	}

	s.deployIngress(authTLSMixedHostProtectedIngress, authTLSMixedHost, "/protected", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSMixedHostPublicIngress, authTLSMixedHost, "/public", "", nil)
	s.deployIngress(authTLSMixedHostPlaintextIngress, authTLSMixedHost, "/plaintext", "", map[string]string{
		"nginx.ingress.kubernetes.io/ssl-redirect": "false",
	})

	s.deployIngress(authTLSMixedModesRequiredIngress, authTLSMixedModesHost, "/required", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "on",
		"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
	})
	s.deployIngress(authTLSMixedModesOptionalIngress, authTLSMixedModesHost, "/optional", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "optional",
	})

	s.deployIngress(authTLSNoCAHostOwnerIngress, authTLSNoCAHost, "/owner", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "optional_no_ca",
	})
	s.deployIngress(authTLSNoCAHostMemberIngress, authTLSNoCAHost, "/member", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "on",
		"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
	})

	s.deployIngress(authTLSOtherCAHostOwnerIngress, authTLSOtherCAHost, "/owner", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSOtherCAHostMemberIngress, authTLSOtherCAHost, "/member", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostOtherCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})

	s.deployIngressWithoutPaths(authTLSNoPathsHostRuleIngress, authTLSNoPathsHost, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSNoPathsHostPathIngress, authTLSNoPathsHost, "/path", "", nil)

	s.deployIngress(authTLSMissingSecretHostBrokenIngress, authTLSMissingSecretHost, "/broken", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSMissingSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})
	s.deployIngress(authTLSMissingSecretHostPublicIngress, authTLSMissingSecretHost, "/public", authTLSSharedHostTLSSecretName, nil)

	// The server TLS secret holds a certificate and its key, but no CA certificate.
	s.deployIngress(authTLSWithoutCAHostWithoutCAIngress, authTLSWithoutCAHost, "/without-ca", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":                       testNamespace + "/" + authTLSSharedHostTLSSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "on",
		"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "true",
	})
	s.deployIngress(authTLSWithoutCAHostOwnerIngress, authTLSWithoutCAHost, "/owner", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostCASecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})

	s.deployIngress(authTLSWithoutCAOnlyIngress, authTLSWithoutCAOnlyHost, "/", authTLSSharedHostTLSSecretName, map[string]string{
		"nginx.ingress.kubernetes.io/auth-tls-secret":        testNamespace + "/" + authTLSSharedHostTLSSecretName,
		"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
	})

	// Every Ingress of a host must be loaded before asserting: with only the protected one,
	// the host would require a client certificate even on an affected Traefik version.
	// The paths of the Ingresses declaring an auth-tls-secret without CA certificate are not waited for:
	// an affected Traefik version does not serve them at all, which the tests report.
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		s.waitForPath(c, authTLSNoPathsHost, "/path")
		s.waitForPath(c, authTLSMissingSecretHost, "/public")
		s.waitForPath(c, authTLSWithoutCAHost, "/owner")
		s.waitForPath(c, authTLSMixedHost, "/protected")
		s.waitForPath(c, authTLSMixedHost, "/public")
		s.waitForPath(c, authTLSMixedHost, "/plaintext")
		s.waitForPath(c, authTLSMixedModesHost, "/required")
		s.waitForPath(c, authTLSMixedModesHost, "/optional")
		s.waitForPath(c, authTLSNoCAHost, "/owner")
		s.waitForPath(c, authTLSNoCAHost, "/member")
		s.waitForPath(c, authTLSOtherCAHost, "/owner")
		s.waitForPath(c, authTLSOtherCAHost, "/member")
	}
}

func (s *AuthTLSSharedHostSuite) TearDownSuite() {
	if s.T().Failed() {
		s.T().Log(s.traefik.GetIngressControllerLogs(500))
		s.T().Log(s.nginx.GetIngressControllerLogs(500))
	}

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		_ = c.DeleteIngress(authTLSMixedHostProtectedIngress)
		_ = c.DeleteIngress(authTLSMixedHostPublicIngress)
		_ = c.DeleteIngress(authTLSMixedHostPlaintextIngress)
		_ = c.DeleteIngress(authTLSMixedModesRequiredIngress)
		_ = c.DeleteIngress(authTLSMixedModesOptionalIngress)
		_ = c.DeleteIngress(authTLSNoCAHostOwnerIngress)
		_ = c.DeleteIngress(authTLSNoCAHostMemberIngress)
		_ = c.DeleteIngress(authTLSOtherCAHostOwnerIngress)
		_ = c.DeleteIngress(authTLSOtherCAHostMemberIngress)
		_ = c.DeleteIngress(authTLSNoPathsHostRuleIngress)
		_ = c.DeleteIngress(authTLSNoPathsHostPathIngress)
		_ = c.DeleteIngress(authTLSMissingSecretHostBrokenIngress)
		_ = c.DeleteIngress(authTLSMissingSecretHostPublicIngress)
		_ = c.DeleteIngress(authTLSWithoutCAHostWithoutCAIngress)
		_ = c.DeleteIngress(authTLSWithoutCAHostOwnerIngress)
		_ = c.DeleteIngress(authTLSWithoutCAOnlyIngress)
		_ = c.DeleteSecret(authTLSSharedHostCASecretName)
		_ = c.DeleteSecret(authTLSSharedHostOtherCASecretName)
		_ = c.DeleteSecret(authTLSSharedHostTLSSecretName)
	}
}

// deployIngress deploys an Ingress declaring a single path on <host>.<cluster>.local to both clusters.
func (s *AuthTLSSharedHostSuite) deployIngress(name, host, path, tlsSecret string, annotations map[string]string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		err := c.DeployIngressWith(ingressTemplateData{
			Name:        name,
			Host:        host + "." + c.Name + ".local",
			Path:        path,
			Annotations: annotations,
			TLSSecret:   tlsSecret,
		})
		require.NoError(s.T(), err, "deploy %s ingress to %s cluster", name, c.Name)
	}
}

// deployIngressWithoutPaths deploys an Ingress declaring a rule without paths on <host>.<cluster>.local to both clusters.
func (s *AuthTLSSharedHostSuite) deployIngressWithoutPaths(name, host string, annotations map[string]string) {
	s.T().Helper()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		var manifest strings.Builder
		fmt.Fprintf(&manifest, "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: %s\n  annotations:\n", c.IngressName(name))
		for key, value := range annotations {
			fmt.Fprintf(&manifest, "    %s: %q\n", key, value)
		}
		fmt.Fprintf(&manifest, "spec:\n  ingressClassName: nginx\n  rules:\n  - host: %q\n", host+"."+c.Name+".local")

		require.NoError(s.T(), c.ApplyManifest(manifest.String()), "deploy %s ingress to %s cluster", name, c.Name)
	}
}

// waitForPath waits until the controller routes the given path, over HTTP so that it does not depend on client certificates.
// Like WaitForIngressReady, it fails the suite when the path never starts routing, as the assertions would then prove nothing.
func (s *AuthTLSSharedHostSuite) waitForPath(c *Cluster, host, path string) {
	s.T().Helper()

	for range 30 {
		resp := c.MakeRequest(s.T(), host+"."+c.Name+".local", http.MethodGet, path, nil, 1, time.Second)
		if resp != nil && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadGateway {
			return
		}
		time.Sleep(time.Second)
	}

	s.T().Fatalf("[%s] path %s on host %s not ready after 30 retries", c.Name, path, host)
}

// request sends the same HTTPS request to both controllers.
func (s *AuthTLSSharedHostSuite) request(host, path string, clientCert *tls.Certificate) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeTLSRequest(s.T(), host+".traefik.local", http.MethodGet, path, nil, clientCert, 3, time.Second)
	nginxResp = s.nginx.MakeTLSRequest(s.T(), host+".nginx.local", http.MethodGet, path, nil, clientCert, 3, time.Second)

	return traefikResp, nginxResp
}

// requestSSLClientHeaders sends the same HTTPS request to both controllers, requires both of them to serve it,
// and returns the Ssl-Client-* headers each backend received.
func (s *AuthTLSSharedHostSuite) requestSSLClientHeaders(host, path string, headers map[string]string, clientCert *tls.Certificate) (traefikHeaders, nginxHeaders map[string]string) {
	s.T().Helper()

	return requestSSLClientHeaders(s.T(), s.traefik, s.nginx, host+".traefik.local", host+".nginx.local", path, headers, clientCert)
}

// assertClientCertRejected asserts that both controllers refused the request.
// Traefik rejects the TLS handshake (nil response), while ingress-nginx answers with a 400.
func (s *AuthTLSSharedHostSuite) assertClientCertRejected(traefikResp, nginxResp *Response, path string) {
	s.T().Helper()

	if nginxResp != nil {
		assert.Contains(s.T(), []int{http.StatusBadRequest, http.StatusForbidden}, nginxResp.StatusCode,
			"nginx: expected %s to be rejected without a valid client certificate", path)
	}

	if traefikResp != nil {
		assert.Contains(s.T(), []int{http.StatusBadRequest, http.StatusForbidden}, traefikResp.StatusCode,
			"traefik: expected %s to be rejected without a valid client certificate, got %d (body: %s)", path, traefikResp.StatusCode, traefikResp.Body)
	}
}

// assertServed asserts that both controllers served the request.
func (s *AuthTLSSharedHostSuite) assertServed(traefikResp, nginxResp *Response, path string) {
	s.T().Helper()

	require.NotNil(s.T(), traefikResp, "traefik: %s should be served with a valid client certificate", path)
	require.NotNil(s.T(), nginxResp, "nginx: %s should be served with a valid client certificate", path)

	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: expected 200 on %s with a valid client certificate", path)
	assert.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: expected 200 on %s with a valid client certificate", path)
}

// TestAnnotatedPathStaysProtected checks that adding an Ingress without auth-tls annotations on a protected host
// does not disable client certificate authentication on the protected path.
func (s *AuthTLSSharedHostSuite) TestAnnotatedPathStaysProtected() {
	traefikResp, nginxResp := s.request(authTLSMixedHost, "/protected", nil)
	s.assertClientCertRejected(traefikResp, nginxResp, "/protected")

	traefikResp, nginxResp = s.request(authTLSMixedHost, "/protected", &s.certs.invalidClientCert)
	s.assertClientCertRejected(traefikResp, nginxResp, "/protected")

	traefikResp, nginxResp = s.request(authTLSMixedHost, "/protected", &s.certs.clientCert)
	s.assertServed(traefikResp, nginxResp, "/protected")
}

// TestUnannotatedPathRequiresClientCert checks that a path declared by an Ingress without auth-tls annotations
// requires a client certificate when another Ingress configures client certificate authentication for the host.
func (s *AuthTLSSharedHostSuite) TestUnannotatedPathRequiresClientCert() {
	traefikResp, nginxResp := s.request(authTLSMixedHost, "/public", nil)
	s.assertClientCertRejected(traefikResp, nginxResp, "/public")

	traefikResp, nginxResp = s.request(authTLSMixedHost, "/public", &s.certs.clientCert)
	s.assertServed(traefikResp, nginxResp, "/public")
}

// TestFirstIngressClientAuthWins checks that, when Ingresses on the same host declare different auth-tls-verify-client values,
// the configuration of the first Ingress applies to the whole host.
func (s *AuthTLSSharedHostSuite) TestFirstIngressClientAuthWins() {
	for _, path := range []string{"/required", "/optional"} {
		traefikResp, nginxResp := s.request(authTLSMixedModesHost, path, nil)
		s.assertClientCertRejected(traefikResp, nginxResp, path)

		traefikResp, nginxResp = s.request(authTLSMixedModesHost, path, &s.certs.clientCert)
		s.assertServed(traefikResp, nginxResp, path)
	}
}

// TestOptionalNoCAOwnerReportsUntrustedClientCert checks a host whose first Ingress requests a client certificate without
// verifying it during the handshake (optional_no_ca), which then applies to the whole host. A certificate the CA did not
// sign gets through the handshake on every path, including the one of an Ingress requiring a verified certificate, and
// Ssl-Client-Verify reports that it failed verification.
// Nginx and Traefik word the failure reason differently (OpenSSL and Go errors), so only the outcome is compared.
func (s *AuthTLSSharedHostSuite) TestOptionalNoCAOwnerReportsUntrustedClientCert() {
	for _, path := range []string{"/owner", "/member"} {
		traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSNoCAHost, path, nil, &s.selfSignedClientCert)

		for name, headers := range map[string]map[string]string{"traefik": traefikHeaders, "nginx": nginxHeaders} {
			assert.True(s.T(), strings.HasPrefix(headers["Ssl-Client-Verify"], "FAILED:"), "%s: %s: Ssl-Client-Verify is %q", name, path, headers["Ssl-Client-Verify"])
			assert.Equal(s.T(), s.selfSignedClientCert.Leaf.Subject.String(), headers["Ssl-Client-Subject-Dn"], "%s: %s", name, path)
		}
	}
}

// TestOptionalNoCAOwnerReportsTrustedClientCert checks that a certificate signed by the CA is reported as verified on every
// path of that host.
func (s *AuthTLSSharedHostSuite) TestOptionalNoCAOwnerReportsTrustedClientCert() {
	for _, path := range []string{"/owner", "/member"} {
		traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSNoCAHost, path, nil, &s.certs.clientCert)

		assert.Equal(s.T(), "SUCCESS", nginxHeaders["Ssl-Client-Verify"], "nginx: %s", path)
		assert.Equal(s.T(), "SUCCESS", traefikHeaders["Ssl-Client-Verify"], "traefik: %s", path)
	}
}

// TestUnannotatedPathSetsCertHeaders checks that a path declared by an Ingress without auth-tls annotations, on a host
// configured for client certificate authentication, sets the Ssl-Client-* headers from the certificate the client presented.
// Nginx sets them on every location of the server block, overwriting the values the client sent.
func (s *AuthTLSSharedHostSuite) TestUnannotatedPathSetsCertHeaders() {
	traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSMixedHost, "/public", clientSentVerificationHeaders(), &s.certs.clientCert)

	assert.Equal(s.T(), verifiedSSLClientHeaders(s.certs.clientCert), nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")
}

// TestUnannotatedPlaintextPathSetsNoCertHeaders checks the same over plaintext HTTP, on a path disabling the HTTPS
// redirection. Nginx sends none of the Ssl-Client-* headers there, as the variables it builds them from are empty outside
// of a TLS connection.
func (s *AuthTLSSharedHostSuite) TestUnannotatedPlaintextPathSetsNoCertHeaders() {
	headers := clientSentVerificationHeaders()

	traefikResp := s.traefik.MakeRequest(s.T(), authTLSMixedHost+".traefik.local", http.MethodGet, "/plaintext", headers, 3, time.Second)
	require.NotNil(s.T(), traefikResp)
	nginxResp := s.nginx.MakeRequest(s.T(), authTLSMixedHost+".nginx.local", http.MethodGet, "/plaintext", headers, 3, time.Second)
	require.NotNil(s.T(), nginxResp)

	require.Equal(s.T(), http.StatusOK, traefikResp.StatusCode, "traefik: body: %s", traefikResp.Body)
	require.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "nginx: body: %s", nginxResp.Body)

	assert.Empty(s.T(), sslClientHeadersReaching(nginxResp.RequestHeaders), "nginx")
	assert.Empty(s.T(), sslClientHeadersReaching(traefikResp.RequestHeaders), "traefik")
}

// TestFirstIngressCAWins checks that, when Ingresses on the same host declare different CA secrets, the CA of the first
// Ingress verifies client certificates on every path of the host.
func (s *AuthTLSSharedHostSuite) TestFirstIngressCAWins() {
	traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSOtherCAHost, "/member", nil, &s.certs.clientCert)

	assert.Equal(s.T(), verifiedSSLClientHeaders(s.certs.clientCert), nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")

	traefikResp, nginxResp := s.request(authTLSOtherCAHost, "/member", &s.otherCerts.clientCert)
	s.assertClientCertRejected(traefikResp, nginxResp, "/member")
}

// TestFirstIngressPassCertWins checks that the client certificate (Ssl-Client-Cert) is passed to the upstream on every path
// of a host or on none of them, following auth-tls-pass-certificate-to-upstream on the first Ingress, which configures
// client certificate authentication for the host, whatever the annotation of the Ingress serving the path.
func (s *AuthTLSSharedHostSuite) TestFirstIngressPassCertWins() {
	// Only the second Ingress passes the certificate.
	traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSNoCAHost, "/member", nil, &s.certs.clientCert)

	assert.NotContains(s.T(), nginxHeaders, "Ssl-Client-Cert", "nginx")
	assert.NotContains(s.T(), traefikHeaders, "Ssl-Client-Cert", "traefik")

	// Only the first Ingress passes the certificate.
	traefikHeaders, nginxHeaders = s.requestSSLClientHeaders(authTLSMixedModesHost, "/optional", nil, &s.certs.clientCert)

	assert.NotEmpty(s.T(), nginxHeaders["Ssl-Client-Cert"], "nginx")
	assert.NotEmpty(s.T(), traefikHeaders["Ssl-Client-Cert"], "traefik")
}

// TestRuleWithoutPathsDoesNotConfigureClientAuth checks that auth-tls annotations on an Ingress whose rule declares no paths
// do not configure client certificate authentication for the host, which serves the path of another Ingress without it.
func (s *AuthTLSSharedHostSuite) TestRuleWithoutPathsDoesNotConfigureClientAuth() {
	traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSNoPathsHost, "/path", nil, nil)

	assert.Empty(s.T(), nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")
}

// TestMissingSecretForbidsWholeHost checks that an Ingress declaring an auth-tls-secret that does not exist makes the whole
// host answer 403, on the paths of every Ingress of the host and on the paths no Ingress declares, over HTTPS and HTTP,
// whatever the client certificate.
func (s *AuthTLSSharedHostSuite) TestMissingSecretForbidsWholeHost() {
	for _, path := range []string{"/broken", "/public", "/undeclared"} {
		for _, clientCert := range []*tls.Certificate{nil, &s.certs.clientCert} {
			traefikResp, nginxResp := s.request(authTLSMissingSecretHost, path, clientCert)
			require.NotNil(s.T(), traefikResp, "traefik: %s", path)
			require.NotNil(s.T(), nginxResp, "nginx: %s", path)

			assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode, "nginx: HTTPS %s (client certificate: %v)", path, clientCert != nil)
			assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "traefik: HTTPS %s (client certificate: %v)", path, clientCert != nil)
		}

		traefikResp := s.traefik.MakeRequest(s.T(), authTLSMissingSecretHost+".traefik.local", http.MethodGet, path, nil, 3, time.Second)
		require.NotNil(s.T(), traefikResp, "traefik: %s", path)
		nginxResp := s.nginx.MakeRequest(s.T(), authTLSMissingSecretHost+".nginx.local", http.MethodGet, path, nil, 3, time.Second)
		require.NotNil(s.T(), nginxResp, "nginx: %s", path)

		assert.Equal(s.T(), http.StatusForbidden, nginxResp.StatusCode, "nginx: HTTP %s", path)
		assert.Equal(s.T(), http.StatusForbidden, traefikResp.StatusCode, "traefik: HTTP %s", path)
	}
}

// TestSecretWithoutCALeavesClientAuthToNextIngress checks that an Ingress declaring an auth-tls-secret without CA certificate
// is served, and that the client certificate authentication of the host comes from the next Ingress declaring one,
// including whether the certificate is passed to the upstream.
func (s *AuthTLSSharedHostSuite) TestSecretWithoutCALeavesClientAuthToNextIngress() {
	for _, path := range []string{"/without-ca", "/owner"} {
		traefikResp, nginxResp := s.request(authTLSWithoutCAHost, path, nil)
		s.assertClientCertRejected(traefikResp, nginxResp, path)

		traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSWithoutCAHost, path, nil, &s.certs.clientCert)

		assert.Equal(s.T(), verifiedSSLClientHeaders(s.certs.clientCert), nginxHeaders, "nginx: %s", path)
		assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik: %s", path)
	}
}

// TestSecretWithoutCAServesWithoutClientAuth checks that an Ingress declaring an auth-tls-secret without CA certificate,
// alone on its host, is served without client certificate authentication.
func (s *AuthTLSSharedHostSuite) TestSecretWithoutCAServesWithoutClientAuth() {
	traefikHeaders, nginxHeaders := s.requestSSLClientHeaders(authTLSWithoutCAOnlyHost, "/", nil, nil)

	assert.Empty(s.T(), nginxHeaders, "nginx")
	assert.Equal(s.T(), nginxHeaders, traefikHeaders, "traefik")
}
