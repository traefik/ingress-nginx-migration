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

	// TRAEFIK_IMAGE is optional. When set, the image is loaded into the k3s
	// cluster and the Helm chart is configured to use it. When empty, the
	// chart uses its default upstream image.
	traefikImage = os.Getenv("TRAEFIK_IMAGE")
	traefikImage = "traefik/traefik:v100.0.0"
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func initClusters() error {
	ctx := context.Background()

	fmt.Printf("Creating k3s clusters with image: %s\n", k3sImage)

	// Render the traefik helm chart, optionally configured with a custom image.
	traefikManifestPath, err := renderTraefikHelmChart(traefikImage)
	if err != nil {
		return fmt.Errorf("failed to render traefik helm chart: %v", err)
	}
	defer os.Remove(traefikManifestPath)

	// Create both k3s containers.
	traefikContainer, err := createCluster(ctx, traefikManifestPath)
	if err != nil {
		return fmt.Errorf("failed to create traefik cluster: %v", err)
	}

	// When a custom Traefik image is provided, load it into the k3s cluster
	// so the HelmChart uses it instead of pulling from the registry.
	if traefikImage != "" {
		fmt.Printf("Loading image %s into traefik cluster...\n", traefikImage)
		if err := traefikContainer.LoadImages(ctx, traefikImage); err != nil {
			return fmt.Errorf("failed to load traefik image: %v", err)
		}
	} else {
		fmt.Println("No TRAEFIK_IMAGE set, using chart default image")
	}

	nginxContainer, err := createCluster(ctx, filepath.Join(fixturesDir, "nginx-helmchart.yaml"))
	if err != nil {
		return fmt.Errorf("failed to create nginx cluster: %v", err)
	}

	// Build Cluster structs.
	sharedTraefik, err = newCluster("traefik", traefikContainer, "traefik", "app.kubernetes.io/name=traefik")
	if err != nil {
		return fmt.Errorf("failed to init traefik cluster: %v", err)
	}

	sharedNginx, err = newCluster("nginx", nginxContainer, "ingress-nginx", "app.kubernetes.io/name=ingress-nginx")
	if err != nil {
		return fmt.Errorf("failed to init nginx cluster: %v", err)
	}

	// Deploy nginx IngressClass to the traefik cluster so the kubernetesingressnginx
	// provider recognizes ingresses with ingressClassName: nginx.
	if err := sharedTraefik.ApplyFixture("nginx-ingressclass.yaml"); err != nil {
		return fmt.Errorf("failed to deploy nginx ingressclass to traefik cluster: %v", err)
	}

	fmt.Println("Waiting for ingress controllers to be ready...")

	// Wait for controllers to be ready.
	if err := waitForDeployment(sharedTraefik, "traefik", "traefik"); err != nil {
		return fmt.Errorf("traefik controller not ready: %v", err)
	}
	if err := waitForDeployment(sharedNginx, "ingress-nginx", "ingress-nginx-controller"); err != nil {
		return fmt.Errorf("nginx controller not ready: %v", err)
	}

	fmt.Println("Deploying shared resources...")

	// Deploy whoami backend to both.
	if err := sharedTraefik.DeploySharedResources(); err != nil {
		return fmt.Errorf("failed to deploy shared resources to traefik cluster: %v", err)
	}
	if err := sharedNginx.DeploySharedResources(); err != nil {
		return fmt.Errorf("failed to deploy shared resources to nginx cluster: %v", err)
	}

	// Wait for whoami pods to be ready.
	if err := waitForDeployment(sharedTraefik, testNamespace, "snippet-test-backend"); err != nil {
		return fmt.Errorf("whoami not ready in traefik cluster: %v", err)
	}
	if err := waitForDeployment(sharedNginx, testNamespace, "snippet-test-backend"); err != nil {
		return fmt.Errorf("whoami not ready in nginx cluster: %v", err)
	}

	fmt.Println("Clusters ready.")
	return nil
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
	Path        string
	PathType    string
	Annotations map[string]string
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
