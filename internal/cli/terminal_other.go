//go:build !darwin && !linux

package cli

// Unsupported platforms use plain text rather than risking escape codes in files.
func isTerminal(fd uintptr) bool { return false }
