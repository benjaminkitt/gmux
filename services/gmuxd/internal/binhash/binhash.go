// Package binhash computes sha256 hashes of binary files.
package binhash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// File returns the hex-encoded sha256 hash of the file at path.
// Returns "" on error.
func File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
