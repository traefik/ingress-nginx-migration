package analyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestClassifyIngressVersion(t *testing.T) {
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
	tests := []struct {
		name                   string
		annotations            map[string]string
		wantSupported          []AnnotationInfo
		wantUnsupportedCount   int
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
			name: "mix of supported and unsupported",
			annotations: map[string]string{
				"nginx.ingress.kubernetes.io/ssl-redirect":       "true",
				"nginx.ingress.kubernetes.io/enable-opentracing": "true",
			},
			wantSupported: []AnnotationInfo{
				{Name: "nginx.ingress.kubernetes.io/ssl-redirect", Version: "v3.6"},
			},
			wantUnsupportedCount:   1,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ing := &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ingress",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
				Spec: netv1.IngressSpec{
					IngressClassName: ptr.To("nginx"),
				},
			}

			report := computeIngressReport(ing)

			assert.Equal(t, "test-ingress", report.Name)
			assert.Equal(t, "default", report.Namespace)
			assert.Equal(t, "nginx", report.IngressClassName)
			assert.Equal(t, tt.wantHasNginxAnnotation, report.HasNginxAnnotation)
			assert.Equal(t, tt.wantSupported, report.SupportedAnnotations)
			assert.Len(t, report.UnsupportedAnnotations, tt.wantUnsupportedCount)
		})
	}
}
