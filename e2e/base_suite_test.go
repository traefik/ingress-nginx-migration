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
	"testing"
	"text/template"
	"time"

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

	sharedTraefik *Cluster
	sharedNginx   *Cluster
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	fixturesDir = filepath.Join(filepath.Dir(filename), "fixtures")

	flag.StringVar(&k3sImage, "k3s-image", "rancher/k3s:v1.31.4-k3s1", "K3s container image")
	flag.StringVar(&testNamespace, "namespace", "default", "Namespace for test resources")

	// TRAEFIK_IMAGE is optional. When set, the image is loaded into the k3s
	// cluster and the Helm chart is configured to use it. When empty, the
	// chart uses its default upstream image.
	traefikImage = os.Getenv("TRAEFIK_IMAGE")
}

func TestMain(m *testing.M) {
	flag.Parse()
	ctx := context.Background()

	fmt.Printf("Creating k3s clusters with image: %s\n", k3sImage)

	// Render the traefik helm chart, optionally configured with a custom image.
	traefikManifestPath, err := renderTraefikHelmChart(traefikImage)
	if err != nil {
		panic(fmt.Sprintf("failed to render traefik helm chart: %v", err))
	}
	defer os.Remove(traefikManifestPath)

	// Create both k3s containers.
	traefikContainer, err := createCluster(ctx, traefikManifestPath)
	if err != nil {
		panic(fmt.Sprintf("failed to create traefik cluster: %v", err))
	}

	// When a custom Traefik image is provided, load it into the k3s cluster
	// so the HelmChart uses it instead of pulling from the registry.
	if traefikImage != "" {
		fmt.Printf("Loading image %s into traefik cluster...\n", traefikImage)
		if err := traefikContainer.LoadImages(ctx, traefikImage); err != nil {
			panic(fmt.Sprintf("failed to load traefik image: %v", err))
		}
	} else {
		fmt.Println("No TRAEFIK_IMAGE set, using chart default image")
	}

	nginxContainer, err := createCluster(ctx, filepath.Join(fixturesDir, "nginx-helmchart.yaml"))
	if err != nil {
		panic(fmt.Sprintf("failed to create nginx cluster: %v", err))
	}

	// Build Cluster structs.
	sharedTraefik, err = newCluster("traefik", traefikContainer, "traefik", "app.kubernetes.io/name=traefik")
	if err != nil {
		panic(fmt.Sprintf("failed to init traefik cluster: %v", err))
	}

	sharedNginx, err = newCluster("nginx", nginxContainer, "ingress-nginx", "app.kubernetes.io/name=ingress-nginx")
	if err != nil {
		panic(fmt.Sprintf("failed to init nginx cluster: %v", err))
	}

	// Deploy nginx IngressClass to the traefik cluster so the kubernetesingressnginx
	// provider recognizes ingresses with ingressClassName: nginx.
	if err := sharedTraefik.ApplyFixture("nginx-ingressclass.yaml"); err != nil {
		panic(fmt.Sprintf("failed to deploy nginx ingressclass to traefik cluster: %v", err))
	}

	fmt.Println("Waiting for ingress controllers to be ready...")

	// Wait for controllers to be ready.
	if err := waitForDeployment(sharedTraefik, "traefik", "traefik"); err != nil {
		panic(fmt.Sprintf("traefik controller not ready: %v", err))
	}
	if err := waitForDeployment(sharedNginx, "ingress-nginx", "ingress-nginx-controller"); err != nil {
		panic(fmt.Sprintf("nginx controller not ready: %v", err))
	}

	fmt.Println("Deploying shared resources...")

	// Deploy whoami backend to both.
	if err := sharedTraefik.DeploySharedResources(); err != nil {
		panic(fmt.Sprintf("failed to deploy shared resources to traefik cluster: %v", err))
	}
	if err := sharedNginx.DeploySharedResources(); err != nil {
		panic(fmt.Sprintf("failed to deploy shared resources to nginx cluster: %v", err))
	}

	// Wait for whoami pods to be ready.
	if err := waitForDeployment(sharedTraefik, testNamespace, "snippet-test-backend"); err != nil {
		panic(fmt.Sprintf("whoami not ready in traefik cluster: %v", err))
	}
	if err := waitForDeployment(sharedNginx, testNamespace, "snippet-test-backend"); err != nil {
		panic(fmt.Sprintf("whoami not ready in nginx cluster: %v", err))
	}

	fmt.Println("Running tests...")
	code := m.Run()

	// Cleanup.
	sharedTraefik.CleanupSharedResources()
	sharedNginx.CleanupSharedResources()

	if err := traefikContainer.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate traefik container: %v\n", err)
	}
	if err := nginxContainer.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate nginx container: %v\n", err)
	}

	os.Exit(code)
}

func createCluster(ctx context.Context, manifestPath string) (*k3s.K3sContainer, error) {
	return k3s.Run(ctx, k3sImage,
		testcontainers.WithExposedPorts("80/tcp", "443/tcp"),
		k3s.WithManifest(manifestPath),
	)
}

func newCluster(name string, container *k3s.K3sContainer, controllerNS, controllerLabel string) (*Cluster, error) {
	ctx := context.Background()

	kubeconfig, err := container.GetKubeConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get kubeconfig: %w", err)
	}

	// Write kubeconfig to temp file.
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("kubeconfig-%s-*.yaml", name))
	fmt.Printf("Created temp kubeconfig file: %s\n", tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("create temp kubeconfig: %w", err)
	}
	if _, err := tmpFile.Write(kubeconfig); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("write kubeconfig: %w", err)
	}
	tmpFile.Close()

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "80/tcp")
	if err != nil {
		return nil, fmt.Errorf("get mapped port: %w", err)
	}

	return &Cluster{
		Name:            name,
		Container:       container,
		KubeconfigPath:  tmpFile.Name(),
		Host:            host,
		Port:            port.Port(),
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

type ingressTemplateData struct {
	Name        string
	Host        string
	Annotations map[string]string
}

func renderIngressManifest(name, host string, annotations map[string]string) (string, error) {
	tmplPath := filepath.Join(fixturesDir, "ingress.yaml.tmpl")
	tmplContent, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read ingress template: %w", err)
	}

	tmpl, err := template.New("ingress").Funcs(template.FuncMap{
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
	}).Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse ingress template: %w", err)
	}

	// Clean annotation values.
	cleaned := make(map[string]string, len(annotations))
	for k, v := range annotations {
		cleaned[k] = strings.ReplaceAll(strings.TrimSpace(v), "\t", "  ")
	}

	data := ingressTemplateData{
		Name:        name,
		Host:        host,
		Annotations: cleaned,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute ingress template: %w", err)
	}

	return buf.String(), nil
}

// BaseSuite provides shared infrastructure for all e2e test suites.
type BaseSuite struct {
	suite.Suite
	traefik *Cluster
	nginx   *Cluster
}

func (s *BaseSuite) SetupSuite() {
	s.traefik = sharedTraefik
	s.nginx = sharedNginx
}

// makeComparisonRequests deploys an ingress to both clusters, makes requests, and returns both responses.
func (s *BaseSuite) makeComparisonRequests(ingressName string, annotations map[string]string, method, path string, reqHeaders map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikHost := ingressName + ".traefik.local"
	nginxHost := ingressName + ".nginx.local"

	// Deploy ingress to both clusters.
	err := s.traefik.DeployIngress(ingressName, traefikHost, annotations)
	require.NoError(s.T(), err, "deploy ingress to traefik cluster")

	err = s.nginx.DeployIngress(ingressName, nginxHost, annotations)
	require.NoError(s.T(), err, "deploy ingress to nginx cluster")

	s.T().Cleanup(func() {
		_ = s.traefik.DeleteIngress(ingressName)
		_ = s.nginx.DeleteIngress(ingressName)
	})

	// Wait for ingress to be configured in both clusters.
	s.traefik.WaitForIngressReady(s.T(), traefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), nginxHost, 20, 1*time.Second)

	// Make actual test requests (retries only on connection errors).
	traefikResp = s.traefik.MakeRequest(s.T(), traefikHost, method, path, reqHeaders, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), nginxHost, method, path, reqHeaders, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}
