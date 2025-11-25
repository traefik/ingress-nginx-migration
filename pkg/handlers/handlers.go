package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/traefik/ingress-nginx-analyzer/pkg/analyzer"
)

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
	client   Client
	analyzer Analyzer
}

// NewHandlers creates HTTP handlers.
func NewHandlers(analyzer *analyzer.Analyzer, client Client) *Handlers {
	return &Handlers{
		client:   client,
		analyzer: analyzer,
	}
}

// Report generates and returns the HTML report for the Ingress Nginx migration.
func (h *Handlers) Report(rw http.ResponseWriter, _ *http.Request) {
	report, err := h.analyzer.Report()
	if err != nil {
		log.Err(err).Msg("Error while generating the report")
		JSONInternalServerError(rw)
		return
	}

	rw.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(rw).Encode(report); err != nil {
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
