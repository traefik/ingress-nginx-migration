package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/traefik/ingress-nginx-migration/pkg/render"
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
	flagFormat             = "format"
	flagOutputFile         = "output-file"
	flagSummary            = "summary"
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
			&cli.StringFlag{
				Name:    flagFormat,
				Usage:   "Output the report once in this format ('json' or 'markdown') and exit, instead of serving the HTML report. When empty, the HTML report is served.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagFormat)),
			},
			&cli.StringFlag{
				Name:    flagOutputFile,
				Usage:   "Write the one-shot report to this file instead of stdout. Requires --format. Overwrites an existing file.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagOutputFile)),
			},
			&cli.BoolFlag{
				Name:    flagSummary,
				Usage:   "Omit the per-Ingress detail from the report. Only valid with --format markdown.",
				Sources: cli.EnvVars(strcase.ToSNAKE(flagSummary)),
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("Error while executing command")
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	format := cmd.String(flagFormat)
	summary := cmd.Bool(flagSummary)
	outputFile := cmd.String(flagOutputFile)

	oneShot := format != ""

	// In one-shot mode stdout is reserved for the report, so logs go to stderr.
	logOut := io.Writer(os.Stdout)
	if oneShot {
		logOut = os.Stderr
	}
	logger.Setup("info", logOut)

	if err := validateOutputFlags(format, summary, outputFile); err != nil {
		return err
	}

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

	// One-shot mode: write the report once and exit without serving.
	if oneShot {
		return writeReport(analyzr.Report(), format, summary, outputFile)
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

// validateOutputFlags rejects flag combinations that don't make sense for the
// one-shot output mode.
func validateOutputFlags(format string, summary bool, outputFile string) error {
	switch format {
	case "", render.FormatJSON, render.FormatMarkdown:
	default:
		return fmt.Errorf("invalid --%s %q (must be %q or %q)", flagFormat, format, render.FormatJSON, render.FormatMarkdown)
	}

	if format == "" && outputFile != "" {
		return fmt.Errorf("--%s requires --%s", flagOutputFile, flagFormat)
	}

	if summary && format != render.FormatMarkdown {
		return fmt.Errorf("--%s is only valid with --%s %s", flagSummary, flagFormat, render.FormatMarkdown)
	}

	return nil
}

// writeReport renders the report to outputFile, or to stdout when outputFile is empty.
func writeReport(report analyzer.Report, format string, summary bool, outputFile string) error {
	if outputFile == "" {
		return render.Render(report, format, summary, os.Stdout)
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	if err := render.Render(report, format, summary, f); err != nil {
		_ = f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("closing output file: %w", err)
	}

	return nil
}
