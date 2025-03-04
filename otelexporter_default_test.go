package otelexporter_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jzx17/otelexporter"
)

var _ = Describe("Default Exporter", func() {
	const (
		shortTimeout = 5 * time.Millisecond
	)

	BeforeEach(func() {
		// Reset the default exporter for each test
		otelexporter.ResetDefaultForTest()

		// Clean up any environment variables that might affect tests
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	})

	AfterEach(func() {
		// Clean up environment variables
		os.Unsetenv("OTEL_SERVICE_NAME")
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	})

	It("should initialize a default exporter", func() {
		// Create a short timeout context
		ctx, cancel := context.WithTimeout(context.Background(), shortTimeout*2)
		defer cancel()

		// Create a config that won't try to connect
		cfg := otelexporter.DefaultConfig()
		cfg.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
		cfg.Timeout = shortTimeout
		cfg.BatchTimeout = shortTimeout
		cfg.MaxExportBatchSize = 1

		err := otelexporter.InitDefault(ctx,
			otelexporter.WithConfig(cfg),
		)

		// Verify initialization attempted
		if err != nil {
			Expect(err.Error()).To(ContainSubstring("failed to create"))
		} else {
			defaultExp := otelexporter.Default()
			Expect(defaultExp).NotTo(BeNil())

			// Clean up
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer shutdownCancel()
			_ = defaultExp.Shutdown(shutdownCtx)
		}
	})

	It("should panic if Default() called before InitDefault()", func() {
		otelexporter.ResetDefaultForTest()

		Expect(func() {
			_ = otelexporter.Default()
		}).To(Panic())
	})

	It("should initialize with provided options", func() {
		// Create a short timeout context
		ctx, cancel := context.WithTimeout(context.Background(), shortTimeout*2)
		defer cancel()

		testLogger := newTestLogger()

		// Create a config that won't try to connect
		cfg := otelexporter.DefaultConfig()
		cfg.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
		cfg.Timeout = shortTimeout
		cfg.BatchTimeout = shortTimeout
		cfg.MaxExportBatchSize = 1

		err := otelexporter.InitDefault(ctx,
			otelexporter.WithLogger(testLogger),
			otelexporter.WithConfig(cfg),
		)

		// Verify initialization with options
		if err != nil {
			Expect(err.Error()).To(ContainSubstring("failed to create"))
		} else {
			defaultExp := otelexporter.Default()
			Expect(defaultExp).NotTo(BeNil())

			// Verify the logger was set correctly
			Expect(defaultExp.Logger()).To(Equal(testLogger))

			// Log a test message and verify it was recorded
			defaultExp.Logger().Info("test message")
			Expect(testLogger.infoMessages).To(ContainElement("test message"))

			// Clean up
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer shutdownCancel()
			_ = defaultExp.Shutdown(shutdownCtx)
		}
	})

	// Tests for new default exporter functions
	Describe("New Default Exporter Functions", func() {
		It("should check IsDefaultInitialized", func() {
			// Initially not initialized
			Expect(otelexporter.IsDefaultInitialized()).To(BeFalse())

			// Set up a simple configuration
			ctx, cancel := context.WithTimeout(context.Background(), shortTimeout*2)
			defer cancel()

			cfg := otelexporter.DefaultConfig()
			cfg.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
			cfg.Timeout = shortTimeout

			// Initialize default exporter, ignoring errors
			_ = otelexporter.InitDefault(ctx, otelexporter.WithConfig(cfg))

			// Now it should be initialized
			Expect(otelexporter.IsDefaultInitialized()).To(BeTrue())

			// Clean up if needed
			if otelexporter.IsDefaultInitialized() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
				defer shutdownCancel()
				_ = otelexporter.ShutdownDefault(shutdownCtx)
			}
		})

		It("should use InitDefaultWithEnvVars", func() {
			// Set environment variables
			const testServiceName = "env-var-service"
			const testEndpoint = "localhost:9999"

			os.Setenv("OTEL_SERVICE_NAME", testServiceName)
			os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", testEndpoint)

			// Create a context with short timeout
			ctx, cancel := context.WithTimeout(context.Background(), shortTimeout*2)
			defer cancel()

			// Initialize with environment variables
			err := otelexporter.InitDefaultWithEnvVars(ctx)

			// Check initialization
			if err != nil {
				// If it failed due to connection issues, that's expected
				Expect(err.Error()).To(Or(
					ContainSubstring("failed to create"),
					ContainSubstring("connection"),
				))
			} else {
				// If successful, verify the settings were read from env vars
				defaultExp := otelexporter.Default()
				Expect(defaultExp).NotTo(BeNil())

				// We can't directly access config fields, but we can check indirectly
				// by examining traces or checking behavior

				// Clean up
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
				defer shutdownCancel()
				_ = defaultExp.Shutdown(shutdownCtx)
			}
		})

		It("should use GetOrInitDefault for lazy initialization", func() {
			// First, ensure it's not initialized
			otelexporter.ResetDefaultForTest()

			// Create a context with short timeout
			ctx, cancel := context.WithTimeout(context.Background(), shortTimeout*2)
			defer cancel()

			// Create a config that will fail quickly
			cfg := otelexporter.DefaultConfig()
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout

			// First attempt should try to initialize
			exp1, err := otelexporter.GetOrInitDefault(ctx, otelexporter.WithConfig(cfg))

			if err != nil {
				// If initialization failed (expected), the error should be specific
				Expect(err.Error()).To(ContainSubstring("failed to initialize default exporter"))
			} else {
				// If it succeeded, we should get a valid exporter
				Expect(exp1).NotTo(BeNil())

				// Second call should return the same instance
				exp2, err := otelexporter.GetOrInitDefault(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(exp2).To(Equal(exp1))

				// Clean up
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
				defer shutdownCancel()
				_ = exp1.Shutdown(shutdownCtx)
			}
		})
	})
})
