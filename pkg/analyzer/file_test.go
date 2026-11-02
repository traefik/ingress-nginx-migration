package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const listWithoutIngressClass = `apiVersion: v1
kind: List
items:
- apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    name: front
    namespace: team-a
    annotations:
      nginx.ingress.kubernetes.io/rewrite-target: /$2
  spec:
    ingressClassName: webapprouting.kubernetes.azure.com
- apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    name: back
    namespace: team-b
  spec:
    ingressClassName: webapprouting.kubernetes.azure.com
- apiVersion: networking.k8s.io/v1
  kind: Ingress
  metadata:
    name: other
    namespace: team-b
  spec:
    ingressClassName: traefik
metadata:
  resourceVersion: ""
`

const multiDocWithIngressClass = `---
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: nginx-internal
spec:
  controller: approuting.kubernetes.azure.com/nginx
---
apiVersion: v1
kind: Service
metadata:
  name: ignored
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: front
  namespace: team-a
spec:
  ingressClassName: nginx-internal
---
`

func writeInputFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "input.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestNewFromFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		content            string
		namespaces         []string
		controllerClass    string
		ingressClass       string
		wantIngressCount   int
		wantCountByClass   map[string]int
		wantSupportedCount int
	}{
		{
			name:               "list without IngressClass matches by ingress class name",
			content:            listWithoutIngressClass,
			ingressClass:       "webapprouting.kubernetes.azure.com",
			wantIngressCount:   2,
			wantCountByClass:   map[string]int{"webapprouting.kubernetes.azure.com": 2},
			wantSupportedCount: 1,
		},
		{
			name:               "namespaces filter the Ingresses",
			content:            listWithoutIngressClass,
			namespaces:         []string{"team-a"},
			ingressClass:       "webapprouting.kubernetes.azure.com",
			wantIngressCount:   1,
			wantCountByClass:   map[string]int{"webapprouting.kubernetes.azure.com": 1},
			wantSupportedCount: 1,
		},
		{
			name:             "list without IngressClass and default ingress class matches nothing",
			content:          listWithoutIngressClass,
			wantIngressCount: 0,
			wantCountByClass: map[string]int{},
		},
		{
			name:             "multi-document file with IngressClass matches by controller",
			content:          multiDocWithIngressClass,
			controllerClass:  "approuting.kubernetes.azure.com/nginx",
			wantIngressCount: 1,
			wantCountByClass: map[string]int{"nginx-internal": 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			a, err := NewFromFile(writeInputFile(t, test.content), test.namespaces, test.controllerClass, false, test.ingressClass, false)
			require.NoError(t, err)
			require.NoError(t, a.Start(t.Context()))
			require.NoError(t, a.GenerateReport())

			report := a.Report()
			assert.Equal(t, test.wantIngressCount, report.IngressCount)
			assert.Equal(t, test.wantCountByClass, report.IngressCountByClass)
			assert.Equal(t, test.wantSupportedCount, report.SupportedIngressCount)
		})
	}
}

func TestNewFromFile_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "unsupported Ingress apiVersion",
			content: `apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: old
`,
		},
		{
			name:    "malformed YAML",
			content: "kind: [Ingress\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewFromFile(writeInputFile(t, test.content), nil, "", false, "", false)
			assert.Error(t, err)
		})
	}

	_, err := NewFromFile(filepath.Join(t.TempDir(), "missing.yaml"), nil, "", false, "", false)
	assert.Error(t, err)
}
