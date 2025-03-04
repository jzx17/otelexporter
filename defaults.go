package otelexporter

import (
	"context"
	"errors"
	"sync"
)

var (
	defaultExporter *Exporter
	once            sync.Once
	initErr         error
)

// InitDefault initializes the default exporter with the given options.
// This should be called early in the application lifecycle.
func InitDefault(ctx context.Context, opts ...Option) error {
	once.Do(func() {
		var err error
		defaultExporter, err = NewExporter(ctx, opts...)
		if err != nil {
			initErr = err
		}
	})
	return initErr
}

// Default returns the default exporter. If it hasn't been initialized,
// it will panic. Use InitDefault to initialize the default exporter.
func Default() *Exporter {
	if defaultExporter == nil {
		panic("default exporter not initialized, call InitDefault first")
	}
	return defaultExporter
}

// IsDefaultInitialized checks if the default exporter has been initialized.
// This can be useful to avoid panics when accessing the default exporter.
func IsDefaultInitialized() bool {
	return defaultExporter != nil
}

// ShutdownDefault gracefully shuts down the default exporter if it exists.
// It's safe to call this method even if the default exporter hasn't been initialized.
func ShutdownDefault(ctx context.Context) error {
	if defaultExporter == nil {
		return nil
	}
	return defaultExporter.Shutdown(ctx)
}

// ResetDefaultForTest resets the default exporter to nil, allowing for a new exporter
// to be initialized. This function should only be used in testing scenarios.
func ResetDefaultForTest() {
	defaultExporter = nil
	once = sync.Once{}
	initErr = nil
}

// Alias for backward compatibility
// ResetDefault resets the default exporter to nil, allowing for a new exporter
// to be initialized. This should generally only be used in testing scenarios.
func ResetDefault() {
	ResetDefaultForTest()
}

// InitDefaultWithEnvVars initializes the default exporter using configuration from environment variables.
// This is a convenience function that loads configuration from the environment and then calls InitDefault.
func InitDefaultWithEnvVars(ctx context.Context, opts ...Option) error {
	// Create a config and load from environment
	cfg := DefaultConfig()
	cfg.FromEnv()

	// Add the config to the options
	configOpt := WithConfig(cfg)
	allOpts := append([]Option{configOpt}, opts...)

	return InitDefault(ctx, allOpts...)
}

// GetOrInitDefault returns the default exporter if it's initialized,
// or attempts to initialize it with the provided options if it's not.
// This is useful for lazy initialization of the default exporter.
func GetOrInitDefault(ctx context.Context, opts ...Option) (*Exporter, error) {
	if defaultExporter != nil {
		return defaultExporter, nil
	}

	// Try to initialize the default exporter
	if err := InitDefault(ctx, opts...); err != nil {
		return nil, errors.New("failed to initialize default exporter: " + err.Error())
	}

	return defaultExporter, nil
}
