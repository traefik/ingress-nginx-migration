package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/ingress-nginx-migration/pkg/analyzer"
)

var update = flag.Bool("update", false, "update golden files")

// mixedReport has vanilla, supported and unsupported (known-unsupported + unknown)
// Ingresses, so every section of every renderer has content.
func mixedReport() analyzer.Report {
	return analyzer.Report{
		GenerationDate:               time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC),
		Version:                      "v0.3.0",
		Hash:                         "abc123def456",
		IngressCount:                 4,
		IngressCountByClass:          map[string]int{"nginx": 4},
		CompatibleIngressCount:       2,
		CompatibleIngressPercentage:  50.0,
		VanillaIngressCount:          1,
		VanillaIngressPercentage:     25.0,
		SupportedIngressCount:        1,
		SupportedIngressPercentage:   25.0,
		UnsupportedIngressCount:      2,
		UnsupportedIngressPercentage: 50.0,
		UnsupportedIngressAnnotations: map[string]int{
			"nginx.ingress.kubernetes.io/limit-connections": 2,
		},
		UnknownIngressAnnotations: map[string]int{
			"nginx.ingress.kubernetes.io/totally-made-up": 1,
		},
		UnsupportedIngresses: []analyzer.IngressReport{
			{
				Name:                   "api",
				Namespace:              "prod",
				IngressClassName:       "nginx",
				UnsupportedAnnotations: []string{"nginx.ingress.kubernetes.io/limit-connections"},
			},
			{
				Name:                   "web",
				Namespace:              "prod",
				IngressClassName:       "nginx",
				UnsupportedAnnotations: []string{"nginx.ingress.kubernetes.io/limit-connections"},
				UnknownAnnotations:     []string{"nginx.ingress.kubernetes.io/totally-made-up"},
			},
		},
		SupportedIngressAnnotations: []analyzer.AnnotationInfo{
			{Name: "nginx.ingress.kubernetes.io/rewrite-target", Version: "v3.7"},
			{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
		},
		CompatibleV36IngressCount: 1,
		CompatibleV37IngressCount: 1,
		CompatibleHubIngressCount: 0,
	}
}

// compatibleReport has no unsupported Ingresses, exercising the "None" branches.
func compatibleReport() analyzer.Report {
	return analyzer.Report{
		GenerationDate:                time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC),
		Version:                       "v0.3.0",
		Hash:                          "allgood",
		IngressCount:                  3,
		IngressCountByClass:           map[string]int{"nginx": 3},
		CompatibleIngressCount:        3,
		CompatibleIngressPercentage:   100.0,
		VanillaIngressCount:           1,
		VanillaIngressPercentage:      33.33333333333333,
		SupportedIngressCount:         2,
		SupportedIngressPercentage:    66.66666666666666,
		UnsupportedIngressAnnotations: map[string]int{},
		UnknownIngressAnnotations:     map[string]int{},
		SupportedIngressAnnotations: []analyzer.AnnotationInfo{
			{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
		},
		CompatibleV36IngressCount: 2,
		CompatibleV37IngressCount: 1,
	}
}

// emptyReport is an analysis of a cluster with no in-scope Ingresses.
func emptyReport() analyzer.Report {
	return analyzer.Report{
		GenerationDate:                time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC),
		Version:                       "v0.3.0",
		Hash:                          "empty",
		IngressCountByClass:           map[string]int{},
		UnsupportedIngressAnnotations: map[string]int{},
		UnknownIngressAnnotations:     map[string]int{},
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		report  analyzer.Report
		format  string
		summary bool
		golden  string
	}{
		{name: "mixed json", report: mixedReport(), format: FormatJSON, golden: "mixed.json"},
		{name: "mixed markdown full", report: mixedReport(), format: FormatMarkdown, golden: "mixed.full.md"},
		{name: "mixed markdown summary", report: mixedReport(), format: FormatMarkdown, summary: true, golden: "mixed.summary.md"},
		{name: "compatible markdown full", report: compatibleReport(), format: FormatMarkdown, golden: "compatible.full.md"},
		{name: "empty json", report: emptyReport(), format: FormatJSON, golden: "empty.json"},
		{name: "empty markdown full", report: emptyReport(), format: FormatMarkdown, golden: "empty.full.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			require.NoError(t, Render(tt.report, tt.format, tt.summary, &buf))

			goldenPath := filepath.Join("testdata", tt.golden)
			if *update {
				require.NoError(t, os.WriteFile(goldenPath, buf.Bytes(), 0o644))
			}

			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err)
			assert.Equal(t, string(want), buf.String())
		})
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := Render(mixedReport(), "yaml", false, &buf)
	require.Error(t, err)
	assert.Empty(t, buf.String())
}
