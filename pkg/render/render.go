// Package render serializes an analyzer.Report into machine- or human-readable
// formats for the one-shot output mode. It has no knowledge of HTTP; the HTML
// web view lives in pkg/handlers.
package render

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/traefik/ingress-nginx-migration/pkg/analyzer"
)

// Supported output formats for the one-shot mode.
const (
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
)

//go:embed report.md.tmpl
var markdownTemplate string

// Render writes report to w in the given format.
//
// summary only applies to FormatMarkdown, where it omits the per-Ingress detail
// table. Passing summary=true with any other format is an error.
func Render(report analyzer.Report, format string, summary bool, w io.Writer) error {
	if summary && format != FormatMarkdown {
		return fmt.Errorf("--summary is only valid with format %q, got %q", FormatMarkdown, format)
	}

	switch format {
	case FormatJSON:
		return renderJSON(report, w)
	case FormatMarkdown:
		return renderMarkdown(report, summary, w)
	default:
		return fmt.Errorf("unknown format %q (must be %q or %q)", format, FormatJSON, FormatMarkdown)
	}
}

// renderJSON writes the full analyzer.Report as indented JSON with a trailing
// newline. The internal struct is the public contract by design.
func renderJSON(report analyzer.Report, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("encoding report as JSON: %w", err)
	}

	return nil
}

// blockingRow is one annotation that prevents automatic migration, with how many
// Ingresses carry it and whether it is a known-unsupported or an unknown annotation.
type blockingRow struct {
	Annotation string
	Count      int
	Kind       string // "unsupported" or "unknown"
}

// detailRow is one Ingress that needs manual attention.
type detailRow struct {
	Namespace string
	Name      string
	Class     string
	Fixes     string // comma-joined unsupported + unknown annotations
}

// markdownView is the pre-computed, deterministically-ordered view model handed
// to the Markdown template, so the template itself stays free of sorting and
// formatting logic.
type markdownView struct {
	GeneratedAt string
	Version     string
	Hash        string

	IngressCount     int
	CompatibleCount  int
	CompatiblePct    string
	VanillaCount     int
	VanillaPct       string
	SupportedCount   int
	SupportedPct     string
	UnsupportedCount int
	UnsupportedPct   string

	V36 int
	V37 int
	Hub int

	Blocking []blockingRow

	ShowDetail bool
	Detail     []detailRow
}

func renderMarkdown(report analyzer.Report, summary bool, w io.Writer) error {
	tmpl, err := template.New("report.md").Parse(markdownTemplate)
	if err != nil {
		return fmt.Errorf("parsing markdown template: %w", err)
	}

	if err := tmpl.Execute(w, buildMarkdownView(report, summary)); err != nil {
		return fmt.Errorf("executing markdown template: %w", err)
	}

	return nil
}

func buildMarkdownView(report analyzer.Report, summary bool) markdownView {
	view := markdownView{
		GeneratedAt:      report.GenerationDate.UTC().Format(time.RFC3339),
		Version:          report.Version,
		Hash:             report.Hash,
		IngressCount:     report.IngressCount,
		CompatibleCount:  report.CompatibleIngressCount,
		CompatiblePct:    formatPct(report.CompatibleIngressPercentage),
		VanillaCount:     report.VanillaIngressCount,
		VanillaPct:       formatPct(report.VanillaIngressPercentage),
		SupportedCount:   report.SupportedIngressCount,
		SupportedPct:     formatPct(report.SupportedIngressPercentage),
		UnsupportedCount: report.UnsupportedIngressCount,
		UnsupportedPct:   formatPct(report.UnsupportedIngressPercentage),
		V36:              report.CompatibleV36IngressCount,
		V37:              report.CompatibleV37IngressCount,
		Hub:              report.CompatibleHubIngressCount,
		Blocking:         buildBlockingRows(report),
		ShowDetail:       !summary,
	}

	if view.ShowDetail {
		view.Detail = buildDetailRows(report.UnsupportedIngresses)
	}

	return view
}

// buildBlockingRows merges the known-unsupported and unknown annotation
// frequencies into a single list, sorted by descending count then name so the
// most impactful blockers surface first and the order is deterministic.
func buildBlockingRows(report analyzer.Report) []blockingRow {
	rows := make([]blockingRow, 0, len(report.UnsupportedIngressAnnotations)+len(report.UnknownIngressAnnotations))

	for ann, count := range report.UnsupportedIngressAnnotations {
		rows = append(rows, blockingRow{Annotation: ann, Count: count, Kind: "unsupported"})
	}
	for ann, count := range report.UnknownIngressAnnotations {
		rows = append(rows, blockingRow{Annotation: ann, Count: count, Kind: "unknown"})
	}

	slices.SortFunc(rows, func(a, b blockingRow) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return cmp.Compare(a.Annotation, b.Annotation)
	})

	return rows
}

// buildDetailRows turns the (already namespace/name-sorted) unsupported Ingresses
// into table rows listing the specific annotations a human must migrate.
func buildDetailRows(ingresses []analyzer.IngressReport) []detailRow {
	rows := make([]detailRow, 0, len(ingresses))

	for _, ing := range ingresses {
		fixes := make([]string, 0, len(ing.UnsupportedAnnotations)+len(ing.UnknownAnnotations))
		fixes = append(fixes, ing.UnsupportedAnnotations...)
		fixes = append(fixes, ing.UnknownAnnotations...)

		rows = append(rows, detailRow{
			Namespace: ing.Namespace,
			Name:      ing.Name,
			Class:     ing.IngressClassName,
			Fixes:     strings.Join(fixes, ", "),
		})
	}

	return rows
}

func formatPct(pct float64) string {
	return fmt.Sprintf("%.1f%%", pct)
}
