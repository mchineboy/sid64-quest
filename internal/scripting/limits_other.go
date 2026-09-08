//go:build !linux

package scripting

// Development hosts retain the wall-clock and step limits and Go memory target.
// Production Linux workers additionally have kernel CPU/address-space limits.
func limitWorker() error { return nil }
