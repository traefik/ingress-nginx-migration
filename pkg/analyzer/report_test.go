package analyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClassifyIngressVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		supportedAnnotations []AnnotationInfo
		wantBucket           string // "v3.6", "v3.7", or "hub"
	}{
		{
			name:                 "v3.6 only annotations",
			supportedAnnotations: []AnnotationInfo{{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"}},
			wantBucket:           "v3.6",
		},
		{
			name: "v3.7 annotation bumps to v3.7",
			supportedAnnotations: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
				{Name: "nginx.ingress.kubernetes.io/rewrite-target", Version: "v3.7"},
			},
			wantBucket: "v3.7",
		},
		{
			name: "Traefik Hub annotation bumps to Hub",
			supportedAnnotations: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
				{Name: "nginx.ingress.kubernetes.io/enable-modsecurity", Version: "Traefik Hub v3.20"},
			},
			wantBucket: "hub",
		},
		{
			name: "Hub takes priority over v3.7",
			supportedAnnotations: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/rewrite-target", Version: "v3.7"},
				{Name: "nginx.ingress.kubernetes.io/enable-modsecurity", Version: "Traefik Hub v3.20"},
			},
			wantBucket: "hub",
		},
		{
			name:                 "no annotations defaults to v3.6",
			supportedAnnotations: nil,
			wantBucket:           "v3.6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var r Report
			r.classifyIngressVersion(tt.supportedAnnotations)

			switch tt.wantBucket {
			case "v3.6":
				assert.Equal(t, 1, r.CompatibleV36IngressCount, "should be classified as v3.6")
				assert.Zero(t, r.CompatibleV37IngressCount)
				assert.Zero(t, r.CompatibleHubIngressCount)
			case "v3.7":
				assert.Zero(t, r.CompatibleV36IngressCount)
				assert.Equal(t, 1, r.CompatibleV37IngressCount, "should be classified as v3.7")
				assert.Zero(t, r.CompatibleHubIngressCount)
			case "hub":
				assert.Zero(t, r.CompatibleV36IngressCount)
				assert.Zero(t, r.CompatibleV37IngressCount)
				assert.Equal(t, 1, r.CompatibleHubIngressCount, "should be classified as Hub")
			}
		})
	}
}

func TestComputeIngressReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		annotations            map[string]string
		wantSupported          []AnnotationInfo
		wantUnsupported        []string
		wantUnknown            []string
		wantHasNginxAnnotation bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
		},
		{
			name: "only supported v3.6 annotations",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-redirect":       "true",
				"nginx.ingress.kubernetes.io/force-ssl-redirect": "true",
			},
			wantSupported: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/force-ssl-redirect", Version: "v3.6"},
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
			},
			wantHasNginxAnnotation: true,
		},
		{
			name: "known-unsupported annotation is classified as unsupported",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-redirect":       "true",
				"nginx.ingress.kubernetes.io/enable-opentracing": "true",
			},
			wantSupported: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
			},
			wantUnsupported:        []string{"nginx.ingress.kubernetes.io/enable-opentracing"},
			wantHasNginxAnnotation: true,
		},
		{
			name: "unknown annotation is classified as unknown, not unsupported",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-redirect":         "true",
				"nginx.ingress.kubernetes.io/totally-unknown-flag": "true",
			},
			wantSupported: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
			},
			wantUnknown:            []string{"nginx.ingress.kubernetes.io/totally-unknown-flag"},
			wantHasNginxAnnotation: true,
		},
		{
			name: "mix of supported, known-unsupported, and unknown annotations",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-redirect":         "true",
				"nginx.ingress.kubernetes.io/limit-connections":    "10",
				"nginx.ingress.kubernetes.io/totally-unknown-flag": "true",
			},
			wantSupported: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
			},
			wantUnsupported:        []string{"nginx.ingress.kubernetes.io/limit-connections"},
			wantUnknown:            []string{"nginx.ingress.kubernetes.io/totally-unknown-flag"},
			wantHasNginxAnnotation: true,
		},
		{
			name: "only known-unsupported annotations",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/denylist-source-range": "192.0.2.0/24",
				"nginx.ingress.kubernetes.io/limit-connections":     "10",
			},
			wantUnsupported: []string{
				"nginx.ingress.kubernetes.io/denylist-source-range",
				"nginx.ingress.kubernetes.io/limit-connections",
			},
			wantHasNginxAnnotation: true,
		},
		{
			name: "only unknown annotations",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/does-not-exist": "true",
			},
			wantUnknown:            []string{"nginx.ingress.kubernetes.io/does-not-exist"},
			wantHasNginxAnnotation: true,
		},
		{
			name: "non-nginx annotations are ignored",
			annotations: map[string]string{
				"kubernetes.io/ingress.class": "nginx",
			},
		},
		{
			name: "supported annotations are sorted by name",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-redirect":       "true",
				"nginx.ingress.kubernetes.io/backend-protocol":   "HTTP",
				"nginx.ingress.kubernetes.io/force-ssl-redirect": "true",
			},
			wantSupported: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/backend-protocol", Version: "v3.6"},
				{Name: "nginx.ingress.kubernetes.io/force-ssl-redirect", Version: "v3.6"},
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
			},
			wantHasNginxAnnotation: true,
		},
		{
			name: "known-unsupported annotations are sorted by name",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/stream-snippet":        "true",
				"nginx.ingress.kubernetes.io/denylist-source-range": "192.0.2.0/24",
			},
			wantUnsupported: []string{
				"nginx.ingress.kubernetes.io/denylist-source-range",
				"nginx.ingress.kubernetes.io/stream-snippet",
			},
			wantHasNginxAnnotation: true,
		},
		{
			name: "unknown annotations are sorted by name",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/zzz-unknown": "true",
				"nginx.ingress.kubernetes.io/aaa-unknown": "true",
			},
			wantUnknown: []string{
				"nginx.ingress.kubernetes.io/aaa-unknown",
				"nginx.ingress.kubernetes.io/zzz-unknown",
			},
			wantHasNginxAnnotation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ing := &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
				Spec: netv1.IngressSpec{
					IngressClassName: new("nginx"),
				},
			}

			report := computeIngressReport(ing)

			assert.Equal(t, "test-ingress", report.Name)
			assert.Equal(t, "default", report.Namespace)
			assert.Equal(t, "nginx", report.IngressClassName)
			assert.Equal(t, tt.wantHasNginxAnnotation, report.HasNginxAnnotation)
			assert.Equal(t, tt.wantSupported, report.SupportedAnnotations)
			assert.Equal(t, tt.wantUnsupported, report.UnsupportedAnnotations)
			assert.Equal(t, tt.wantUnknown, report.UnknownAnnotations)
		})
	}
}

func TestComputeReport_AnnotationClassification(t *testing.T) {
	t.Parallel()

	a := &Analyzer{
		ingressClass:    "nginx",
		controllerClass: "k8s.io/ingress-nginx",
	}

	ingressClass := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx"},
		Spec:       netv1.IngressClassSpec{Controller: "k8s.io/ingress-nginx"},
	}

	makeIngress := func(name string, annotations map[string]string) *netv1.Ingress {
		className := "nginx"
		return &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   "default",
				Annotations: annotations,
			},
			Spec: netv1.IngressSpec{IngressClassName: &className},
		}
	}

	ingresses := []*netv1.Ingress{
		// Vanilla: no NGINX annotations.
		makeIngress("vanilla", nil),
		// Supported only.
		makeIngress("supported", map[string]string{
			"nginx.ingress.kubernetes.io/ssl-redirect": "true",
		}),
		// Known-unsupported annotation.
		makeIngress("known-unsupported", map[string]string{
			"nginx.ingress.kubernetes.io/limit-connections": "10",
		}),
		// Unknown annotation.
		makeIngress("unknown", map[string]string{
			"nginx.ingress.kubernetes.io/totally-made-up": "true",
		}),
		// Mix of unsupported and unknown.
		makeIngress("mixed", map[string]string{
			"nginx.ingress.kubernetes.io/limit-connections":  "10",
			"nginx.ingress.kubernetes.io/totally-made-up":    "true",
			"nginx.ingress.kubernetes.io/force-ssl-redirect": "true",
		}),
	}

	report := a.computeReport([]*netv1.IngressClass{ingressClass}, ingresses)

	assert.Equal(t, 5, report.IngressCount)
	assert.Equal(t, 2, report.CompatibleIngressCount, "vanilla + supported should be compatible")
	assert.Equal(t, 1, report.VanillaIngressCount)
	assert.Equal(t, 1, report.SupportedIngressCount)
	assert.Equal(t, 3, report.UnsupportedIngressCount, "known-unsupported + unknown + mixed")

	// Unknown annotation frequencies.
	assert.Equal(t, map[string]int{
		"nginx.ingress.kubernetes.io/totally-made-up": 2,
	}, report.UnknownIngressAnnotations)

	// Known-unsupported annotation frequencies.
	assert.Equal(t, map[string]int{
		"nginx.ingress.kubernetes.io/limit-connections": 2,
	}, report.UnsupportedIngressAnnotations)

	// Verify individual ingress reports carry the right buckets.
	byName := make(map[string]IngressReport)
	for _, ir := range report.UnsupportedIngresses {
		byName[ir.Name] = ir
	}

	knownUnsupportedReport := byName["known-unsupported"]
	assert.Equal(t, []string{"nginx.ingress.kubernetes.io/limit-connections"}, knownUnsupportedReport.UnsupportedAnnotations)
	assert.Empty(t, knownUnsupportedReport.UnknownAnnotations)

	unknownReport := byName["unknown"]
	assert.Empty(t, unknownReport.UnsupportedAnnotations)
	assert.Equal(t, []string{"nginx.ingress.kubernetes.io/totally-made-up"}, unknownReport.UnknownAnnotations)

	mixedReport := byName["mixed"]
	assert.Equal(t, []string{"nginx.ingress.kubernetes.io/limit-connections"}, mixedReport.UnsupportedAnnotations)
	assert.Equal(t, []string{"nginx.ingress.kubernetes.io/totally-made-up"}, mixedReport.UnknownAnnotations)
	assert.Len(t, mixedReport.SupportedAnnotations, 1)
}

// TestNoOverlapBetweenSupportedAndKnownUnsupported guards against an annotation
// being accidentally placed in both the supported and known-unsupported sets,
// which would silently bias classification toward whichever lookup runs first.
func TestNoOverlapBetweenSupportedAndKnownUnsupported(t *testing.T) {
	t.Parallel()

	for ann := range knownUnsupportedAnnotations {
		_, ok := supportedAnnotations[ann]
		assert.Falsef(t, ok, "annotation %q is present in both supportedAnnotations and knownUnsupportedAnnotations", ann)
	}
}
