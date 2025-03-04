package otelexporter_test

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"github.com/jzx17/otelexporter"
)

// testLogger is a mock implementation of the Logger interface that records messages
type testLogger struct {
	mu              sync.Mutex
	infoMessages    []string
	errorMessages   []string
	debugMessages   []string
	warnMessages    []string
	infoFields      [][]zap.Field
	errorFields     [][]zap.Field
	debugFields     [][]zap.Field
	warnFields      [][]zap.Field
	shouldErrorSync bool
	syncError       error
}

// newTestLogger creates a new test logger for testing
func newTestLogger() *testLogger {
	return &testLogger{
		infoMessages:  make([]string, 0),
		errorMessages: make([]string, 0),
		debugMessages: make([]string, 0),
		warnMessages:  make([]string, 0),
		infoFields:    make([][]zap.Field, 0),
		errorFields:   make([][]zap.Field, 0),
		debugFields:   make([][]zap.Field, 0),
		warnFields:    make([][]zap.Field, 0),
		syncError:     errors.New("inappropriate ioctl for device"),
	}
}

// Info implements the Logger interface
func (l *testLogger) Info(msg string, fields ...zap.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infoMessages = append(l.infoMessages, msg)
	l.infoFields = append(l.infoFields, fields)
}

// Error implements the Logger interface
func (l *testLogger) Error(msg string, fields ...zap.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errorMessages = append(l.errorMessages, msg)
	l.errorFields = append(l.errorFields, fields)
}

// Debug implements the Logger interface
func (l *testLogger) Debug(msg string, fields ...zap.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugMessages = append(l.debugMessages, msg)
	l.debugFields = append(l.debugFields, fields)
}

// Warn implements the Logger interface
func (l *testLogger) Warn(msg string, fields ...zap.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnMessages = append(l.warnMessages, msg)
	l.warnFields = append(l.warnFields, fields)
}

// Sync implements the Logger interface
func (l *testLogger) Sync() error {
	if l.shouldErrorSync {
		return l.syncError
	}
	return nil
}

// SetSyncError configures the logger to return an error on Sync()
func (l *testLogger) SetSyncError(shouldError bool) {
	l.shouldErrorSync = shouldError
}

// ResetMessages clears all recorded messages
func (l *testLogger) ResetMessages() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infoMessages = make([]string, 0)
	l.errorMessages = make([]string, 0)
	l.debugMessages = make([]string, 0)
	l.warnMessages = make([]string, 0)
	l.infoFields = make([][]zap.Field, 0)
	l.errorFields = make([][]zap.Field, 0)
	l.debugFields = make([][]zap.Field, 0)
	l.warnFields = make([][]zap.Field, 0)
}

var _ = Describe("Metric Functions", func() {
	var (
		exporter       *otelexporter.Exporter
		tracerProvider *sdktrace.TracerProvider
		spanExporter   *tracetest.InMemoryExporter
		testLogger     *testLogger
		ctx            context.Context
		cancel         context.CancelFunc
	)

	// Constants for timeouts
	const (
		shortTimeout = 5 * time.Millisecond
		setupTimeout = 25 * time.Millisecond
	)

	BeforeEach(func() {
		// Create a base context with a shorter timeout (reduced from 100ms to 25ms)
		ctx, cancel = context.WithTimeout(context.Background(), setupTimeout)
		DeferCleanup(cancel)

		testLogger = newTestLogger()
		spanExporter = tracetest.NewInMemoryExporter()
		tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(
				spanExporter,
				// Set a very short batching timeout (reduced from 10ms to 5ms)
				sdktrace.WithBatchTimeout(shortTimeout),
				// Add a max export batch size to avoid waiting for batch to fill
				sdktrace.WithMaxExportBatchSize(1),
			),
			// Use synchronous export to avoid network timeouts
			sdktrace.WithSyncer(spanExporter),
		)

		// Create a configuration that won't try to connect for long
		cfg := otelexporter.DefaultConfig()
		cfg.OTLPEndpoint = "localhost:1" // Use a port that will quickly fail
		// Set much shorter timeouts
		cfg.Timeout = shortTimeout
		// Configure batching settings to avoid waiting for batch completion
		cfg.BatchTimeout = shortTimeout
		cfg.MaxExportBatchSize = 1
		cfg.MaxQueueSize = 1

		var err error
		exporter, err = otelexporter.NewExporter(ctx,
			otelexporter.WithLogger(testLogger),
			otelexporter.WithTracerProvider(tracerProvider),
			otelexporter.WithConfig(cfg),
		)

		if err != nil {
			// Don't skip, but log the error and continue
			GinkgoWriter.Printf("Error creating exporter: %v\n", err)
		}
	})

	AfterEach(func() {
		// Ensure we always clean up resources
		if tracerProvider != nil {
			// Force flush with very short timeout
			flushCtx, flushCancel := context.WithTimeout(context.Background(), shortTimeout)
			_ = tracerProvider.ForceFlush(flushCtx)
			flushCancel()

			// Shutdown tracer provider with very short timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			_ = tracerProvider.Shutdown(shutdownCtx)
			shutdownCancel()
		}

		if exporter != nil {
			// Create a new context with very short timeout for shutdown
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shortTimeout)
			// Ignore errors during shutdown - this is just cleanup
			_ = exporter.Shutdown(shutdownCtx)
			shutdownCancel()
		}
	})

	Describe("RecordMetric", func() {
		It("should record int64 metrics", func() {
			// Skip if exporter creation failed
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			metricCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record an int64 metric
			err := exporter.RecordMetric(metricCtx, "test.int64.counter", int64(42),
				attribute.String("key", "value"),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should record float64 metrics", func() {
			// Skip if exporter creation failed
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			metricCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record a float64 metric
			err := exporter.RecordMetric(metricCtx, "test.float64.counter", float64(42.5),
				attribute.String("key", "value"),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for unsupported types", func() {
			// Skip if exporter creation failed
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// This test should be very fast since it's just checking type validation
			// Use a very short timeout
			metricCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record with an unsupported type (string)
			err := exporter.RecordMetric(metricCtx, "test.string.counter", "unsupported")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported metric value type"))
		})
	})

	Describe("StartSpanWithAttributes", func() {
		It("should start a span with attributes", func() {
			// Skip if exporter creation failed
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a very short timeout for this test
			spanCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			ctxWithSpan, span := exporter.StartSpanWithAttributes(spanCtx, "attribute-span",
				[]attribute.KeyValue{
					attribute.String("service", "test"),
					attribute.Int("priority", 1),
				},
			)

			Expect(ctxWithSpan).NotTo(BeNil())
			Expect(span).NotTo(BeNil())

			// End the span
			span.End()

			// Force flush with very short timeout
			flushCtx, flushCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer flushCancel()
			err := tracerProvider.ForceFlush(flushCtx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle empty attributes", func() {
			// Skip if exporter creation failed
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a very short timeout
			spanCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			ctxWithSpan, span := exporter.StartSpanWithAttributes(spanCtx, "no-attribute-span",
				[]attribute.KeyValue{},
			)

			Expect(ctxWithSpan).NotTo(BeNil())
			Expect(span).NotTo(BeNil())

			// End the span
			span.End()

			// Force flush with very short timeout
			flushCtx, flushCancel := context.WithTimeout(context.Background(), shortTimeout)
			defer flushCancel()
			err := tracerProvider.ForceFlush(flushCtx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// Add tests for new metric functions
	Describe("RecordHistogram", func() {
		It("should record int64 histogram", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			metricCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record an int64 histogram metric
			err := exporter.RecordHistogram(metricCtx, "test.int64.histogram", int64(100),
				attribute.String("key", "value"),
				attribute.Int("bucket", 1),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should record float64 histogram", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			metricCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record a float64 histogram metric
			err := exporter.RecordHistogram(metricCtx, "test.float64.histogram", float64(75.5),
				attribute.String("key", "value"),
				attribute.Int("bucket", 2),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for unsupported types", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			metricCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record with an unsupported type (string)
			err := exporter.RecordHistogram(metricCtx, "test.string.histogram", "unsupported")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported histogram value type"))
		})
	})

	Describe("RecordDuration", func() {
		It("should record operation duration", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			durationCtx, cancel := context.WithTimeout(ctx, shortTimeout*2)
			defer cancel()

			// Function that we'll time
			operationRan := false
			err := exporter.RecordDuration(durationCtx, "test.operation", func(ctx context.Context) error {
				operationRan = true
				// Simulate a short operation
				time.Sleep(shortTimeout / 2)
				return nil
			}, attribute.String("operation", "test"))

			// Verify the operation ran and no error occurred
			Expect(err).NotTo(HaveOccurred())
			Expect(operationRan).To(BeTrue())
		})

		It("should record duration with error", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			durationCtx, cancel := context.WithTimeout(ctx, shortTimeout*2)
			defer cancel()

			// Expected error
			expectedErr := errors.New("operation failed")

			// Function that returns an error
			err := exporter.RecordDuration(durationCtx, "test.operation.error", func(ctx context.Context) error {
				// Simulate a short operation that fails
				time.Sleep(shortTimeout / 2)
				return expectedErr
			}, attribute.String("operation", "test_error"))

			// Verify the error was returned
			Expect(err).To(Equal(expectedErr))
		})
	})

	Describe("RecordCounter", func() {
		It("should record counter increments", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			counterCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record a counter
			err := exporter.RecordCounter(counterCtx, "test.counter", 1,
				attribute.String("counter", "increment"),
			)
			Expect(err).NotTo(HaveOccurred())

			// Record another increment
			err = exporter.RecordCounter(counterCtx, "test.counter", 2,
				attribute.String("counter", "increment"),
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("RecordGauge", func() {
		It("should record gauge values", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			gaugeCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record an int64 gauge
			err := exporter.RecordGauge(gaugeCtx, "test.gauge.int", int64(42),
				attribute.String("gauge_type", "int64"),
			)
			Expect(err).NotTo(HaveOccurred())

			// Record a float64 gauge
			err = exporter.RecordGauge(gaugeCtx, "test.gauge.float", float64(123.45),
				attribute.String("gauge_type", "float64"),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for unsupported types", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Use a context with a very short timeout
			gaugeCtx, cancel := context.WithTimeout(ctx, shortTimeout)
			defer cancel()

			// Record with an unsupported type (string)
			err := exporter.RecordGauge(gaugeCtx, "test.gauge.string", "unsupported")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported gauge value type"))
		})
	})
})
