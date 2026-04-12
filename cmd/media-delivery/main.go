package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "media-delivery",
		Short: "Misskey-compatible media proxy",
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newStatsCmd())
	root.AddCommand(newPurgeCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
