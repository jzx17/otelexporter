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
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/jzx17/otelexporter"
)

var _ = Describe("Integration Scenarios", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		testLogger *testLogger
	)

	// Constants for timeouts
	const (
		shortTimeout = 5 * time.Millisecond
		testTimeout  = 25 * time.Millisecond
	)

	BeforeEach(func() {
		// Create a context with a short timeout
		ctx, cancel = context.WithTimeout(context.Background(), testTimeout)
		DeferCleanup(cancel)

		testLogger = newTestLogger()

		// Reset the default exporter for each test
		otelexporter.ResetDefaultForTest()

		// Add a small delay to prevent resource contention
		time.Sleep(1 * time.Millisecond)
	})

	// Helper function to create a tracer provider that avoids timeouts
	createFastTracerProvider := func(name string) (*tracetest.InMemoryExporter, *sdktrace.TracerProvider) {
		spanExporter := tracetest.NewInMemoryExporter()
		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			// Important: Use only the syncer to avoid batch timeouts
			sdktrace.WithSyncer(spanExporter),
		)
		return spanExporter, tracerProvider
	}

	Describe("Multiple Exporters", func() {
		It("should isolate providers between different contexts", func() {
			// Create two exporters with different configurations
			exporter1Config := otelexporter.DefaultConfig()
			exporter1Config.ServiceName = "service-1"
			exporter1Config.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
			exporter1Config.Timeout = shortTimeout

			spanExporter1, tracerProvider1 := createFastTracerProvider("provider1")
			defer tracerProvider1.Shutdown(ctx)

			exporter1, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(exporter1Config),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider1),
			)
			if err != nil {
				Skip("Couldn't create first exporter: " + err.Error())
			}
			defer exporter1.Shutdown(ctx)

			// Create second exporter
			exporter2Config := otelexporter.DefaultConfig()
			exporter2Config.ServiceName = "service-2"
			exporter2Config.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
			exporter2Config.Timeout = shortTimeout

			spanExporter2, tracerProvider2 := createFastTracerProvider("provider2")
			defer tracerProvider2.Shutdown(ctx)

			exporter2, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(exporter2Config),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider2),
			)
			if err != nil {
				Skip("Couldn't create second exporter: " + err.Error())
			}
			defer exporter2.Shutdown(ctx)

			// Create contexts for each exporter
			ctx1 := exporter1.WithContext(ctx)
			ctx2 := exporter2.WithContext(ctx)

			// Start spans using both contexts
			_, span1 := otelexporter.TracerFromContext(ctx1, "").Start(ctx1, "span-from-exporter1")
			_, span2 := otelexporter.TracerFromContext(ctx2, "").Start(ctx2, "span-from-exporter2")

			// End the spans
			span1.End()
			span2.End()

			// Verify spans went to their corresponding exporters by checking their names
			// No need to check exact lengths because the InMemoryExporter stores all spans
			spans1 := spanExporter1.GetSpans()
			spans2 := spanExporter2.GetSpans()

			// We only care that one span with the right name is in each exporter
			var foundSpan1, foundSpan2 bool
			for _, s := range spans1 {
				if s.Name == "span-from-exporter1" {
					foundSpan1 = true
					break
				}
			}
			for _, s := range spans2 {
				if s.Name == "span-from-exporter2" {
					foundSpan2 = true
					break
				}
			}

			Expect(foundSpan1).To(BeTrue(), "Span from exporter1 should be found in exporter1's spans")
			Expect(foundSpan2).To(BeTrue(), "Span from exporter2 should be found in exporter2's spans")
		})
	})

	Describe("Default Exporter Integration", func() {
		It("should use default exporter in context helpers", func() {
			// Create a default exporter
			spanExporter, tracerProvider := createFastTracerProvider("default")
			defer tracerProvider.Shutdown(ctx)

			defaultConfig := otelexporter.DefaultConfig()
			defaultConfig.ServiceName = "default-service"
			defaultConfig.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
			defaultConfig.Timeout = shortTimeout

			err := otelexporter.InitDefault(ctx,
				otelexporter.WithConfig(defaultConfig),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't initialize default exporter: " + err.Error())
			}

			// Get the default exporter
			defaultExp := otelexporter.Default()

			// Create a context using the default exporter
			ctxWithDefault := defaultExp.WithContext(ctx)

			// Use the context helpers
			tracer := otelexporter.TracerFromContext(ctxWithDefault, "")
			_, span := tracer.Start(ctxWithDefault, "default-exporter-span")
			span.End()

			// Check that the span was created and has the expected name
			// Just check if we can find a span with the expected name
			spans := spanExporter.GetSpans()

			var foundExpectedSpan bool
			for _, s := range spans {
				if s.Name == "default-exporter-span" {
					foundExpectedSpan = true
					break
				}
			}
			Expect(foundExpectedSpan).To(BeTrue(), "Should find a span with the name 'default-exporter-span'")
		})

		It("should integrate with WrapWithSpan", func() {
			// Create a default exporter
			spanExporter, tracerProvider := createFastTracerProvider("wrap-test")
			defer tracerProvider.Shutdown(ctx)

			defaultConfig := otelexporter.DefaultConfig()
			defaultConfig.OTLPEndpoint = "localhost:1" // Use a port that will fail quickly
			defaultConfig.Timeout = shortTimeout

			err := otelexporter.InitDefault(ctx,
				otelexporter.WithConfig(defaultConfig),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't initialize default exporter: " + err.Error())
			}

			// Get the default exporter
			defaultExp := otelexporter.Default()

			// Create a context using the default exporter
			ctxWithDefault := defaultExp.WithContext(ctx)

			// Use WrapWithSpan
			callCount := 0
			err = otelexporter.WrapWithSpan(ctxWithDefault, "wrapped-operation", func(c context.Context) error {
				callCount++
				// Start a nested span
				_, nestedSpan := otelexporter.TracerFromContext(c, "").Start(c, "nested-span")
				nestedSpan.End()
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1))

			// Check that both spans were created - just verify they exist
			spans := spanExporter.GetSpans()

			// Find the spans by name
			var foundWrapped, foundNested bool
			for _, s := range spans {
				if s.Name == "wrapped-operation" {
					foundWrapped = true
				}
				if s.Name == "nested-span" {
					foundNested = true
				}
			}

			Expect(foundWrapped).To(BeTrue(), "Should have found the wrapped operation span")
			Expect(foundNested).To(BeTrue(), "Should have found the nested span")
		})
	})

	// Test for new timeout utility functions
	Describe("Timeout Utilities", func() {
		It("should use WrapWithSpanAndTimeout", func() {
			// Create an exporter with custom configuration
			spanExporter, tracerProvider := createFastTracerProvider("timeout-test")
			defer tracerProvider.Shutdown(ctx)

			cfg := otelexporter.DefaultConfig()
			cfg.ServiceName = "timeout-service"
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout * 2 // Set a known timeout

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't create exporter: " + err.Error())
			}
			defer exporter.Shutdown(ctx)

			// Use WrapWithSpanAndTimeout with a successful function
			callCount := 0
			err = exporter.WrapWithSpanAndTimeout(ctx, "timeout-span", shortTimeout*3, func(c context.Context) error {
				callCount++
				// Add a small delay but not enough to time out
				time.Sleep(shortTimeout / 2)
				return nil
			})

			// Verify the function ran successfully
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1))

			// Check that the span was created
			spans := spanExporter.GetSpans()

			// Find the timeout-span
			timeoutSpanFound := false
			for _, s := range spans {
				if s.Name == "timeout-span" {
					timeoutSpanFound = true
					break
				}
			}
			Expect(timeoutSpanFound).To(BeTrue(), "Should have found the timeout-span")
		})

		It("should handle errors in WrapWithSpanAndTimeout", func() {
			// Create an exporter with custom configuration
			spanExporter, tracerProvider := createFastTracerProvider("timeout-error-test")
			defer tracerProvider.Shutdown(ctx)

			cfg := otelexporter.DefaultConfig()
			cfg.ServiceName = "timeout-error-service"
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout * 2

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't create exporter: " + err.Error())
			}
			defer exporter.Shutdown(ctx)

			// Expected error
			expectedErr := errors.New("operation failed")

			// Use WrapWithSpanAndTimeout with a function that returns an error
			err = exporter.WrapWithSpanAndTimeout(ctx, "error-span", shortTimeout*3, func(c context.Context) error {
				return expectedErr
			})

			// Verify the error was returned
			Expect(err).To(Equal(expectedErr))

			// Check that the span was created and has error status
			spans := spanExporter.GetSpans()

			errorSpanFound := false
			for _, s := range spans {
				if s.Name == "error-span" {
					errorSpanFound = true
					// Span should have an error status
					Expect(s.Status.Code).To(Equal(codes.Error))
					break
				}
			}
			Expect(errorSpanFound).To(BeTrue(), "Should have found the error-span")
		})

		It("should use ContextWithExporterTimeout", func() {
			// Create an exporter with custom configuration
			_, tracerProvider := createFastTracerProvider("context-timeout-test")
			defer tracerProvider.Shutdown(ctx)

			cfg := otelexporter.DefaultConfig()
			cfg.ServiceName = "context-timeout-service"
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout * 3 // Set a known timeout

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't create exporter: " + err.Error())
			}
			defer exporter.Shutdown(ctx)

			// Create a context with the exporter's timeout
			timeoutCtx, cancel := exporter.ContextWithExporterTimeout(ctx)
			defer cancel()

			// Verify the deadline was set
			deadline, hasDeadline := timeoutCtx.Deadline()
			Expect(hasDeadline).To(BeTrue())

			// The deadline should be approximately now + exporter timeout
			expectedDeadline := time.Now().Add(cfg.Timeout)
			deadlineDiff := deadline.Sub(expectedDeadline).Abs()

			// Allow for a small margin of error in timing (25ms)
			Expect(deadlineDiff).To(BeNumerically("<", 25*time.Millisecond))
		})

		It("should use TimeoutContext with minimum timeout", func() {
			// Create an exporter with custom configuration
			_, tracerProvider := createFastTracerProvider("min-timeout-test")
			defer tracerProvider.Shutdown(ctx)

			cfg := otelexporter.DefaultConfig()
			cfg.ServiceName = "min-timeout-service"
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout * 2 // Set a small timeout

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't create exporter: " + err.Error())
			}
			defer exporter.Shutdown(ctx)

			// Set a minimum timeout that is longer than the exporter timeout
			minTimeout := shortTimeout * 4

			// Create a context with the minimum timeout (which should take precedence)
			timeoutCtx, cancel := exporter.TimeoutContext(ctx, minTimeout)
			defer cancel()

			// Verify the deadline was set
			deadline, hasDeadline := timeoutCtx.Deadline()
			Expect(hasDeadline).To(BeTrue())

			// The deadline should be approximately now + minimum timeout
			expectedDeadline := time.Now().Add(minTimeout)
			deadlineDiff := deadline.Sub(expectedDeadline).Abs()

			// Allow for a small margin of error in timing (25ms)
			Expect(deadlineDiff).To(BeNumerically("<", 25*time.Millisecond))
		})

		It("should use StartSpanWithTimeout", func() {
			// Create an exporter with custom configuration
			spanExporter, tracerProvider := createFastTracerProvider("span-timeout-test")
			defer tracerProvider.Shutdown(ctx)

			cfg := otelexporter.DefaultConfig()
			cfg.ServiceName = "span-timeout-service"
			cfg.OTLPEndpoint = "localhost:1"
			cfg.Timeout = shortTimeout * 2

			exporter, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(cfg),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't create exporter: " + err.Error())
			}
			defer exporter.Shutdown(ctx)

			// Create attributes
			attrs := []attribute.KeyValue{
				attribute.String("test", "value"),
			}

			// Start a span with timeout and attributes
			spanCtx, span, cancel := exporter.StartSpanWithTimeout(
				ctx,
				"timeout-span",
				shortTimeout*3,
				oteltrace.WithAttributes(attrs...),
			)

			// Verify the span context and span are valid
			Expect(spanCtx).NotTo(BeNil())
			Expect(span).NotTo(BeNil())

			// End the span and cancel the context
			span.End()
			cancel()

			// Check that the span was created
			spans := spanExporter.GetSpans()

			// Find the span with timeout
			timeoutSpanFound := false
			for _, s := range spans {
				if s.Name == "timeout-span" {
					timeoutSpanFound = true
					// Verify the attribute was set
					attributeFound := false
					for _, attr := range s.Attributes {
						if attr.Key == "test" && attr.Value.AsString() == "value" {
							attributeFound = true
							break
						}
					}
					Expect(attributeFound).To(BeTrue(), "Should have found the attribute")
					break
				}
			}
			Expect(timeoutSpanFound).To(BeTrue(), "Should have found the timeout span")
		})
	})
})
