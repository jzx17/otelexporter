// http.go
package otelexporter

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// wrappedResponseWriter is a wrapper for http.ResponseWriter that captures the status code
type wrappedResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// newWrappedResponseWriter creates a new wrapped response writer
func newWrappedResponseWriter(w http.ResponseWriter) *wrappedResponseWriter {
	return &wrappedResponseWriter{w, http.StatusOK}
}

// WriteHeader captures the status code and calls the underlying WriteHeader
func (w *wrappedResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware creates middleware to instrument HTTP servers.
func (e *Exporter) HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	if serviceName == "" {
		serviceName = e.config.ServiceName
	}

	tracer := e.Tracer(serviceName)
	meter := e.Meter(serviceName)

	// Create HTTP request counter
	requestCounter, _ := meter.Int64Counter(
		"http.server.request.count",
		otelmetric.WithDescription("Number of HTTP requests received"),
	)

	// Create HTTP duration histogram
	requestDuration, _ := meter.Float64Histogram(
		"http.server.duration",
		otelmetric.WithDescription("Duration of HTTP requests in seconds"),
		otelmetric.WithUnit("s"),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract context from the request headers
			ctx := r.Context()
			propagator := e.Propagator()
			if propagator != nil {
				ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))
			} else {
				// Fallback to global propagator if not set in the exporter
				ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
			}

			// Start a span for this request
			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(
				ctx,
				spanName,
				oteltrace.WithSpanKind(oteltrace.SpanKindServer),
				oteltrace.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.url", r.URL.String()),
					attribute.String("http.target", r.URL.Path),
					attribute.String("http.scheme", r.URL.Scheme),
					attribute.String("http.host", r.Host),
					attribute.String("user_agent.original", r.UserAgent()),
				),
			)
			defer span.End()

			// Create a wrapped response writer to capture the status code
			wrw := newWrappedResponseWriter(w)

			// Record metrics with basic HTTP attributes
			requestCounter.Add(ctx, 1,
				otelmetric.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.target", r.URL.Path),
				),
			)

			startTime := time.Now()

			// Call the next handler with the updated context
			next.ServeHTTP(wrw, r.WithContext(ctx))

			// Add status code to span
			span.SetAttributes(attribute.Int("http.status_code", wrw.statusCode))

			// Record duration
			duration := time.Since(startTime).Seconds()
			requestDuration.Record(ctx, duration,
				otelmetric.WithAttributes(
					attribute.String("http.method", r.Method),
					attribute.String("http.target", r.URL.Path),
					attribute.Int("http.status_code", wrw.statusCode),
				),
			)

			// Mark span as error if status code is 5xx
			if wrw.statusCode >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrw.statusCode))
			}
		})
	}
}

// WrapHTTPClient wraps an http.Client to add instrumentation to outgoing requests.
func (e *Exporter) WrapHTTPClient(client *http.Client, serviceName string) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}

	if serviceName == "" {
		serviceName = e.config.ServiceName
	}

	// Create a copy of the client
	wrappedClient := *client

	// Override the Transport with our instrumented version
	baseTransport := client.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	tracer := e.Tracer(serviceName)

	wrappedClient.Transport = &instrumentedTransport{
		base:       baseTransport,
		tracer:     tracer,
		propagator: e.Propagator(),
	}

	return &wrappedClient
}

// instrumentedTransport is an http.RoundTripper that adds instrumentation.
type instrumentedTransport struct {
	base       http.RoundTripper
	tracer     oteltrace.Tracer
	propagator propagation.TextMapPropagator
}

// RoundTrip implements http.RoundTripper and adds tracing.
func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Start a span for this request
	spanName := fmt.Sprintf("%s %s", req.Method, req.URL.Path)
	ctx, span := t.tracer.Start(
		ctx,
		spanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(
			attribute.String("http.method", req.Method),
			attribute.String("http.url", req.URL.String()),
			attribute.String("http.target", req.URL.Path),
			attribute.String("http.host", req.Host),
		),
	)
	defer span.End()

	// Inject trace context into the request headers
	if t.propagator != nil {
		t.propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))
	} else {
		// Fallback to global propagator if not set in the transport
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	}

	// Make the request with our updated context
	resp, err := t.base.RoundTrip(req.WithContext(ctx))

	// Record the result
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, err
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	// Mark span as error if status code is 5xx
	if resp.StatusCode >= 500 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", resp.StatusCode))
	}

	return resp, nil
}
