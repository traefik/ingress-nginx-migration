package e2e

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
)

const (
	classNginxController   = "nginx"
	classTraefikController = "traefik"
	classProbeHeader       = "X-Class-Probe"
	classProbeValue        = "conformance"
)

// IngressClassSuite compares class selection with ingress-nginx when class lookup
// is forbidden by RBAC or the only readable class belongs to another controller.
// It uses isolated clusters because BaseSuite creates a matching NGINX IngressClass.
type IngressClassSuite struct {
	suite.Suite

	traefik *Cluster
	nginx   *Cluster
}

func TestIngressClassSuite(t *testing.T) {
	suite.Run(t, new(IngressClassSuite))
}

// TestRBACRestricted checks that forbidden class lookup preserves the
// watch-without-class fallback, even when the Ingress declares a class name.
func (s *IngressClassSuite) TestRBACRestricted() {
	s.startControllers(false)

	traefikResp, nginxResp := s.request()
	assert.Equal(s.T(), http.StatusOK, nginxResp.StatusCode, "ingress-nginx should use the watch-without-class fallback")
	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code must match ingress-nginx")
	assert.Equal(s.T(), classProbeValue, nginxResp.RequestHeaders[classProbeHeader], "nginx response must come from the backend")
	assert.Equal(s.T(), classProbeValue, traefikResp.RequestHeaders[classProbeHeader], "traefik response must come from the backend")
}

// TestForeignClass checks that an explicitly classed Ingress stays unrouted when
// its IngressClass belongs to another controller, despite watch-without-class.
func (s *IngressClassSuite) TestForeignClass() {
	s.startControllers(true)

	// A single startup 404 must not hide a route created by a later reconciliation.
	deadline := time.Now().Add(5 * time.Second)
	for {
		traefikResp, nginxResp := s.request()
		require.Equal(s.T(), http.StatusNotFound, nginxResp.StatusCode, "ingress-nginx must ignore the foreign class")
		require.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "foreign-class status must match ingress-nginx")
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// request sends the same request to both controllers after the classless
// positive control has established backend reachability in startControllers.
func (s *IngressClassSuite) request() (traefikResp, nginxResp *Response) {
	s.T().Helper()

	headers := map[string]string{classProbeHeader: classProbeValue}
	traefikResp = s.traefik.MakeRequest(s.T(), "foreign-class.example.test", http.MethodGet, "/", headers, 1, 0)
	nginxResp = s.nginx.MakeRequest(s.T(), "foreign-class.example.test", http.MethodGet, "/", headers, 1, 0)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

func (s *IngressClassSuite) startControllers(readIngressClasses bool) {
	t := s.T()
	t.Helper()

	// Class resources and RBAC are cluster-wide. Never mutate the shared annotation-test cluster.
	ctx := t.Context()
	container, err := k3s.Run(ctx, k3sImage, testcontainers.WithExposedPorts("30080/tcp", "30081/tcp"))
	if container != nil {
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			assert.NoError(t, container.Terminate(cleanupCtx))
		})
	}
	require.NoError(t, err)

	kubeconfig, err := container.GetKubeConfig(ctx)
	require.NoError(t, err)
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigPath, kubeconfig, 0o600))
	require.NoError(t, container.LoadImages(ctx, traefikImage))
	host, err := container.Host(ctx)
	require.NoError(t, err)

	var controllers []*Cluster
	for _, endpoint := range []struct {
		name string
		port nat.Port
	}{
		{name: classNginxController, port: "30080/tcp"},
		{name: classTraefikController, port: "30081/tcp"},
	} {
		port, portErr := container.MappedPort(ctx, endpoint.port)
		require.NoError(t, portErr)
		controllers = append(controllers, &Cluster{
			Name:            endpoint.name,
			Container:       container,
			KubeconfigPath:  kubeconfigPath,
			Host:            host,
			Port:            port.Port(),
			PortHTTPS:       "",
			TestNamespace:   testNamespace,
			ControllerNS:    testNamespace,
			ControllerLabel: "app=class-" + endpoint.name,
		})
	}
	t.Cleanup(func() {
		if t.Failed() {
			for _, controller := range controllers {
				t.Logf("[%s] controller logs:\n%s", controller.Name, controller.GetIngressControllerLogs(100))
			}
		}
	})

	data := struct {
		Namespace          string
		TraefikImage       string
		ReadIngressClasses bool
	}{testNamespace, traefikImage, readIngressClasses}
	resources, err := renderManifest("ingressclass/resources.yaml.tmpl", data)
	require.NoError(t, err)
	cluster := controllers[0]
	require.NoError(t, cluster.ApplyManifest(resources))
	require.NoError(t, cluster.ApplyFixture("deployment.yaml"))
	require.NoError(t, cluster.ApplyFixture("service.yaml"))
	require.NoError(t, waitForDeployment(cluster, testNamespace, "backend"))

	// Both controllers use this account. Check the real API result before either starts.
	exitCode, output := ingressClassKubectl(t, container, "--as=system:serviceaccount:"+testNamespace+":class-controller",
		"get", "ingressclasses", "-o", "jsonpath={range .items[*]}{.metadata.name}:{.spec.controller}{\"\\n\"}{end}")
	if readIngressClasses {
		require.Zero(t, exitCode, "%s", output)
		require.Equal(t, "traefik:traefik.io/ingress-controller\n", output, "there must be no matching NGINX IngressClass")
	} else {
		require.NotZero(t, exitCode)
		require.Contains(t, output, "Forbidden", "the API must reject IngressClass access")
	}

	// Default-class admission must not turn the positive control into an explicitly classed Ingress.
	exitCode, output = ingressClassKubectl(t, container, "get", "ingress", "classless", "-n", testNamespace, "-o", "jsonpath={.spec.ingressClassName}")
	require.Zero(t, exitCode, "%s", output)
	require.Empty(t, output)

	manifest, err := renderManifest("ingressclass/controllers.yaml.tmpl", data)
	require.NoError(t, err)
	require.NoError(t, cluster.ApplyManifest(manifest))
	for _, controller := range controllers {
		require.NoError(t, waitForDeployment(controller, testNamespace, "class-"+controller.Name))
	}
	s.nginx, s.traefik = controllers[0], controllers[1]
	for _, controller := range controllers {
		waitForClasslessBackend(t, controller, map[string]string{classProbeHeader: classProbeValue})
	}
}

func ingressClassKubectl(t *testing.T, container *k3s.K3sContainer, args ...string) (int, string) {
	t.Helper()

	exitCode, reader, err := container.Exec(t.Context(), append([]string{"kubectl"}, args...), exec.Multiplexed())
	require.NoError(t, err)
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	return exitCode, string(output)
}

func waitForClasslessBackend(t *testing.T, controller *Cluster, headers map[string]string) {
	t.Helper()

	var response *Response
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response = controller.MakeRequest(t, "classless.example.test", http.MethodGet, "/", headers, 1, 0)
		if response != nil && response.StatusCode == http.StatusOK && response.RequestHeaders[classProbeHeader] == classProbeValue {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("classless ingress did not reach the backend: last response=%+v", response)
}
