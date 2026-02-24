package e2e

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/k3s"
)

// Cluster represents a k3s cluster with an ingress controller.
type Cluster struct {
	Name            string
	Container       *k3s.K3sContainer
	KubeconfigPath  string
	Host            string
	Port            string
	TestNamespace   string
	ControllerNS    string
	ControllerLabel string
}

// Response holds the parsed HTTP response from a cluster.
type Response struct {
	StatusCode      int
	Body            string
	ResponseHeaders http.Header
	RequestHeaders  map[string]string // parsed from whoami body
}

// Kubectl runs a kubectl command against this cluster.
func (c *Cluster) Kubectl(args ...string) error {
	fullArgs := append([]string{"--kubeconfig", c.KubeconfigPath}, args...)
	return runCommand("kubectl", fullArgs...)
}

// ApplyManifest applies a YAML manifest via stdin.
func (c *Cluster) ApplyManifest(manifest string) error {
	cmd := exec.Command("kubectl", "--kubeconfig", c.KubeconfigPath, "apply", "-f", "-", "-n", c.TestNamespace)
	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %v, output: %s", err, string(output))
	}
	return nil
}

// ApplyFixture applies a fixture file from the fixtures directory.
func (c *Cluster) ApplyFixture(filename string) error {
	fixturePath := filepath.Join(fixturesDir, filename)
	return c.Kubectl("apply", "-f", fixturePath, "-n", c.TestNamespace)
}

// DeployIngress deploys an ingress resource with the given annotations.
func (c *Cluster) DeployIngress(name, host string, annotations map[string]string) error {
	manifest, err := renderIngressManifest(name, host, annotations)
	if err != nil {
		return err
	}
	return c.ApplyManifest(manifest)
}

// DeleteIngress deletes an ingress resource.
func (c *Cluster) DeleteIngress(name string) error {
	return c.Kubectl("delete", "ingress", name, "-n", c.TestNamespace, "--ignore-not-found")
}

// DeploySharedResources deploys the whoami backend.
func (c *Cluster) DeploySharedResources() error {
	if err := c.ApplyFixture("deployment.yaml"); err != nil {
		return fmt.Errorf("failed to deploy deployment: %w", err)
	}
	if err := c.ApplyFixture("service.yaml"); err != nil {
		return fmt.Errorf("failed to deploy service: %w", err)
	}
	return nil
}

// CleanupSharedResources removes the whoami backend.
func (c *Cluster) CleanupSharedResources() {
	_ = c.Kubectl("delete", "deployment", "snippet-test-backend", "-n", c.TestNamespace, "--ignore-not-found")
	_ = c.Kubectl("delete", "service", "snippet-test-backend", "-n", c.TestNamespace, "--ignore-not-found")
}

// WaitForIngressReady waits until the ingress controller starts routing for the given host
// by polling GET / until a non-404/non-502 response is received.
func (c *Cluster) WaitForIngressReady(t *testing.T, host string, maxRetries int, delay time.Duration) {
	t.Helper()

	url := fmt.Sprintf("http://%s:%s/", c.Host, c.Port)
	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			time.Sleep(delay)
			continue
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(delay)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadGateway {
			return
		}
		time.Sleep(delay)
	}
	t.Logf("[%s] ingress for host %s not ready after %d retries", c.Name, host, maxRetries)
}

// MakeRequest makes an HTTP request to this cluster's ingress controller.
// Retries only on connection errors, not on HTTP status codes.
func (c *Cluster) MakeRequest(t *testing.T, host, method, path string, headers map[string]string, maxRetries int, delay time.Duration) *Response {
	t.Helper()

	url := fmt.Sprintf("http://%s:%s%s", c.Host, c.Port, path)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		req.Host = host
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}

		return &Response{
			StatusCode:      resp.StatusCode,
			Body:            string(body),
			ResponseHeaders: resp.Header,
			RequestHeaders:  parseWhoamiHeaders(string(body)),
		}
	}

	t.Logf("[%s] request failed after %d retries: %v", c.Name, maxRetries, lastErr)
	return nil
}

// GetIngressControllerLogs retrieves recent logs from the ingress controller.
func (c *Cluster) GetIngressControllerLogs(lines int) string {
	cmd := exec.Command("kubectl", "--kubeconfig", c.KubeconfigPath, "logs",
		"-l", c.ControllerLabel,
		"-n", c.ControllerNS,
		"--tail", fmt.Sprintf("%d", lines),
		"--all-containers=true",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("failed to get logs: %v\noutput: %s", err, string(output))
	}
	return string(output)
}
