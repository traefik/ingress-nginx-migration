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
	"github.com/urfave/cli/v3"
)

const (
	flagAddr       = "addr"
	flagLogLevel   = "log-level"
	flagKubeconfig = "kubeconfig"
)

// FIXME authentication
// FIXME authentify client to avoid multiple report
// FIXME version command and distribution
// FIXME add message with a link to open the web interface
func main() {
	cmd := &cli.Command{
		Name:  "Ingress Nginx Analyzer",
		Usage: "Analyze Nginx Ingresses to build a migration report to Traefik", // FIXME
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    flagAddr,
				Usage:   flagAddr,
				Sources: cli.EnvVars(strcase.ToSNAKE(flagAddr)),
				Value:   ":8080",
			},
			&cli.StringFlag{
				Name:    flagLogLevel,
				Usage:   flagLogLevel,
				Sources: cli.EnvVars(strcase.ToSNAKE(flagLogLevel)),
				Value:   "info",
			},
			&cli.StringFlag{
				Name:    flagKubeconfig,
				Usage:   flagKubeconfig,
				Sources: cli.EnvVars(strcase.ToSNAKE(flagKubeconfig)),
			},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return run(ctx, command)
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	logger.Setup(cmd.String(flagLogLevel))

	analyzer, err := analyzer.New(ctx, cmd.String(flagKubeconfig))
	if err != nil {
		return fmt.Errorf("creating analyzer: %w", err)
	}

	client := client.New("http://127.0.0.1:8080")

	hdl := handlers.NewHandlers(analyzer, client)

	router := httprouter.New()
	router.HandlerFunc(http.MethodGet, "/report", hdl.Report)
	router.HandlerFunc(http.MethodPut, "/send-report", hdl.SendReport)

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
