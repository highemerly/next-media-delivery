package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	var adminPort int
	var showCB bool
	var showNC bool

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show cache and in-memory state statistics",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := fmt.Sprintf("http://127.0.0.1:%d", adminPort)
			endpoint := base + "/stats"
			if showCB {
				endpoint = base + "/stats/circuit-breaker"
			} else if showNC {
				endpoint = base + "/stats/negative-cache"
			}
			return printJSON(endpoint)
		},
	}

	cmd.Flags().IntVar(&adminPort, "admin-port", 3001, "Admin server port")
	cmd.Flags().BoolVar(&showCB, "circuit-breaker", false, "Show Circuit Breaker details")
	cmd.Flags().BoolVar(&showNC, "negative-cache", false, "Show Negative Cache details")
	return cmd
}

func printJSON(url string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("cannot reach admin server: %w\nIs the server running?", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Pretty-print JSON.
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		os.Stdout.Write(body) //nolint:errcheck
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
