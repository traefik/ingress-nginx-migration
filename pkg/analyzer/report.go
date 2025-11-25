package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/traefik/ingress-nginx-migration/pkg/version"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
)

const (
	annotationIngressClass       = "kubernetes.io/ingress.class"
	ingressNginxAnnotationPrefix = "nginx.ingress.kubernetes.io"
	withoutClass                 = "without-class"
)

// Supported annotations contains the list of supported NGINX ingress controller annotations.
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
type Report struct {
	GenerationDate time.Time `json:"generationDate"`
	Version        string    `json:"version"`

	// Hash is a SHA-256 hash of the report content (excluding GenerationDate).
	// Used for localStorage persistence to detect report changes.
	Hash string `json:"-"`

	IngressCount        int            `json:"ingressCount"`
	IngressCountByClass map[string]int `json:"ingressCountByClass"`

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

func (a *Analyzer) computeReport(ingressClasses []*netv1.IngressClass, ingresses []*netv1.Ingress) Report {
	report := Report{
		GenerationDate:                time.Now().UTC(),
		Version:                       version.Version,
		IngressCountByClass:           make(map[string]int),
		UnsupportedIngressAnnotations: make(map[string]int),
	}

	// First we filter all NGINX ingress classes.
	var nginxIngressClasses []*netv1.IngressClass
	for _, ic := range ingressClasses {
		if a.ingressClassByName && ic.Name == a.ingressClass {
			nginxIngressClasses = append(nginxIngressClasses, ic)
			break
		}

		if ic.Spec.Controller == a.controllerClass {
			nginxIngressClasses = append(nginxIngressClasses, ic)
		}
	}

	// Then we iterate over all ingresses and check if they use a NGINX ingress class.
	for _, ing := range ingresses {
		ok, nginxIngressClass := a.shouldProcessIngress(ing, nginxIngressClasses)
		if !ok {
			continue
		}

		report.IngressCount++
		report.IngressCountByClass[nginxIngressClass]++

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

		// Has unsupported NGINX annotations
		report.UnsupportedIngressCount++
		report.UnsupportedIngresses = append(report.UnsupportedIngresses, *ingReport)

		for _, a := range ingReport.UnsupportedAnnotations {
			report.UnsupportedIngressAnnotations[a]++
		}
	}

	// Calculate percentages
	if report.IngressCount > 0 {
		report.CompatibleIngressPercentage = float64(report.CompatibleIngressCount) / float64(report.IngressCount) * 100
		report.VanillaIngressPercentage = float64(report.VanillaIngressCount) / float64(report.IngressCount) * 100
		report.SupportedIngressPercentage = float64(report.SupportedIngressCount) / float64(report.IngressCount) * 100
		report.UnsupportedIngressPercentage = float64(report.UnsupportedIngressCount) / float64(report.IngressCount) * 100
	}

	// Compute hash for localStorage persistence (excludes GenerationDate).
	report.Hash = computeReportHash(report)

	return report
}

// reportHashPayload contains fields used to compute the report hash (excludes GenerationDate).
type reportHashPayload struct {
	Version                       string         `json:"version"`
	IngressCount                  int            `json:"ingressCount"`
	CompatibleIngressCount        int            `json:"compatibleIngressCount"`
	VanillaIngressCount           int            `json:"vanillaIngressCount"`
	SupportedIngressCount         int            `json:"supportedIngressCount"`
	UnsupportedIngressCount       int            `json:"unsupportedIngressCount"`
	UnsupportedIngressAnnotations map[string]int `json:"unsupportedIngressAnnotations"`
}

func computeReportHash(report Report) string {
	payload := reportHashPayload{
		Version:                       report.Version,
		IngressCount:                  report.IngressCount,
		CompatibleIngressCount:        report.CompatibleIngressCount,
		VanillaIngressCount:           report.VanillaIngressCount,
		SupportedIngressCount:         report.SupportedIngressCount,
		UnsupportedIngressCount:       report.UnsupportedIngressCount,
		UnsupportedIngressAnnotations: report.UnsupportedIngressAnnotations,
	}

	data, _ := json.Marshal(payload) //nolint:errchkjson
	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:])
}

func computeIngressReport(ing *netv1.Ingress) *IngressReport {
	var hasNginxAnnotation bool
	var unsupportedAnnotations []string

	for annotation := range ing.Annotations {
		if strings.HasPrefix(annotation, ingressNginxAnnotationPrefix) {
			hasNginxAnnotation = true
			// This is a NGINX ingress annotation
			if _, ok := supportedAnnotations[annotation]; !ok {
				unsupportedAnnotations = append(unsupportedAnnotations, annotation)
			}
			// TODO: also check the annotation value that are not supported, like nginx.ingress.kubernetes.io/backend-protocol: FCGI
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

func (a *Analyzer) shouldProcessIngress(ingress *netv1.Ingress, ingressClasses []*netv1.IngressClass) (bool, string) {
	if len(ingressClasses) > 0 && ingress.Spec.IngressClassName != nil {
		for _, ic := range ingressClasses {
			if ic.Name == *ingress.Spec.IngressClassName {
				return true, ic.Name
			}
		}
	}

	if class, ok := ingress.Annotations[annotationIngressClass]; ok {
		return class == a.ingressClass, class
	}

	return a.watchIngressWithoutClass, withoutClass
}
