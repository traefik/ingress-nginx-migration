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
	Name                   string
	Namespace              string
	IngressClassName       string
	UnsupportedAnnotations []string
}

type Report struct {
	IngressCount                  int
	CompatibleIngressCount        int
	UnsupportedIngressCount       int
	UnsupportedIngressAnnotations map[string]int
	UnsupportedIngresses          []IngressReport
}

func computeReport(ingresses []*netv1.Ingress) Report {
	report := Report{
		IngressCount:                  len(ingresses),
		UnsupportedIngressAnnotations: make(map[string]int),
	}

	for _, ing := range ingresses {
		ingReport := computeIngressReport(ing)

		if len(ingReport.UnsupportedAnnotations) == 0 {
			report.CompatibleIngressCount++
			continue
		}

		report.UnsupportedIngressCount++
		report.UnsupportedIngresses = append(report.UnsupportedIngresses, *ingReport)

		for _, a := range ingReport.UnsupportedAnnotations {
			report.UnsupportedIngressAnnotations[a] += 1
		}
	}

	return report
}

func computeIngressReport(ing *netv1.Ingress) *IngressReport {
	var unsupportedAnnotations []string
	for annotation := range ing.Annotations {
		if !strings.HasPrefix(annotation, ingressNginxAnnotationPrefix) {
			continue
		}

		if _, ok := supportedAnnotations[annotation]; !ok {
			unsupportedAnnotations = append(unsupportedAnnotations, annotation)
		}
	}

	return &IngressReport{
		Name:                   ing.Name,
		Namespace:              ing.Namespace,
		IngressClassName:       ptr.Deref(ing.Spec.IngressClassName, ""),
		UnsupportedAnnotations: unsupportedAnnotations,
	}
}
