package main

import (
	"context"
	"fmt"
	"os"

	"github.com/traefik/ingress-nginx-migration/pkg/version"
	"github.com/urfave/cli/v3"
)

func printVersion(_ context.Context, _ *cli.Command) error {
	if err := version.Print(os.Stdout); err != nil {
		return fmt.Errorf("printing version: %w", err)
	}

	return nil
}
