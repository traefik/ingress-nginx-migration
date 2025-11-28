package analyzer

import (
	"strings"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
)

const (
	defaultAnnotationValue       = "nginx"
	annotationIngressClass       = "kubernetes.io/ingress.class"
	ingressNginxAnnotationPrefix = "nginx.ingress.kubernetes.io"
)

// FIXME timestamp
// FIXME: all value are not supported
// Supported annotations contains the list of supported nginx ingress controller annotations.
var supportedAnnotations = map[string]struct{}{
	"nginx.ingress.kubernetes.io/auth-type":               {},
	"nginx.ingress.kubernetes.io/auth-secret":             {},
	"nginx.ingress.kubernetes.io/auth-realm":              {},
	"nginx.ingress.kubernetes.io/auth-secret-type":        {},
	"nginx.ingress.kubernetes.io/auth-url":                {},
	"nginx.ingress.kubernetes.io/auth-response-headers":   {},
	"nginx.ingress.kubernetes.io/force-ssl-redirect":      {},
	"nginx.ingress.kubernetes.io/ssl-redirect":            {},
	"nginx.ingress.kubernetes.io/ssl-passthrough":         {},
	"nginx.ingress.kubernetes.io/use-regex":               {},
	"nginx.ingress.kubernetes.io/affinity":                {},
	"nginx.ingress.kubernetes.io/session-cookie-name":     {},
	"nginx.ingress.kubernetes.io/session-cookie-secure":   {},
	"nginx.ingress.kubernetes.io/session-cookie-path":     {},
	"nginx.ingress.kubernetes.io/session-cookie-domain":   {},
	"nginx.ingress.kubernetes.io/session-cookie-samesite": {},
	"nginx.ingress.kubernetes.io/session-cookie-max-age":  {},
	"nginx.ingress.kubernetes.io/service-upstream":        {},
	"nginx.ingress.kubernetes.io/backend-protocol":        {},
	"nginx.ingress.kubernetes.io/proxy-ssl-secret":        {},
	"nginx.ingress.kubernetes.io/proxy-ssl-verify":        {},
	"nginx.ingress.kubernetes.io/proxy-ssl-name":          {},
	"nginx.ingress.kubernetes.io/proxy-ssl-server-name":   {},
	"nginx.ingress.kubernetes.io/enable-cors":             {},
	"nginx.ingress.kubernetes.io/cors-allow-credentials":  {},
	"nginx.ingress.kubernetes.io/cors-expose-headers":     {},
	"nginx.ingress.kubernetes.io/cors-allow-headers":      {},
	"nginx.ingress.kubernetes.io/cors-allow-methods":      {},
	"nginx.ingress.kubernetes.io/cors-allow-origin":       {},
	"nginx.ingress.kubernetes.io/cors-max-age":            {},
}

// IngressReport contains the analysis report for a single Ingress.
type IngressReport struct {
	Name                   string   `json:"name"`
	Namespace              string   `json:"namespace"`
	IngressClassName       string   `json:"ingressClassName"`
	UnsupportedAnnotations []string `json:"unsupportedAnnotations"`
	HasNginxAnnotation     bool     `json:"-"`
}

// Report contains the analysis report for all Ingresses.
// FIXME: maybe we should report which IgressClasses are discovered.
type Report struct {
	IngressCount int `json:"ingressCount"`

	// Compatible means all ingresses compatible with the ingress-nginx provider with or without NGINX annotations.
	CompatibleIngressCount      int     `json:"compatibleIngressCount"`
	CompatibleIngressPercentage float64 `json:"compatibleIngressPercentage"`

	// Vanilla means all (supported) ingresses without ingress-nginx controller specific annotations.
	VanillaIngressCount      int     `json:"vanillaIngressCount"`
	VanillaIngressPercentage float64 `json:"vanillaIngressPercentage"`

	// Supported means all ingresses with only supported ingress-nginx controller specific annotations.
	SupportedIngressCount      int     `json:"supportedIngressCount"`
	SupportedIngressPercentage float64 `json:"supportedIngressPercentage"`

	// Unsupported means all ingresses with unsupported ingress-nginx controller specific annotations.
	UnsupportedIngressCount       int             `json:"unsupportedIngressCount"`
	UnsupportedIngressPercentage  float64         `json:"unsupportedIngressPercentage"`
	UnsupportedIngressAnnotations map[string]int  `json:"unsupportedIngressAnnotations"`
	UnsupportedIngresses          []IngressReport `json:"unsupportedIngresses"`
}

func computeReport(ingressClasses []*netv1.IngressClass, ingresses []*netv1.Ingress, controllerClass string) Report {
	report := Report{UnsupportedIngressAnnotations: make(map[string]int)}

	// First we filter all Nginx ingress classes.
	ingressClassNames := make(map[string]struct{})
	for _, ic := range ingressClasses {
		if ic.Spec.Controller == controllerClass {
			ingressClassNames[ic.Name] = struct{}{}
		}
	}

	// Then we iterate over all ingresses and check if they use a Nginx ingress class.
	for _, ing := range ingresses {
		// Ingress does not use a Nginx ingress class.
		if _, exists := ingressClassNames[ptr.Deref(ing.Spec.IngressClassName, "")]; !exists && len(ingressClassNames) > 0 {
			continue
		}
		if ing.Annotations[annotationIngressClass] != "" && ing.Annotations[annotationIngressClass] != defaultAnnotationValue {
			continue
		}

		report.IngressCount++

		ingReport := computeIngressReport(ing)

		// Ingress is compatible
		if len(ingReport.UnsupportedAnnotations) == 0 {
			report.CompatibleIngressCount++

			if ingReport.HasNginxAnnotation {
				report.SupportedIngressCount++
				continue
			}

			report.VanillaIngressCount++
			continue
		}

		// Has unsupported nginx annotations
		report.UnsupportedIngressCount++
		report.UnsupportedIngresses = append(report.UnsupportedIngresses, *ingReport)

		for _, a := range ingReport.UnsupportedAnnotations {
			report.UnsupportedIngressAnnotations[a] += 1
		}
	}

	// Calculate percentages
	if report.IngressCount > 0 {
		report.CompatibleIngressPercentage = float64(report.CompatibleIngressCount) / float64(report.IngressCount) * 100
		report.VanillaIngressPercentage = float64(report.VanillaIngressCount) / float64(report.IngressCount) * 100
		report.SupportedIngressPercentage = float64(report.SupportedIngressCount) / float64(report.IngressCount) * 100
		report.UnsupportedIngressPercentage = float64(report.UnsupportedIngressCount) / float64(report.IngressCount) * 100
	}

	return report
}

func computeIngressReport(ing *netv1.Ingress) *IngressReport {
	var hasNginxAnnotation bool
	var unsupportedAnnotations []string

	for annotation := range ing.Annotations {
		if strings.HasPrefix(annotation, ingressNginxAnnotationPrefix) {
			hasNginxAnnotation = true
			// This is a nginx ingress annotation
			if _, ok := supportedAnnotations[annotation]; !ok {
				unsupportedAnnotations = append(unsupportedAnnotations, annotation)
			}
			// FIXME: also check the annotation value? like nginx.ingress.kubernetes.io/backend-protocol: FCGI
		}
	}

	return &IngressReport{
		Name:                   ing.Name,
		Namespace:              ing.Namespace,
		IngressClassName:       ptr.Deref(ing.Spec.IngressClassName, ""),
		UnsupportedAnnotations: unsupportedAnnotations,
		HasNginxAnnotation:     hasNginxAnnotation,
	}
}
