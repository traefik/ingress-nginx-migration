package analyzer

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
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

// supportedAnnotations maps supported NGINX ingress controller annotations
// to the minimum Traefik version that supports them.
var supportedAnnotations = map[string]string{
	// Authentication (basic/digest).
	"nginx.ingress.kubernetes.io/auth-type":        "v3.6",
	"nginx.ingress.kubernetes.io/auth-secret":      "v3.6",
	"nginx.ingress.kubernetes.io/auth-realm":       "v3.6",
	"nginx.ingress.kubernetes.io/auth-secret-type": "v3.6",
	// Forward authentication.
	"nginx.ingress.kubernetes.io/auth-url":              "v3.6",
	"nginx.ingress.kubernetes.io/auth-response-headers": "v3.6",
	"nginx.ingress.kubernetes.io/auth-signin":           "v3.7",
	// Client TLS authentication.
	"nginx.ingress.kubernetes.io/auth-tls-secret":                       "v3.7",
	"nginx.ingress.kubernetes.io/auth-tls-verify-client":                "v3.7",
	"nginx.ingress.kubernetes.io/auth-tls-pass-certificate-to-upstream": "v3.7",
	// SSL/TLS.
	"nginx.ingress.kubernetes.io/force-ssl-redirect": "v3.6",
	"nginx.ingress.kubernetes.io/ssl-redirect":       "v3.6",
	"nginx.ingress.kubernetes.io/ssl-passthrough":    "v3.6",
	// Path matching & rewriting.
	"nginx.ingress.kubernetes.io/use-regex":      "v3.6",
	"nginx.ingress.kubernetes.io/rewrite-target": "v3.7",
	"nginx.ingress.kubernetes.io/app-root":       "v3.7",
	// Redirects.
	"nginx.ingress.kubernetes.io/permanent-redirect":      "v3.7",
	"nginx.ingress.kubernetes.io/permanent-redirect-code": "v3.7",
	"nginx.ingress.kubernetes.io/temporal-redirect":       "v3.7",
	"nginx.ingress.kubernetes.io/temporal-redirect-code":  "v3.7",
	"nginx.ingress.kubernetes.io/from-to-www-redirect":    "v3.7",
	// Session affinity.
	"nginx.ingress.kubernetes.io/affinity":                 "v3.6",
	"nginx.ingress.kubernetes.io/affinity-canary-behavior": "v3.7",
	"nginx.ingress.kubernetes.io/session-cookie-name":      "v3.6",
	"nginx.ingress.kubernetes.io/session-cookie-secure":    "v3.6",
	"nginx.ingress.kubernetes.io/session-cookie-path":      "v3.6",
	"nginx.ingress.kubernetes.io/session-cookie-domain":    "v3.6",
	"nginx.ingress.kubernetes.io/session-cookie-samesite":  "v3.6",
	"nginx.ingress.kubernetes.io/session-cookie-max-age":   "v3.6",
	"nginx.ingress.kubernetes.io/session-cookie-expires":   "v3.7",
	// Service upstream.
	"nginx.ingress.kubernetes.io/service-upstream": "v3.6",
	// Backend protocol.
	"nginx.ingress.kubernetes.io/backend-protocol": "v3.6",
	// Proxy SSL.
	"nginx.ingress.kubernetes.io/proxy-ssl-secret":      "v3.6",
	"nginx.ingress.kubernetes.io/proxy-ssl-verify":      "v3.6",
	"nginx.ingress.kubernetes.io/proxy-ssl-name":        "v3.6",
	"nginx.ingress.kubernetes.io/proxy-ssl-server-name": "v3.6",
	// Proxy timeout.
	"nginx.ingress.kubernetes.io/proxy-connect-timeout": "v3.7",
	"nginx.ingress.kubernetes.io/proxy-read-timeout":    "v3.7",
	"nginx.ingress.kubernetes.io/proxy-send-timeout":    "v3.7",
	// CORS.
	"nginx.ingress.kubernetes.io/enable-cors":            "v3.6",
	"nginx.ingress.kubernetes.io/cors-allow-credentials": "v3.6",
	"nginx.ingress.kubernetes.io/cors-expose-headers":    "v3.6",
	"nginx.ingress.kubernetes.io/cors-allow-headers":     "v3.6",
	"nginx.ingress.kubernetes.io/cors-allow-methods":     "v3.6",
	"nginx.ingress.kubernetes.io/cors-allow-origin":      "v3.6",
	"nginx.ingress.kubernetes.io/cors-max-age":           "v3.6",
	// Error pages.
	"nginx.ingress.kubernetes.io/custom-http-errors": "v3.7",
	"nginx.ingress.kubernetes.io/default-backend":    "v3.7",
	// Proxy next upstream.
	"nginx.ingress.kubernetes.io/proxy-next-upstream":         "v3.7",
	"nginx.ingress.kubernetes.io/proxy-next-upstream-tries":   "v3.7",
	"nginx.ingress.kubernetes.io/proxy-next-upstream-timeout": "v3.7",
	// IP allowlist.
	"nginx.ingress.kubernetes.io/whitelist-source-range": "v3.7",
	"nginx.ingress.kubernetes.io/allowlist-source-range": "v3.7",
	// Custom headers.
	"nginx.ingress.kubernetes.io/custom-headers": "v3.7",
	"nginx.ingress.kubernetes.io/upstream-vhost": "v3.7",
	// Buffering.
	"nginx.ingress.kubernetes.io/proxy-request-buffering":  "v3.7",
	"nginx.ingress.kubernetes.io/client-body-buffer-size":  "v3.7",
	"nginx.ingress.kubernetes.io/proxy-body-size":          "v3.7",
	"nginx.ingress.kubernetes.io/proxy-buffering":          "v3.7",
	"nginx.ingress.kubernetes.io/proxy-buffer-size":        "v3.7",
	"nginx.ingress.kubernetes.io/proxy-buffers-number":     "v3.7",
	"nginx.ingress.kubernetes.io/proxy-max-temp-file-size": "v3.7",
	// Rate limiting.
	"nginx.ingress.kubernetes.io/limit-rpm": "v3.7",
	"nginx.ingress.kubernetes.io/limit-rps": "v3.7",
	// Server alias.
	"nginx.ingress.kubernetes.io/server-alias": "v3.7",
	// Upstream hash.
	"nginx.ingress.kubernetes.io/upstream-hash-by": "v3.7",
	// Proxy HTTP version.
	"nginx.ingress.kubernetes.io/proxy-http-version": "v3.7",
	// X-Forwarded-Prefix.
	"nginx.ingress.kubernetes.io/x-forwarded-prefix": "v3.7",
	// Snippets.
	"nginx.ingress.kubernetes.io/configuration-snippet": "v3.7",
	"nginx.ingress.kubernetes.io/server-snippet":        "v3.7",
	// Canary.
	"nginx.ingress.kubernetes.io/canary":                   "v3.7",
	"nginx.ingress.kubernetes.io/canary-by-cookie":         "v3.7",
	"nginx.ingress.kubernetes.io/canary-by-header":         "v3.7",
	"nginx.ingress.kubernetes.io/canary-by-header-value":   "v3.7",
	"nginx.ingress.kubernetes.io/canary-by-header-pattern": "v3.7",
	"nginx.ingress.kubernetes.io/canary-weight":            "v3.7",
	"nginx.ingress.kubernetes.io/canary-weight-total":      "v3.7",
	// ModSecurity (Traefik Hub only).
	"nginx.ingress.kubernetes.io/enable-modsecurity":         "Traefik Hub v3.20",
	"nginx.ingress.kubernetes.io/enable-owasp-core-rules":    "Traefik Hub v3.20",
	"nginx.ingress.kubernetes.io/modsecurity-transaction-id": "Traefik Hub v3.20",
	"nginx.ingress.kubernetes.io/modsecurity-snippet":        "Traefik Hub v3.20",
}

// AnnotationInfo contains annotation name and its minimum required Traefik version.
type AnnotationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// IngressReport contains the analysis report for a single Ingress.
type IngressReport struct {
	Name                   string            `json:"name"`
	Namespace              string            `json:"namespace"`
	IngressClassName       string            `json:"ingressClassName"`
	UnsupportedAnnotations []string          `json:"unsupportedAnnotations"`
	SupportedAnnotations   []AnnotationInfo  `json:"supportedAnnotations,omitempty"`
	HasNginxAnnotation     bool              `json:"-"`
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

	// SupportedIngressAnnotations lists all supported annotations found in user's ingresses, sorted by name.
	SupportedIngressAnnotations []AnnotationInfo `json:"supportedIngressAnnotations"`

	// Version-specific breakdown of compatible ingresses (only those with nginx annotations).
	CompatibleV36IngressCount int `json:"compatibleV36IngressCount"`
	CompatibleV37IngressCount int `json:"compatibleV37IngressCount"`
	CompatibleHubIngressCount int `json:"compatibleHubIngressCount"`
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

	// Aggregate all supported annotations across ingresses.
	allSupportedAnnotations := make(map[string]string)

	// Then we iterate over all ingresses and check if they use a NGINX ingress class.
	for _, ing := range ingresses {
		ok, nginxIngressClass := a.shouldProcessIngress(ing, nginxIngressClasses)
		if !ok {
			continue
		}

		report.IngressCount++
		report.IngressCountByClass[nginxIngressClass]++

		ingReport := computeIngressReport(ing)

		// Merge supported annotations into report-level map.
		for _, ann := range ingReport.SupportedAnnotations {
			allSupportedAnnotations[ann.Name] = ann.Version
		}

		// Ingress is compatible
		if len(ingReport.UnsupportedAnnotations) == 0 {
			report.CompatibleIngressCount++

			if !ingReport.HasNginxAnnotation {
				report.VanillaIngressCount++
				report.CompatibleV36IngressCount++
				continue
			}

			report.SupportedIngressCount++
			report.classifyIngressVersion(ingReport.SupportedAnnotations)
			continue
		}

		// Has unsupported NGINX annotations
		report.UnsupportedIngressCount++
		report.UnsupportedIngresses = append(report.UnsupportedIngresses, *ingReport)

		for _, a := range ingReport.UnsupportedAnnotations {
			report.UnsupportedIngressAnnotations[a]++
		}
	}

	// Build sorted slice of supported annotations.
	for ann, ver := range allSupportedAnnotations {
		report.SupportedIngressAnnotations = append(report.SupportedIngressAnnotations, AnnotationInfo{Name: ann, Version: ver})
	}
	slices.SortFunc(report.SupportedIngressAnnotations, func(a, b AnnotationInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

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
	Version                       string           `json:"version"`
	IngressCount                  int              `json:"ingressCount"`
	CompatibleIngressCount        int              `json:"compatibleIngressCount"`
	VanillaIngressCount           int              `json:"vanillaIngressCount"`
	SupportedIngressCount         int              `json:"supportedIngressCount"`
	UnsupportedIngressCount       int              `json:"unsupportedIngressCount"`
	UnsupportedIngressAnnotations map[string]int   `json:"unsupportedIngressAnnotations"`
	SupportedIngressAnnotations   []AnnotationInfo `json:"supportedIngressAnnotations"`
	CompatibleV36IngressCount     int              `json:"compatibleV36IngressCount"`
	CompatibleV37IngressCount     int              `json:"compatibleV37IngressCount"`
	CompatibleHubIngressCount     int              `json:"compatibleHubIngressCount"`
}

func (r *Report) classifyIngressVersion(supportedAnnotations []AnnotationInfo) {
	var requiresHub, requiresV37 bool

	for _, ann := range supportedAnnotations {
		switch {
		case strings.HasPrefix(ann.Version, "Traefik Hub"):
			requiresHub = true
		case ann.Version == "v3.7":
			requiresV37 = true
		}
	}

	switch {
	case requiresHub:
		r.CompatibleHubIngressCount++
	case requiresV37:
		r.CompatibleV37IngressCount++
	default:
		r.CompatibleV36IngressCount++
	}
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
		SupportedIngressAnnotations:   report.SupportedIngressAnnotations,
		CompatibleV36IngressCount:     report.CompatibleV36IngressCount,
		CompatibleV37IngressCount:     report.CompatibleV37IngressCount,
		CompatibleHubIngressCount:     report.CompatibleHubIngressCount,
	}

	data, _ := json.Marshal(payload) //nolint:errchkjson
	hash := sha256.Sum256(data)

	return hex.EncodeToString(hash[:])
}

func computeIngressReport(ing *netv1.Ingress) *IngressReport {
	var hasNginxAnnotation bool
	var unsupportedAnnotations []string
	var supported []AnnotationInfo

	for annotation := range ing.Annotations {
		if strings.HasPrefix(annotation, ingressNginxAnnotationPrefix) {
			hasNginxAnnotation = true
			// This is a NGINX ingress annotation
			if ver, ok := supportedAnnotations[annotation]; ok {
				supported = append(supported, AnnotationInfo{Name: annotation, Version: ver})
			} else {
				unsupportedAnnotations = append(unsupportedAnnotations, annotation)
			}
			// TODO: also check the annotation value that are not supported, like nginx.ingress.kubernetes.io/backend-protocol: FCGI
		}
	}

	slices.SortFunc(supported, func(a, b AnnotationInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return &IngressReport{
		Name:                   ing.Name,
		Namespace:              ing.Namespace,
		IngressClassName:       ptr.Deref(ing.Spec.IngressClassName, ""),
		UnsupportedAnnotations: unsupportedAnnotations,
		SupportedAnnotations:   supported,
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
