package e2e

import (
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const samePathRegexPath = "/p"

// samePathRegexCase declares Exact and Prefix samePathRegexPath in one Ingress, as the admission
// webhook rejects the same host and path across Ingresses. Exact routes to backend and Prefix
// to canary-backend, so the response tells which location served the request.
type samePathRegexCase struct {
	name string
	// ownRegex sets use-regex on the Ingress declaring both paths, otherwise another Ingress of
	// the host sets it.
	ownRegex   bool
	exactFirst bool
	// prefixServed lists the samePathRegexRequests served by the Prefix location, the others by the Exact one.
	prefixServed []string
}

var samePathRegexRequests = []string{samePathRegexPath, samePathRegexPath + "/child", "/P", "/pfoo"}

// ingress-nginx sorts locations before appending a slash to a Prefix path shared with an Exact one,
// so the two locations keep their definition order and the first matching regex serves a request.
var samePathRegexCases = []samePathRegexCase{
	{name: "own-regex-exact-first", ownRegex: true, exactFirst: true},
	{name: "own-regex-prefix-first", ownRegex: true, prefixServed: samePathRegexRequests},
	{name: "host-regex-exact-first", exactFirst: true},
	{name: "host-regex-prefix-first", prefixServed: []string{samePathRegexPath + "/child"}},
}

func (sc samePathRegexCase) ingressName() string { return "same-path-" + sc.name }

func (sc samePathRegexCase) host(c *Cluster) string {
	return "same-path-" + sc.name + "." + c.Name + ".local"
}

func (sc samePathRegexCase) manifests(c *Cluster) []string {
	exact := pathEntry(samePathRegexPath, "Exact", "backend")
	prefix := pathEntry(samePathRegexPath, "Prefix", "canary-backend")
	paths := prefix + exact
	if sc.exactFirst {
		paths = exact + prefix
	}

	annotations := map[string]string{}
	if sc.ownRegex {
		annotations["nginx.ingress.kubernetes.io/use-regex"] = "true"
	}
	manifests := []string{ingressManifest(c.IngressName(sc.ingressName()), sc.host(c), annotations, paths)}

	if !sc.ownRegex {
		manifests = append(manifests, ingressManifest(c.IngressName(sc.ingressName()+"-regex"), sc.host(c),
			map[string]string{"nginx.ingress.kubernetes.io/use-regex": "true"}, pathEntry("/regex", "Prefix", "backend")))
	}
	return manifests
}

func pathEntry(path, pathType, service string) string {
	return fmt.Sprintf("      - path: %s\n        pathType: %s\n        backend:\n          service:\n            name: %s\n            port:\n              number: 80\n", path, pathType, service)
}

func ingressManifest(name, host string, annotations map[string]string, paths string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: %s\n", name)
	if len(annotations) > 0 {
		b.WriteString("  annotations:\n")
		for k, v := range annotations {
			fmt.Fprintf(&b, "    %s: %q\n", k, v)
		}
	}
	fmt.Fprintf(&b, "spec:\n  ingressClassName: nginx\n  rules:\n  - host: %q\n    http:\n      paths:\n%s", host, paths)
	return b.String()
}

// SamePathRegexSuite checks which location serves an Exact and a Prefix path sharing a path on a
// host using use-regex.
type SamePathRegexSuite struct {
	BaseSuite
}

func TestSamePathRegexSuite(t *testing.T) { //nolint:paralleltest // Suites share cluster resources and must run sequentially.
	suite.Run(t, new(SamePathRegexSuite))
}

func (s *SamePathRegexSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	for _, c := range []*Cluster{s.traefik, s.nginx} {
		s.Require().NoError(c.ApplyFixture("canary-backend.yaml"), "deploy canary-backend fixture to %s", c.Name)
		s.Require().NoError(waitForDeployment(c, testNamespace, "canary-backend"), "canary-backend deployment not ready on %s", c.Name)

		for _, sc := range samePathRegexCases {
			for _, manifest := range sc.manifests(c) {
				s.Require().NoError(c.ApplyManifest(manifest), "deploy %s ingresses to %s cluster", sc.name, c.Name)
			}

			// Without the use-regex Ingress, the host is not in regex mode yet.
			readyPath := samePathRegexPath
			if !sc.ownRegex {
				readyPath = "/regex"
			}
			s.Require().Eventually(func() bool {
				resp := c.MakeRequest(s.T(), sc.host(c), http.MethodGet, readyPath, nil, 1, time.Second)
				return resp != nil && resp.StatusCode == http.StatusOK
			}, 30*time.Second, time.Second, "[%s] %s ingresses did not become ready", c.Name, sc.name)
		}
	}
}

func (s *SamePathRegexSuite) TearDownSuite() {
	for _, c := range []*Cluster{s.traefik, s.nginx} {
		if c == nil {
			continue
		}
		if s.T().Failed() {
			s.T().Log(c.GetIngressControllerLogs(500))
		}
		for _, sc := range samePathRegexCases {
			_ = c.DeleteIngress(sc.ingressName())
			_ = c.DeleteIngress(sc.ingressName() + "-regex")
		}
		_ = c.Kubectl("delete", "-f", filepath.Join(fixturesDir, "canary-backend.yaml"), "-n", testNamespace, "--ignore-not-found")
	}
}

func (s *SamePathRegexSuite) TestServingLocation() {
	for _, sc := range samePathRegexCases {
		s.Run(sc.name, func() {
			for _, path := range samePathRegexRequests {
				expected := "exact"
				if slices.Contains(sc.prefixServed, path) {
					expected = "prefix"
				}
				s.Run(path, func() {
					for _, c := range []*Cluster{s.nginx, s.traefik} {
						s.Run(c.Name, func() {
							resp := c.MakeRequest(s.T(), sc.host(c), http.MethodGet, path, nil, 3, time.Second)
							s.Require().NotNil(resp, "request to %s returned no response", path)
							s.Require().Equal(http.StatusOK, resp.StatusCode, "unexpected status for %s", path)
							s.Equal(expected, servingLocation(resp), "unexpected location for %s", path)
						})
					}
				})
			}
		})
	}
}

func servingLocation(resp *Response) string {
	switch hostname := resp.RequestHeaders["Hostname"]; {
	case strings.HasPrefix(hostname, "canary-backend"):
		return "prefix"
	case strings.HasPrefix(hostname, "backend"):
		return "exact"
	default:
		return hostname
	}
}
