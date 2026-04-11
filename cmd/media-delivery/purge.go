package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/highemerly/media-delivery/internal/cachekey"
	"github.com/highemerly/media-delivery/internal/variant"
)

func newPurgeCmd() *cobra.Command {
	var adminPort int
	var rawURL string
	var variantStr string
	var allVariants bool
	var all bool

	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge cached items from L1 (and L2 if enabled)",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := fmt.Sprintf("http://127.0.0.1:%d", adminPort)

			if all {
				return purgeAll(base)
			}
			if rawURL == "" {
				return fmt.Errorf("--url is required (or use --all)")
			}
			if allVariants {
				return purgeAllVariants(base, rawURL)
			}
			if variantStr == "" {
				return fmt.Errorf("--variant is required (or use --all-variants / --all)")
			}
			key := cachekey.Compute(rawURL, variantStr)
			return purgeKey(base, key)
		},
	}

	cmd.Flags().IntVar(&adminPort, "admin-port", 3001, "Admin server port")
	cmd.Flags().StringVar(&rawURL, "url", "", "Origin URL to purge")
	cmd.Flags().StringVar(&variantStr, "variant", "", "Variant to purge (raw/emoji/avatar/preview/badge/static)")
	cmd.Flags().BoolVar(&allVariants, "all-variants", false, "Purge all variants for --url")
	cmd.Flags().BoolVar(&all, "all", false, "Purge entire cache")
	return cmd
}

func purgeKey(base, key string) error {
	req, _ := http.NewRequest(http.MethodDelete, base+"/cache/"+key, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach admin server: %w\nIs the server running?", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("purged: %s\n", key)
		return nil
	}
	return fmt.Errorf("server returned %d", resp.StatusCode)
}

func purgeAllVariants(base, rawURL string) error {
	variants := []variant.Variant{
		variant.Raw, variant.Emoji, variant.Avatar,
		variant.Preview, variant.Badge, variant.Static,
	}
	for _, v := range variants {
		key := cachekey.Compute(rawURL, v.String())
		if err := purgeKey(base, key); err != nil {
			fmt.Printf("skip %s: %v\n", v.String(), err)
		}
	}
	return nil
}

func purgeAll(base string) error {
	req, _ := http.NewRequest(http.MethodDelete, base+"/cache", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach admin server: %w\nIs the server running?", err)
	}
	defer resp.Body.Close()
	return printJSON(base + "/stats")
}
