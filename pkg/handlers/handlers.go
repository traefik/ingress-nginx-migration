package handlers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/traefik/ingress-nginx-analyzer/pkg/analyzer"
)

//go:embed report.html
var reportHTML string

// Client is the interface for sending reports.
type Client interface {
	SendReport(report analyzer.Report) error
}

// Analyzer is the interface for generating reports.
type Analyzer interface {
	Report() (analyzer.Report, error)
}

// Handlers holds handler configuration.
type Handlers struct {
	client     Client
	analyzer   Analyzer
	reportTmpl *template.Template
}

// NewHandlers creates HTTP handlers.
func NewHandlers(analyzer *analyzer.Analyzer, client Client) (*Handlers, error) {
	reportTmpl, err := template.New("report").Parse(reportHTML)
	if err != nil {
		return nil, fmt.Errorf("parsing report template: %w", err)
	}

	return &Handlers{
		client:     client,
		analyzer:   analyzer,
		reportTmpl: reportTmpl,
	}, nil
}

type reportVariables struct {
	analyzer.Report
	ReportJSON template.JS
}

// Report generates and returns the HTML report for the Ingress Nginx migration.
func (h *Handlers) Report(rw http.ResponseWriter, _ *http.Request) {
	report, err := h.analyzer.Report()
	if err != nil {
		log.Err(err).Msg("Error while generating the report")
		JSONInternalServerError(rw)
		return
	}

	reportJSON, err := json.Marshal(report)
	if err != nil {
		log.Err(err).Msg("Error while marshaling the report")
		JSONInternalServerError(rw)
		return
	}

	reportVars := reportVariables{
		Report:     report,
		ReportJSON: template.JS(reportJSON),
	}

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(http.StatusOK)

	if err := h.reportTmpl.Execute(rw, reportVars); err != nil {
		log.Err(err).Msg("Error while executing report template")
		JSONInternalServerError(rw)
		return
	}
}

// SendReport generates and sends the HTML report for the Ingress Nginx migration to Traefik Labs.
func (h *Handlers) SendReport(rw http.ResponseWriter, _ *http.Request) {
	report, err := h.analyzer.Report()
	if err != nil {
		log.Err(err).Msg("Error while generating the report")
		JSONInternalServerError(rw)
		return
	}

	if err := h.client.SendReport(report); err != nil {
		log.Err(err).Msg("Error while sending the report")
		JSONInternalServerError(rw)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
