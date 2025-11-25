package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/traefik/ingress-nginx-analyzer/pkg/analyzer"
)

// Token is the authentication token used for sending reports injected at build time.
var Token = "dev"

type Client struct {
	endpointURL string
	httpClient  *http.Client
}

func New(endpointURL string) *Client {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	return &Client{
		endpointURL: endpointURL,
		httpClient:  httpClient,
	}
}

func (c *Client) SendReport(report analyzer.Report) error {
	reportBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshalling report to JSON: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.endpointURL+"/fixme", bytes.NewBuffer(reportBytes)) // FIXME
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+Token)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting report to %s: %w", c.endpointURL, err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode/100 != 2 {
		return fmt.Errorf("invalid response status code: %d", res.StatusCode)
	}

	return nil
}
