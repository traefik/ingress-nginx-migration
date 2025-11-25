package handlers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/ingress-nginx-migration/pkg/analyzer"
)

//go:embed report.html
var htmlReportTemplate string

// Client is the interface for sending reports.
type Client interface {
	SendReport(reportPayload ReportPayload) error
}

// Analyzer is the interface for generating reports.
type Analyzer interface {
	GenerateReport() error
	Report() analyzer.Report
}

// Handlers holds handler configuration.
type Handlers struct {
	client     Client
	analyzer   Analyzer
	reportTmpl *template.Template
}

// New creates HTTP handlers.
func New(analyzr *analyzer.Analyzer, client Client) (*Handlers, error) {
	reportTmpl, err := template.New("report").Parse(htmlReportTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing report template: %w", err)
	}

	return &Handlers{
		client:     client,
		analyzer:   analyzr,
		reportTmpl: reportTmpl,
	}, nil
}

type reportVariables struct {
	analyzer.Report

	ReportJSON template.JS
	ReportHash string
}

// ReportPayload is a lightweight version of analyzer.Report for API transmission.
type ReportPayload struct {
	GenerationDate          time.Time `json:"generationDate"`
	IngressCount            int       `json:"ingressCount"`
	CompatibleIngressCount  int       `json:"compatibleIngressCount"`
	VanillaIngressCount     int       `json:"vanillaIngressCount"`
	SupportedIngressCount   int       `json:"supportedIngressCount"`
	UnsupportedIngressCount int       `json:"unsupportedIngressCount"`

	UnsupportedIngressAnnotations map[string]int `json:"unsupportedIngressAnnotations"`

	Version string `json:"version"`
}

// Report returns the HTML report for the Ingress NGINX migration.
func (h *Handlers) Report(rw http.ResponseWriter, _ *http.Request) {
	report := h.analyzer.Report()

	reportPayload := ReportPayload{
		GenerationDate:                report.GenerationDate,
		IngressCount:                  report.IngressCount,
		CompatibleIngressCount:        report.CompatibleIngressCount,
		VanillaIngressCount:           report.VanillaIngressCount,
		SupportedIngressCount:         report.SupportedIngressCount,
		UnsupportedIngressCount:       report.UnsupportedIngressCount,
		UnsupportedIngressAnnotations: report.UnsupportedIngressAnnotations,
		Version:                       report.Version,
	}

	reportJSON, err := json.Marshal(reportPayload)
	if err != nil {
		log.Err(err).Msg("Error while marshaling the report")
		JSONInternalServerError(rw)
		return
	}

	reportVars := reportVariables{
		Report:     report,
		ReportJSON: template.JS(reportJSON),
		ReportHash: report.Hash,
	}

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(http.StatusOK)

	if err := h.reportTmpl.Execute(rw, reportVars); err != nil {
		log.Err(err).Msg("Error while executing report template")
		JSONInternalServerError(rw)
		return
	}
}

// UpdateReport updates the analysis report.
func (h *Handlers) UpdateReport(rw http.ResponseWriter, _ *http.Request) {
	if err := h.analyzer.GenerateReport(); err != nil {
		log.Err(err).Msg("Error while updating the report")
		JSONInternalServerError(rw)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// SendReport sends the HTML report for the Ingress NGINX migration to Traefik Labs.
func (h *Handlers) SendReport(rw http.ResponseWriter, _ *http.Request) {
	report := h.analyzer.Report()

	reportPayload := ReportPayload{
		GenerationDate:                report.GenerationDate,
		IngressCount:                  report.IngressCount,
		CompatibleIngressCount:        report.CompatibleIngressCount,
		VanillaIngressCount:           report.VanillaIngressCount,
		SupportedIngressCount:         report.SupportedIngressCount,
		UnsupportedIngressCount:       report.UnsupportedIngressCount,
		UnsupportedIngressAnnotations: report.UnsupportedIngressAnnotations,
		Version:                       report.Version,
	}

	if err := h.client.SendReport(reportPayload); err != nil {
		log.Err(err).Msg("Error while sending the report")
		JSONInternalServerError(rw)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
