package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/highemerly/media-delivery/internal/cachekey"
	"github.com/highemerly/media-delivery/internal/format"
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
			// Purge both WebP and AVIF entries for the given variant.
			for _, f := range []format.OutputFormat{format.WebP, format.AVIF} {
				key := cachekey.Compute(rawURL, variantStr, f.String())
				if err := purgeKey(base, key); err != nil {
					fmt.Printf("skip %s/%s: %v\n", variantStr, f.String(), err)
				}
			}
			return nil
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
	formats := []format.OutputFormat{format.WebP, format.AVIF}
	for _, v := range variants {
		for _, f := range formats {
			key := cachekey.Compute(rawURL, v.String(), f.String())
			if err := purgeKey(base, key); err != nil {
				fmt.Printf("skip %s/%s: %v\n", v.String(), f.String(), err)
			}
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
