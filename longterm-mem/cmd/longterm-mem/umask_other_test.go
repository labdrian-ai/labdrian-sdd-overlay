//go:build !unix

package main

import "testing"

// fixedUmask has nothing to fix where there is no umask.
func fixedUmask(t *testing.T) { t.Helper() }
