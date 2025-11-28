package analyzer

import (
	"strings"

	netv1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
)

// FIXME timestamp

//Analyzed 100 Ingress Resources.
//80 Ingress (80%) can be migrated automatically
//20 Ingress (20%) needs your attention because of unsupported annotations (see below)
//
//Unsupported annotations
//Annotation 1 (number of times it appears)
//Annotation 2 (number of times it appears)
//Annotation 3 (number of times it appears)
//…
//
//Ingress Resource to migrate
//Name 1 (list of annotations)
//Name 2 (list of annotations)
//…

const ingressNginxAnnotationPrefix = "nginx.ingress.kubernetes.io"
const traefikIngressAnnotationPrefix = "traefik.ingress.kubernetes.io"

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

type IngressReport struct {
	Name                   string   `json:"name"`
	Namespace              string   `json:"namespace"`
	IngressClassName       string   `json:"ingressClassName"`
	UnsupportedAnnotations []string `json:"unsupportedAnnotations"`
	HasNginxAnnotation     bool     `json:"-"`
}

type Report struct {
	IngressCount int `json:"ingressCount"`

	// Compatible means all ingresses compatible with the ingress-nginx provider,
	// would they be with or without NGINX annotations.
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
	UnsupportedPercentage         float64         `json:"unsupportedPercentage"`
	UnsupportedIngressAnnotations map[string]int  `json:"unsupportedIngressAnnotations"`
	UnsupportedIngresses          []IngressReport `json:"unsupportedIngresses"`
}

func computeReport(ingresses []*netv1.Ingress) Report {
	report := Report{
		IngressCount:                  len(ingresses),
		UnsupportedIngressAnnotations: make(map[string]int),
	}

	for _, ing := range ingresses {
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
		report.UnsupportedPercentage = float64(report.UnsupportedIngressCount) / float64(report.IngressCount) * 100
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
