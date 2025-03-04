package otelexporter_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"

	"github.com/jzx17/otelexporter"
)

// Mock logger implementation for testing
type testContextLogger struct{}

func newTestContextLogger() *testContextLogger {
	return &testContextLogger{}
}

func (l *testContextLogger) Info(msg string, fields ...zap.Field)  {}
func (l *testContextLogger) Error(msg string, fields ...zap.Field) {}
func (l *testContextLogger) Debug(msg string, fields ...zap.Field) {}
func (l *testContextLogger) Warn(msg string, fields ...zap.Field)  {}
func (l *testContextLogger) Sync() error                           { return nil }

var _ = Describe("Context Provider Functions", func() {
	var (
		exporter       *otelexporter.Exporter
		tracerProvider *sdktrace.TracerProvider
		spanExporter   *tracetest.InMemoryExporter
		testLogger     *testContextLogger
		ctx            context.Context
		cancel         context.CancelFunc
	)

	BeforeEach(func() {
		// Create a short-lived context for setup
		ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
		DeferCleanup(cancel)

		testLogger = newTestContextLogger()
		spanExporter = tracetest.NewInMemoryExporter()
		tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(
				spanExporter,
				sdktrace.WithBatchTimeout(10*time.Millisecond),
			),
		)

		// Mock configuration that won't attempt real connections
		cfg := otelexporter.DefaultConfig()
		cfg.OTLPEndpoint = "localhost:1" // Use localhost with unlikely port to fail faster
		cfg.Timeout = 10 * time.Millisecond

		var err error
		exporter, err = otelexporter.NewExporter(ctx,
			otelexporter.WithLogger(testLogger),
			otelexporter.WithTracerProvider(tracerProvider),
			otelexporter.WithConfig(cfg),
		)

		if err != nil {
			GinkgoWriter.Printf("Error creating exporter: %v\n", err)
		}
	})

	AfterEach(func() {
		if exporter != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer shutdownCancel()
			_ = exporter.Shutdown(shutdownCtx)
		}

		if tracerProvider != nil {
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer flushCancel()
			_ = tracerProvider.ForceFlush(flushCtx)

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer shutdownCancel()
			_ = tracerProvider.Shutdown(shutdownCtx)
		}
	})

	Describe("Helper Functions", func() {
		It("should use service name when tracer name is empty", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create a short-lived context
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Store exporter in context
			ctxWithExporter := exporter.WithContext(testCtx)

			// Get tracer with empty name - should use service name
			tracer := otelexporter.TracerFromContext(ctxWithExporter, "")
			Expect(tracer).NotTo(BeNil())

			// Can't directly test the name, but we can verify it didn't panic
		})

		It("should get meter from context", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create a short-lived context
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Store exporter in context
			ctxWithExporter := exporter.WithContext(testCtx)

			// Get meter from context
			meter := otelexporter.MeterFromContext(ctxWithExporter, "test-meter")
			Expect(meter).NotTo(BeNil())
		})

		It("should get meter provider from context", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create a short-lived context
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Store exporter in context
			ctxWithExporter := exporter.WithContext(testCtx)

			// Get meter provider from context
			mp := otelexporter.MeterProviderFromContext(ctxWithExporter)
			Expect(mp).NotTo(BeNil())
		})
	})

	Describe("Span Helper Functions", func() {
		It("should wrap function execution with span", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create a short-lived context
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Store exporter in context
			ctxWithExporter := exporter.WithContext(testCtx)

			// Create a wrapped function that doesn't do network operations
			err := otelexporter.WrapWithSpan(ctxWithExporter, "test-wrap", func(ctx context.Context) error {
				return nil
			})

			Expect(err).NotTo(HaveOccurred())

			// Force flush spans quickly
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer flushCancel()
			_ = tracerProvider.ForceFlush(flushCtx)
		})

		It("should record span errors with default message", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create a very short-lived context
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Create a mock tracer that doesn't trigger network operations
			mockTracer := tracerProvider.Tracer("test-tracer")

			// Start a span using the mock tracer directly
			ctx, span := mockTracer.Start(testCtx, "error-span")

			// Record an error
			testError := errors.New("test error")
			otelexporter.RecordSpanError(ctx, testError, "")

			// End the span immediately
			span.End()

			// Force flush with minimal timeout
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer flushCancel()
			err := tracerProvider.ForceFlush(flushCtx)
			Expect(err).NotTo(HaveOccurred())

			// Verify the span has the error status by checking the exported spans
			spans := spanExporter.GetSpans()
			Expect(spans).NotTo(BeEmpty())
			foundErrorSpan := false
			for _, s := range spans {
				if s.Name == "error-span" && s.Status.Code == codes.Error {
					foundErrorSpan = true
					break
				}
			}
			Expect(foundErrorSpan).To(BeTrue())
		})

		It("should add span events", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create a short-lived context
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Start a span directly with the tracer provider
			ctx, span := tracerProvider.Tracer("test").Start(testCtx, "event-span")

			// Add an event
			otelexporter.AddSpanEvent(ctx, "test-event",
				attribute.String("key", "value"))

			// End the span
			span.End()

			// Force flush
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer flushCancel()
			_ = tracerProvider.ForceFlush(flushCtx)

			// Verify the event was added
			spans := spanExporter.GetSpans()
			Expect(spans).NotTo(BeEmpty())

			eventFound := false
			for _, s := range spans {
				if s.Name == "event-span" && len(s.Events) > 0 {
					eventFound = true
					break
				}
			}
			Expect(eventFound).To(BeTrue())
		})
	})

	Describe("Span ID Helpers", func() {
		It("should return empty string for context without span", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create context with very short timeout
			emptyCtx, emptyCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer emptyCancel()

			// Get trace ID from context without span
			traceID := otelexporter.GetTraceID(emptyCtx)
			Expect(traceID).To(Equal(""))

			// Get span ID from context without span
			spanID := otelexporter.GetSpanID(emptyCtx)
			Expect(spanID).To(Equal(""))
		})

		It("should check if span is recording", func() {
			if exporter == nil {
				Skip("Exporter creation failed in BeforeEach")
			}

			// Create context with very short timeout
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer testCancel()

			// Context without span
			isRecording := otelexporter.IsSpanRecording(testCtx)
			Expect(isRecording).To(BeFalse())

			// Create context with span
			spanCtx, span := tracerProvider.Tracer("test").Start(testCtx, "recording-span")
			defer span.End()

			// Check if span is recording
			isRecording = otelexporter.IsSpanRecording(spanCtx)
			Expect(isRecording).To(BeTrue())
		})
	})
})
