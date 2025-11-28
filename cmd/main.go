package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/ettle/strcase"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
	"github.com/traefik/ingress-nginx-analyzer/pkg/analyzer"
	"github.com/traefik/ingress-nginx-analyzer/pkg/client"
	"github.com/traefik/ingress-nginx-analyzer/pkg/handlers"
	"github.com/traefik/ingress-nginx-analyzer/pkg/logger"
	"github.com/traefik/ingress-nginx-analyzer/pkg/version"
	"github.com/urfave/cli/v3"
)

const (
	flagAddr            = "addr"
	flagLogLevel        = "log-level"
	flagKubeconfig      = "kubeconfig"
	flagNamespaces      = "namespaces"
	flagControllerClass = "controller-class"
)

// FIXME authentication.
// FIXME authentify client to avoid multiple report.
// FIXME add message with a link to open the web interface.
func main() {
	cmd := &cli.Command{
		Name:  "ingress-nginx-analyzer",
		Usage: "Analyze Nginx Ingresses to build a migration report to Traefik",
		Commands: []*cli.Command{
			{
				Name:  "version",
				Usage: "Shows the current version",
				Action: func(_ context.Context, _ *cli.Command) error {
					if err := version.Print(os.Stdout); err != nil {
						return err
					}
					fmt.Println()
					return nil
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    flagAddr,
				Usage:   "Defines the address to listen on for serving the migration report.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagAddr)),
				Value:   ":8080",
			},
			&cli.StringFlag{
				Name:    flagLogLevel,
				Usage:   "Defines the log level.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagLogLevel)),
				Value:   "info",
			},
			&cli.StringFlag{
				Name:    flagKubeconfig,
				Usage:   "Defines the kubeconfig file to use to connect to the Kubernetes cluster.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagKubeconfig)),
			},
			&cli.StringSliceFlag{
				Name:    flagNamespaces,
				Usage:   "Defines the namespaces to analyze. When empty, all namespaces are analyzed.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagNamespaces)),
			},
			&cli.StringSliceFlag{
				Name:    flagControllerClass,
				Usage:   "Defines the Ingress Controller class to analyze. When empty, 'k8s.io/ingress-nginx' is used.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagControllerClass)),
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	logger.Setup(cmd.String(flagLogLevel))

	analyzer, err := analyzer.New(ctx, cmd.String(flagKubeconfig), cmd.StringSlice(flagNamespaces), cmd.String(flagControllerClass))
	if err != nil {
		return fmt.Errorf("creating analyzer: %w", err)
	}

	endpointURL := os.Getenv("ENDPOINT_STATS_URL")
	if endpointURL == "" {
		endpointURL = "https://collect.ingressnginxmigration.org/a2181946f5561e7e7405000e5c94de97"
	}
	client, err := client.New(endpointURL)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	hdl, err := handlers.NewHandlers(analyzer, client)
	if err != nil {
		return fmt.Errorf("creating handlers: %w", err)
	}

	router := httprouter.New()
	router.HandlerFunc(http.MethodPut, "/send-report", hdl.SendReport)
	router.HandlerFunc(http.MethodGet, "/", hdl.Report)

	addr := cmd.String(flagAddr)
	errCh := make(chan error)
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("Starting Ingress Nginx analyzer server")
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		_ = server.Close()
	case err := <-errCh:
		return fmt.Errorf("analyzer server error: %w", err)
	}

	return nil
}
