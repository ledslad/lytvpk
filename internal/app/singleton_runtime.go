//go:build !bindings

package app

import "os"

// EnsureSingleton forwards protocol arguments to an existing instance and exits.
func EnsureSingleton(args []string) bool {
	if !IsFirstInstance(args) {
		os.Exit(0)
	}
	return true
}
