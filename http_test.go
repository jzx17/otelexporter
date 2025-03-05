// http_test.go
package otelexporter_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/jzx17/otelexporter"
)

// Helper function to check for a specific attribute in a span
func assertHasAttribute(span tracetest.SpanStub, key string, expectedValue interface{}) bool {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			switch expected := expectedValue.(type) {
			case string:
				return expected == attr.Value.AsString()
			case int64:
				return expected == attr.Value.AsInt64()
			case float64:
				return expected == attr.Value.AsFloat64()
			case bool:
				return expected == attr.Value.AsBool()
			default:
				return false
			}
		}
	}
	return false
}

var _ = Describe("HTTP Instrumentation", func() {
	// Constants for test timeouts - using pre-defined constants from otelexporter_helpers_test.go
	// const shortTimeout = 5 * time.Millisecond
	// const testTimeout = 25 * time.Millisecond

	Describe("HTTP Middleware", func() {
		It("should trace incoming HTTP requests", func() {
			// Setup in-memory exporter to capture spans
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			testLogger := newTestLogger()

			// Create exporter with memory trace provider
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create test HTTP handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify that context has span
				span := trace.SpanFromContext(r.Context())
				Expect(span).NotTo(Equal(trace.SpanFromContext(context.Background())))

				// Write response
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			})

			// Wrap the handler with our middleware
			wrappedHandler := exporter.HTTPMiddleware("test-service")(testHandler)

			// Create a test request
			req := httptest.NewRequest("GET", "/test-path", nil)
			req.Header.Set("User-Agent", "test-agent")
			rec := httptest.NewRecorder()

			// Execute the request
			wrappedHandler.ServeHTTP(rec, req)

			// Verify the response
			Expect(rec.Code).To(Equal(http.StatusOK))

			// Shutdown the provider to ensure all spans are exported
			err = tracerProvider.ForceFlush(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Check that spans were created
			spans := spanExporter.GetSpans()
			Expect(spans).To(HaveLen(1))

			// Verify span has expected attributes
			span := spans[0]
			Expect(span.Name).To(Equal("GET /test-path"))
			Expect(assertHasAttribute(span, "http.method", "GET")).To(BeTrue())
			Expect(assertHasAttribute(span, "http.target", "/test-path")).To(BeTrue())
			Expect(assertHasAttribute(span, "http.status_code", int64(200))).To(BeTrue())
			Expect(assertHasAttribute(span, "user_agent.original", "test-agent")).To(BeTrue())
		})

		It("should mark 5xx responses as errors", func() {
			// Setup in-memory exporter to capture spans
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			testLogger := newTestLogger()

			// Create exporter with memory trace provider
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create test HTTP handler that returns 500
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			})

			// Wrap the handler with our middleware
			wrappedHandler := exporter.HTTPMiddleware("test-service")(testHandler)

			// Create a test request
			req := httptest.NewRequest("GET", "/error-path", nil)
			rec := httptest.NewRecorder()

			// Execute the request
			wrappedHandler.ServeHTTP(rec, req)

			// Verify the response
			Expect(rec.Code).To(Equal(http.StatusInternalServerError))

			// Shutdown the provider to ensure all spans are exported
			err = tracerProvider.ForceFlush(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Check that spans were created
			spans := spanExporter.GetSpans()
			Expect(spans).To(HaveLen(1))

			// Verify span has error status and expected attributes
			span := spans[0]
			Expect(span.Name).To(Equal("GET /error-path"))
			Expect(assertHasAttribute(span, "http.status_code", int64(500))).To(BeTrue())
			Expect(span.Status.Code).To(Equal(codes.Error))
			Expect(span.Status.Description).To(ContainSubstring("HTTP 500"))
		})
	})

	Describe("HTTP Client Instrumentation", func() {
		It("should trace outgoing HTTP requests", func() {
			// Setup in-memory exporter to capture spans
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			testLogger := newTestLogger()

			// Create exporter with memory trace provider
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
				otelexporter.WithPropagator(propagation.TraceContext{}),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check if trace context was propagated
				traceHeader := r.Header.Get("traceparent")
				Expect(traceHeader).NotTo(BeEmpty(), "Trace context not propagated")

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer server.Close()

			// Create and wrap an HTTP client
			client := exporter.WrapHTTPClient(&http.Client{}, "test-client")

			// Make a request with the client
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/test-client-path", nil)
			Expect(err).NotTo(HaveOccurred())

			// Execute the request
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			// Check response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Shutdown the provider to ensure all spans are exported
			err = tracerProvider.ForceFlush(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Check that spans were created
			spans := spanExporter.GetSpans()
			Expect(spans).To(HaveLen(1))

			// Find the HTTP client span
			span := spans[0]
			Expect(span.Name).To(Equal("GET /test-client-path"))
			Expect(span.SpanKind).To(Equal(trace.SpanKindClient))
			Expect(assertHasAttribute(span, "http.method", "GET")).To(BeTrue())
			Expect(assertHasAttribute(span, "http.status_code", int64(200))).To(BeTrue())
		})

		It("should mark client errors", func() {
			// Setup in-memory exporter to capture spans
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			testLogger := newTestLogger()

			// Create exporter with memory trace provider
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a test server that returns 500
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			// Create and wrap an HTTP client
			client := exporter.WrapHTTPClient(&http.Client{}, "test-client")

			// Make a request with the client
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/error-path", nil)
			Expect(err).NotTo(HaveOccurred())

			// Execute the request
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			// Check response
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))

			// Shutdown the provider to ensure all spans are exported
			err = tracerProvider.ForceFlush(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Check that spans were created
			spans := spanExporter.GetSpans()
			Expect(spans).To(HaveLen(1))

			// Verify span has error status
			span := spans[0]
			Expect(span.Name).To(Equal("GET /error-path"))
			Expect(assertHasAttribute(span, "http.status_code", int64(500))).To(BeTrue())
			Expect(span.Status.Code).To(Equal(codes.Error))
		})

		It("should work with default client", func() {
			testLogger := newTestLogger()

			// Create exporter
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create client with nil (should use default)
			client := exporter.WrapHTTPClient(nil, "test-client")
			Expect(client).NotTo(BeNil())

			// Should be using http.DefaultClient but with instrumentation
			Expect(client).NotTo(Equal(http.DefaultClient))
		})
	})

	Describe("Context Propagation", func() {
		It("should use configured propagator", func() {
			// Create custom propagator
			customPropagator := propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			)

			testLogger := newTestLogger()

			// Create exporter with custom propagator
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithPropagator(customPropagator),
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify the propagator
			Expect(exporter.Propagator()).To(Equal(customPropagator))
		})

		It("should propagate trace context", func() {
			// Setup in-memory exporter to capture spans
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			testLogger := newTestLogger()

			// Create exporter with memory trace provider and W3C propagator
			var err error
			exporter, err := otelexporter.NewExporter(context.Background(),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
				otelexporter.WithPropagator(propagation.TraceContext{}),
			)
			Expect(err).NotTo(HaveOccurred())

			// Create a parent span
			parentCtx, parentSpan := tracerProvider.Tracer("test").Start(context.Background(), "parent-span")

			// Create a request with the parent span context injected
			req := httptest.NewRequest("GET", "/path", nil)
			exporter.Propagator().Inject(parentCtx, propagation.HeaderCarrier(req.Header))

			// Create a handler that extracts and creates a child span
			var childSpanTraceID trace.TraceID
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := exporter.Propagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
				_, childSpan := exporter.Tracer("test").Start(ctx, "child-span")
				childSpanTraceID = childSpan.SpanContext().TraceID()
				childSpan.End()
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with middleware
			wrappedHandler := exporter.HTTPMiddleware("test")(handler)

			// Make the request
			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			// End the parent span
			parentSpan.End()

			// Verify the child span has the same trace ID as the parent
			Expect(childSpanTraceID).To(Equal(parentSpan.SpanContext().TraceID()))
		})
	})
})
