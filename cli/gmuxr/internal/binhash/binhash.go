// Package binhash computes the sha256 hash of the running executable.
// Called once at startup; the result is cached for the process lifetime.
package binhash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sync"
)

var (
	once sync.Once
	hash string
)

// Self returns the hex-encoded sha256 hash of the current executable.
// Computed once, cached for the process lifetime. Returns "" on error.
func Self() string {
	once.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		f, err := os.Open(exe)
		if err != nil {
			return
		}
		defer f.Close()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return
		}
		hash = hex.EncodeToString(h.Sum(nil))
	})
	return hash
}
