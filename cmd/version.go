package main

import (
	"context"
	"fmt"
	"os"

	"github.com/traefik/ingress-nginx-analyzer/pkg/version"
	"github.com/urfave/cli/v3"
)

func printVersion(ctx context.Context, cmd *cli.Command) error {
	if err := version.Print(os.Stdout); err != nil {
		return fmt.Errorf("printing version: %w", err)
	}
	fmt.Println()

	return nil
}
