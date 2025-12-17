package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ettle/strcase"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
	"github.com/traefik/ingress-nginx-migration/pkg/analyzer"
	"github.com/traefik/ingress-nginx-migration/pkg/client"
	"github.com/traefik/ingress-nginx-migration/pkg/handlers"
	"github.com/traefik/ingress-nginx-migration/pkg/logger"
	"github.com/urfave/cli/v3"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	flagAddr               = "addr"
	flagKubeconfig         = "kubeconfig"
	flagNamespaces         = "namespaces"
	flagIngressClass       = "ingress-class"
	flagControllerClass    = "controller-class"
	flagWatchWithoutClass  = "watch-ingress-without-class"
	flagIngressClassByName = "ingress-class-by-name"
)

func main() {
	cmd := &cli.Command{
		Name:  "ingress-nginx-migration",
		Usage: "Analyze NGINX Ingresses to build a migration report to Traefik",
		Commands: []*cli.Command{
			{
				Name:   "version",
				Usage:  "Shows the current version",
				Action: printVersion,
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
				Name:    flagKubeconfig,
				Usage:   "Defines the kubeconfig file to use to connect to the Kubernetes cluster.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagKubeconfig)),
			},
			&cli.StringSliceFlag{
				Name:    flagNamespaces,
				Usage:   "Defines the namespaces to analyze. When empty, all namespaces are analyzed.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagNamespaces)),
			},
			&cli.StringFlag{
				Name:    flagIngressClass,
				Usage:   "Defines the name of the ingress class this controller satisfies.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagIngressClass)),
			},
			&cli.StringFlag{
				Name:    flagControllerClass,
				Usage:   "Defines the Ingress Controller class to analyze. When empty, 'k8s.io/ingress-nginx' is used.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagControllerClass)),
			},
			&cli.BoolFlag{
				Name:    flagWatchWithoutClass,
				Usage:   "Defines if Ingress Controller should also watch for Ingresses without an IngressClass or the annotation specified.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagWatchWithoutClass)),
			},
			&cli.BoolFlag{
				Name:    flagIngressClassByName,
				Usage:   "Defines if Ingress Controller should watch for Ingress Class by Name together with Controller Class.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagIngressClassByName)),
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	logger.Setup("info")

	// Creates the Kubernetes client.
	config, err := rest.InClusterConfig()
	if err != nil && !errors.Is(err, rest.ErrNotInCluster) {
		return fmt.Errorf("creating in cluster config: %w", err)
	}
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", cmd.String(flagKubeconfig))
		if err != nil {
			return fmt.Errorf("creating config from flags: %w", err)
		}
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating k8s client from config: %w", err)
	}

	// Creates and starts the analyzer and generates the report.
	analyzr, err := analyzer.New(k8sClient, cmd.StringSlice(flagNamespaces), cmd.String(flagControllerClass), cmd.Bool(flagWatchWithoutClass), cmd.String(flagIngressClass), cmd.Bool(flagIngressClassByName))
	if err != nil {
		return fmt.Errorf("creating analyzer: %w", err)
	}

	if err = analyzr.Start(ctx); err != nil {
		return fmt.Errorf("starting analyzer: %w", err)
	}

	if err = analyzr.GenerateReport(); err != nil {
		return fmt.Errorf("generating report: %w", err)
	}

	// Creates the platform client.
	clt, err := client.New(os.Getenv("ENDPOINT_STATS_URL"))
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Creates the HTTP server.
	hdl, err := handlers.New(analyzr, clt)
	if err != nil {
		return fmt.Errorf("creating handlers: %w", err)
	}

	router := httprouter.New()
	router.HandlerFunc(http.MethodPut, "/update", hdl.UpdateReport)
	router.HandlerFunc(http.MethodPut, "/send", hdl.SendReport)
	router.HandlerFunc(http.MethodGet, "/", hdl.Report)

	addr := cmd.String(flagAddr)
	errCh := make(chan error)
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	reportAddr := addr
	if strings.HasPrefix(addr, ":") {
		reportAddr = "localhost" + addr
	}

	go func() {
		log.Info().Msg("Starting Ingress NGINX analyzer server")
		log.Info().Msgf("Please browse the Ingress NGINX analyzer report on: http://%s", reportAddr)

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
