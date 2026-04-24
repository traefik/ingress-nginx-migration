package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
)

type traefikHelmChartData struct {
	HasImage   bool
	Registry   string
	Repository string
	Tag        string
}

var (
	k3sImage      string
	traefikImage  string
	testNamespace string
	fixturesDir   string
	reuseCluster  = os.Getenv("E2E_REUSE_CLUSTER") == "true"

	clusterOnce    sync.Once
	clusterInitErr error
	sharedTraefik  *Cluster
	sharedNginx    *Cluster
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	fixturesDir = filepath.Join(filepath.Dir(filename), "fixtures")

	flag.StringVar(&k3sImage, "k3s-image", "rancher/k3s:v1.31.4-k3s1", "K3s container image")
	flag.StringVar(&testNamespace, "namespace", "default", "Namespace for test resources")

	// TRAEFIK_IMAGE must be set to a Traefik image that includes the
	// kubernetesIngressNginx provider. The image is loaded from the local
	// Docker daemon into the k3s cluster.
	traefikImage = os.Getenv("TRAEFIK_IMAGE")
}

func TestMain(m *testing.M) {
	flag.Parse()

	if traefikImage == "" {
		fmt.Fprintln(os.Stderr, "TRAEFIK_IMAGE is required (e.g. TRAEFIK_IMAGE=traefik/traefik:v100.0.0)")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func initClusters() error {
	ctx := context.Background()

	opts := []testcontainers.ContainerCustomizer{
		testcontainers.WithExposedPorts("80/tcp", "443/tcp", "30080/tcp", "30443/tcp"),
		testcontainers.WithName("cluster"),
		k3s.WithManifest(filepath.Join(fixturesDir, "nginx-helmchart.yaml")),
	}

	if reuseCluster {
		// Prevent Ryuk from cleaning up the container on process exit.
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
		opts = append(opts, testcontainers.WithReuseByName("e2e-k3s-cluster"))
		fmt.Println("Cluster reuse enabled (E2E_REUSE_CLUSTER=true)")
	}

	fmt.Printf("Creating k3s cluster with image: %s\n", k3sImage)

	container, err := k3s.Run(ctx, k3sImage, opts...)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %v", err)
	}

	// Write kubeconfig (always needed — temp file is ephemeral across runs).
	kubeconfig, err := container.GetKubeConfig(ctx)
	if err != nil {
		return fmt.Errorf("get kubeconfig: %v", err)
	}
	kubeconfigPath, err := writeKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("write kubeconfig: %v", err)
	}
	fmt.Printf("Created temp kubeconfig file: %s\n", kubeconfigPath)

	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("get container host: %v", err)
	}

	// Build Cluster structs (same container, different ports).
	sharedTraefik, err = newCluster("traefik", container, kubeconfigPath, host, "traefik", "app.kubernetes.io/name=traefik", "80/tcp", "443/tcp")
	if err != nil {
		return fmt.Errorf("failed to init traefik cluster: %v", err)
	}

	sharedNginx, err = newCluster("nginx", container, kubeconfigPath, host, "ingress-nginx", "app.kubernetes.io/name=ingress-nginx", "30080/tcp", "30443/tcp")
	if err != nil {
		return fmt.Errorf("failed to init nginx cluster: %v", err)
	}

	// If reusing an already-provisioned cluster, skip setup.
	if reuseCluster && isClusterReady(sharedTraefik) {
		fmt.Println("Reusing existing cluster — skipping setup.")
		fmt.Println("Note: Traefik image is NOT reloaded on reuse. Run `docker rm -f e2e-k3s-cluster` to force a fresh cluster.")
		return nil
	}

	// Fresh cluster: load image, deploy charts, wait for readiness.
	fmt.Printf("Loading image %s into cluster...\n", traefikImage)
	if err := container.LoadImages(ctx, traefikImage); err != nil {
		return fmt.Errorf("failed to load traefik image: %v", err)
	}

	// Render and apply the traefik helm chart after the image is loaded,
	// so k3s can find the image when the HelmChart controller starts.
	traefikManifestPath, err := renderTraefikHelmChart(traefikImage)
	if err != nil {
		return fmt.Errorf("failed to render traefik helm chart: %v", err)
	}
	defer os.Remove(traefikManifestPath)

	traefikManifest, err := os.ReadFile(traefikManifestPath)
	if err != nil {
		return fmt.Errorf("failed to read traefik manifest: %v", err)
	}

	if err := container.CopyToContainer(ctx, traefikManifest, "/var/lib/rancher/k3s/server/manifests/traefik-helmchart.yaml", 0o644); err != nil {
		return fmt.Errorf("failed to copy traefik manifest: %v", err)
	}

	fmt.Println("Waiting for ingress controllers to be ready...")

	// Wait for both controllers to be ready.
	if err := waitForDeployment(sharedTraefik, "traefik", "traefik"); err != nil {
		return fmt.Errorf("traefik controller not ready: %v", err)
	}
	if err := waitForDeployment(sharedNginx, "ingress-nginx", "ingress-nginx-controller"); err != nil {
		return fmt.Errorf("nginx controller not ready: %v", err)
	}

	fmt.Println("Deploying shared resources...")

	// Deploy whoami backend (single cluster, deploy once).
	if err := sharedTraefik.DeploySharedResources(); err != nil {
		return fmt.Errorf("failed to deploy shared resources: %v", err)
	}

	// Wait for whoami pods to be ready.
	if err := waitForDeployment(sharedTraefik, testNamespace, "backend"); err != nil {
		return fmt.Errorf("whoami not ready: %v", err)
	}

	fmt.Println("Cluster ready.")
	return nil
}

// isClusterReady checks whether the cluster is already provisioned by
// verifying the traefik controller deployment exists.
func isClusterReady(c *Cluster) bool {
	return c.Kubectl("get", "deployment", "traefik",
		"-n", "traefik", "-o", "name") == nil
}

func combineManifests(paths ...string) (string, error) {
	var parts []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read manifest %s: %w", p, err)
		}
		parts = append(parts, string(data))
	}

	tmpFile, err := os.CreateTemp("", "combined-manifests-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := tmpFile.WriteString(strings.Join(parts, "\n---\n")); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}

func writeKubeconfig(kubeconfig []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp kubeconfig: %w", err)
	}
	if _, err := tmpFile.Write(kubeconfig); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write kubeconfig: %w", err)
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}

func newCluster(name string, container *k3s.K3sContainer, kubeconfigPath, host, controllerNS, controllerLabel, httpPort, httpsPort string) (*Cluster, error) {
	ctx := context.Background()

	port, err := container.MappedPort(ctx, nat.Port(httpPort))
	if err != nil {
		return nil, fmt.Errorf("get mapped HTTP port: %w", err)
	}

	portHTTPS, err := container.MappedPort(ctx, nat.Port(httpsPort))
	if err != nil {
		return nil, fmt.Errorf("get mapped HTTPS port: %w", err)
	}

	return &Cluster{
		Name:            name,
		Container:       container,
		KubeconfigPath:  kubeconfigPath,
		Host:            host,
		Port:            port.Port(),
		PortHTTPS:       portHTTPS.Port(),
		TestNamespace:   testNamespace,
		ControllerNS:    controllerNS,
		ControllerLabel: controllerLabel,
	}, nil
}

func waitForDeployment(c *Cluster, namespace, deploymentName string) error {
	// Retry for up to 5 minutes.
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		err := c.Kubectl("wait", "deployment", deploymentName,
			"-n", namespace,
			"--for=condition=Available",
			"--timeout=10s",
		)
		if err == nil {
			return nil
		}
		fmt.Printf("[%s] Waiting for deployment %s/%s...\n", c.Name, namespace, deploymentName)
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("deployment %s/%s not ready after 5 minutes", namespace, deploymentName)
}

// parseImageRef splits a Docker image reference into registry, repository, and tag.
func parseImageRef(image string) (registry, repository, tag string) {
	ref := image
	tag = "latest"
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		tag = ref[idx+1:]
		ref = ref[:idx]
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repository = parts[1]
	} else {
		registry = "docker.io"
		repository = ref
	}
	return
}

// renderTraefikHelmChart renders the traefik helm chart template. When image
// is non-empty, the chart is configured to use that specific image. Returns
// the path to a temp file containing the rendered manifest.
func renderTraefikHelmChart(image string) (string, error) {
	tmplPath := filepath.Join(fixturesDir, "traefik-helmchart.yaml.tmpl")
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read traefik helmchart template: %w", err)
	}

	tmpl, err := template.New("traefik-helmchart").Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse traefik helmchart template: %w", err)
	}

	data := traefikHelmChartData{}
	if image != "" {
		registry, repository, tag := parseImageRef(image)
		data = traefikHelmChartData{
			HasImage:   true,
			Registry:   registry,
			Repository: repository,
			Tag:        tag,
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute traefik helmchart template: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "traefik-helmchart-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := tmpFile.Write(buf.Bytes()); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	return tmpFile.Name(), nil
}

type ingressDefaultBackend struct {
	ServiceName string
	ServicePort int
}

type ingressTemplateData struct {
	Name            string
	Host            string
	Path            string
	PathType        string
	Annotations     map[string]string
	DefaultBackend  *ingressDefaultBackend
	ServiceName     string // default: "backend"
	ServicePort     int    // default: 80
	ServicePortName string // if non-empty, uses port name instead of number
	TLSSecret       string // if non-empty, adds spec.tls section
}

type nginxBackendTemplateData struct {
	Name          string
	ConfigMapName string
	TLSSecretName string
}

type secretTemplateData struct {
	Name string
	Type string
	Data map[string]string
}

type configMapTemplateData struct {
	Name string
	Data map[string]string
}

func renderManifest(templateFile string, data any) (string, error) {
	tmplPath := filepath.Join(fixturesDir, templateFile)
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templateFile, err)
	}

	tmpl, err := template.New(templateFile).Funcs(template.FuncMap{
		"indent": func(spaces int, s string) string {
			pad := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = pad + line
				}
			}
			return strings.Join(lines, "\n")
		},
		"multiline": func(s string) bool {
			return strings.Contains(s, "\n")
		},
		"quote": func(s string) string {
			return fmt.Sprintf("%q", s)
		},
	}).Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templateFile, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateFile, err)
	}

	return buf.String(), nil
}

func renderIngressManifest(data ingressTemplateData) (string, error) {
	if data.Path == "" {
		data.Path = "/"
	}
	if data.PathType == "" {
		data.PathType = "Prefix"
	}
	if data.ServiceName == "" {
		data.ServiceName = "backend"
	}
	if data.ServicePort == 0 {
		data.ServicePort = 80
	}

	// Clean annotation values.
	cleaned := make(map[string]string, len(data.Annotations))
	for k, v := range data.Annotations {
		cleaned[k] = strings.ReplaceAll(strings.TrimSpace(v), "\t", "  ")
	}
	data.Annotations = cleaned

	return renderManifest("ingress.yaml.tmpl", data)
}

func renderSecretManifest(data secretTemplateData) (string, error) {
	if data.Type == "" {
		data.Type = "Opaque"
	}
	return renderManifest("secret.yaml.tmpl", data)
}

func renderConfigMapManifest(data configMapTemplateData) (string, error) {
	return renderManifest("configmap.yaml.tmpl", data)
}

// BaseSuite provides shared infrastructure for all e2e test suites.
type BaseSuite struct {
	suite.Suite

	traefik *Cluster
	nginx   *Cluster
}

func (s *BaseSuite) SetupSuite() {
	clusterOnce.Do(func() {
		clusterInitErr = initClusters()
	})
	require.NoError(s.T(), clusterInitErr, "cluster initialization failed")

	s.traefik = sharedTraefik
	s.nginx = sharedNginx
}
