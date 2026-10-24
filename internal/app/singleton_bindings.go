//go:build bindings

package app

// Wails executes main with the bindings build tag to generate its bridge.
// Code generation must not contact or exit through the running desktop app.
func EnsureSingleton(_ []string) bool { return true }
