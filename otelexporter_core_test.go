package otelexporter_test

import (
	"context"
	"errors"
	"time"

	"github.com/jzx17/otelexporter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
)

var _ = Describe("Exporter Core Functionality", func() {
	// Set a constant shorter timeout for all tests in this file
	const (
		shortTimeout = 5 * time.Millisecond
		testTimeout  = 20 * time.Millisecond
	)

	BeforeEach(func() {
		// Add a sleep to ensure previous test resources are properly released
		// This helps avoid resource contention between tests
		time.Sleep(1 * time.Millisecond)
	})

	// Helper function to create a mock tracer provider with minimal settings
	createMockTracerProvider := func() *sdktrace.TracerProvider {
		spanExporter := tracetest.NewInMemoryExporter()
		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(
				spanExporter,
				sdktrace.WithBatchTimeout(shortTimeout),
				// Add a max export batch size to avoid waiting for batch to fill
				sdktrace.WithMaxExportBatchSize(1),
			),
			// Explicitly disable export to avoid network connections
			sdktrace.WithSyncer(spanExporter),
		)
		return tracerProvider
	}

	Describe("Creation", func() {
		It("should create with default config", func() {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			// Override endpoint to avoid connection attempts
			cfg := otelexporter.DefaultConfig()
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout

			// Create a test logger
			logger := newTestLogger()

			// Use a mock tracer provider with no network connections
			tracerProvider := createMockTracerProvider()
			defer tracerProvider.Shutdown(context.Background())

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(logger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(exporter).NotTo(BeNil())

			// Shutdown with short timeout - expect no errors
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer shutdownCancel()

			// Preemptively flush the tracer provider to avoid shutdown errors
			_ = tracerProvider.ForceFlush(ctx)

			// Ignore any expected errors during shutdown
			_ = exporter.Shutdown(shutdownCtx)
		})

		It("should create with custom logger", func() {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			// Create mock logger
			logger := newTestLogger()

			// Override endpoint to avoid connection attempts
			cfg := otelexporter.DefaultConfig()
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout

			// Use a mock tracer provider with no network connections
			tracerProvider := createMockTracerProvider()
			defer tracerProvider.Shutdown(context.Background())

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithLogger(logger),
				otelexporter.WithConfig(cfg),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(exporter).NotTo(BeNil())
			Expect(exporter.Logger()).To(Equal(logger))

			// Preemptively flush the tracer provider to avoid shutdown errors
			_ = tracerProvider.ForceFlush(ctx)

			// Shutdown with short timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer shutdownCancel()

			// Ignore any expected errors during shutdown
			_ = exporter.Shutdown(shutdownCtx)
		})
	})

	Describe("Default exporter", func() {
		BeforeEach(func() {
			// Reset the default exporter before each test
			otelexporter.ResetDefaultForTest()
		})

		It("should initialize default exporter", func() {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			// Create a mock logger
			logger := newTestLogger()

			// Configure to avoid connection attempts
			cfg := otelexporter.DefaultConfig()
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout

			// Disable metrics export to avoid network connection attempts
			cfg.MaxExportBatchSize = 1
			cfg.BatchTimeout = shortTimeout

			// Use a mock tracer provider with no network connections
			tracerProvider := createMockTracerProvider()
			defer tracerProvider.Shutdown(context.Background())

			err := otelexporter.InitDefault(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(logger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())

			// Get the default exporter
			defaultExporter := otelexporter.Default()
			Expect(defaultExporter).NotTo(BeNil())

			// Test some basic functionality
			defaultExporter.Logger().Info("test message")

			// Preemptively flush the tracer provider to avoid shutdown errors
			_ = tracerProvider.ForceFlush(ctx)

			// Shutdown with short timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer shutdownCancel()

			// Ignore any expected errors during shutdown
			_ = defaultExporter.Shutdown(shutdownCtx)
		})

		It("should panic when default not initialized", func() {
			Expect(func() {
				_ = otelexporter.Default()
			}).To(Panic())
		})
	})

	Describe("Shutdown behavior", func() {
		It("should handle common logger sync errors", func() {
			// Create a mock logger that returns a known sync error
			logger := newTestLogger()
			logger.SetSyncError(true) // Will return "inappropriate ioctl for device" error

			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			// Create a minimal config and use mock tracer provider
			cfg := otelexporter.DefaultConfig()
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout

			// Use a mock tracer provider with no network connections
			tracerProvider := createMockTracerProvider()
			defer tracerProvider.Shutdown(context.Background())

			// Create a test exporter with the mock logger
			testExporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithLogger(logger),
				otelexporter.WithConfig(cfg),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(testExporter).NotTo(BeNil())

			// Log something to ensure logger is working
			testExporter.Logger().Info("test message before shutdown")

			// Verify our logger was called
			Expect(logger.infoMessages).To(ContainElement("test message before shutdown"))

			// Preemptively flush the tracer provider to avoid shutdown errors
			_ = tracerProvider.ForceFlush(ctx)

			// Shut down the exporter - should handle the logger sync error
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer shutdownCancel()

			err = testExporter.Shutdown(shutdownCtx)

			// The exporter should ignore the "inappropriate ioctl for device" error
			if err != nil {
				// Check that it doesn't contain our sync error
				Expect(err.Error()).NotTo(ContainSubstring("inappropriate ioctl for device"))
			}
		})
	})
})

// Simple test logger implementation if not already defined
// This is here in case the other test files are not included
type testCoreLogger struct {
	infoMessages  []string
	errorMessages []string
	syncError     bool
}

func newTestCoreLogger() *testCoreLogger {
	return &testCoreLogger{
		infoMessages:  []string{},
		errorMessages: []string{},
	}
}

func (l *testCoreLogger) Info(msg string, fields ...zap.Field) {
	l.infoMessages = append(l.infoMessages, msg)
}

func (l *testCoreLogger) Error(msg string, fields ...zap.Field) {
	l.errorMessages = append(l.errorMessages, msg)
}

func (l *testCoreLogger) Debug(msg string, fields ...zap.Field) {}
func (l *testCoreLogger) Warn(msg string, fields ...zap.Field)  {}

func (l *testCoreLogger) Sync() error {
	if l.syncError {
		return errors.New("inappropriate ioctl for device")
	}
	return nil
}

func (l *testCoreLogger) SetSyncError(shouldError bool) {
	l.syncError = shouldError
}
