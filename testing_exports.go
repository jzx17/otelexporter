package otelexporter

import "sync"

// ResetDefaultForTest resets the default exporter for testing purposes.
// This function should only be used in tests.
func ResetDefaultForTest() {
	// Reset the global variables
	defaultExporter = nil
	once = sync.Once{}
	initErr = nil
}
