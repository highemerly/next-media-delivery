// Package cachekey computes deterministic cache keys shared by the HTTP handler,
// L1 cleanup goroutine, and the purge CLI.
package cachekey

import (
	"crypto/sha256"
	"fmt"
)

// Compute returns SHA256(url + "|" + variant) as a hex string.
// variant must be the canonical string from variant.Variant.String().
func Compute(rawURL, variant string) string {
	h := sha256.Sum256([]byte(rawURL + "|" + variant))
	return fmt.Sprintf("%x", h)
}
