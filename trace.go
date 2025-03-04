package otelexporter

import (
	"context"
	"errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"time"
)

// StartSpan is a helper method to start a new span with the default tracer.
func (e *Exporter) StartSpan(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return e.Tracer("").Start(ctx, name, opts...)
}

// StartSpanWithAttributes is a helper method to start a new span with attributes.
func (e *Exporter) StartSpanWithAttributes(
	ctx context.Context,
	name string,
	attributes []attribute.KeyValue,
	opts ...oteltrace.SpanStartOption,
) (context.Context, oteltrace.Span) {
	// Create a new span with the given options
	ctx, span := e.StartSpan(ctx, name, opts...)

	// Set attributes on the span
	if len(attributes) > 0 {
		span.SetAttributes(attributes...)
	}

	return ctx, span
}

// RecordSpanError records an error in the current span.
func RecordSpanError(ctx context.Context, err error, msg string) {
	span := oteltrace.SpanFromContext(ctx)
	span.RecordError(err)

	// Set the span status to error with the message
	if msg != "" {
		span.SetStatus(codes.Error, msg)
	} else {
		span.SetStatus(codes.Error, err.Error())
	}
}

// AddSpanEvent adds an event to the current span.
func AddSpanEvent(ctx context.Context, name string, attributes ...attribute.KeyValue) {
	span := oteltrace.SpanFromContext(ctx)
	span.AddEvent(name, oteltrace.WithAttributes(attributes...))
}

// WrapWithSpan wraps a function execution with a span.
// This is useful for instrumenting functions or code blocks.
func WrapWithSpan(ctx context.Context, name string, fn func(context.Context) error) error {
	// Get the tracer from the context or fall back to the global tracer
	tracer := TracerProviderFromContext(ctx).Tracer("")

	// Start a new span
	ctx, span := tracer.Start(ctx, name)
	defer span.End()

	// Execute the function
	if err := fn(ctx); err != nil {
		// Record the error in the span
		RecordSpanError(ctx, err, "")
		return err
	}

	return nil
}

// GetTraceID extracts the trace ID from the context if a span is active.
// Returns an empty string if no span is found.
func GetTraceID(ctx context.Context) string {
	span := oteltrace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// GetSpanID extracts the span ID from the context if a span is active.
// Returns an empty string if no span is found.
func GetSpanID(ctx context.Context) string {
	span := oteltrace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().SpanID().String()
}

// IsSpanRecording returns true if the current span is recording.
func IsSpanRecording(ctx context.Context) bool {
	return oteltrace.SpanFromContext(ctx).IsRecording()
}

// WrapWithSpanAndTimeout wraps a function execution with a span and adds a timeout
// based on the exporter's configuration.
func (e *Exporter) WrapWithSpanAndTimeout(ctx context.Context, name string, timeout time.Duration, fn func(context.Context) error) error {
	// Create a timeout context
	timeoutCtx, cancel := e.ContextWithTimeout(ctx, timeout)
	defer cancel()

	// Get the tracer from the context or fall back to the global tracer
	tracer := e.Tracer("")

	// Start a new span
	timeoutCtx, span := tracer.Start(timeoutCtx, name)
	defer span.End()

	// Execute the function with the timeout context
	if err := fn(timeoutCtx); err != nil {
		// Check if the error is due to context deadline exceeded
		if errors.Is(err, context.DeadlineExceeded) {
			RecordSpanError(timeoutCtx, err, "Operation timed out")
		} else {
			RecordSpanError(timeoutCtx, err, "")
		}
		return err
	}

	return nil
}

// StartSpanWithTimeout starts a new span with a timeout context based on the exporter's configuration.
func (e *Exporter) StartSpanWithTimeout(ctx context.Context, name string, timeout time.Duration, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span, context.CancelFunc) {
	// Create a timeout context
	timeoutCtx, cancel := e.ContextWithTimeout(ctx, timeout)

	// Start a new span
	spanCtx, span := e.Tracer("").Start(timeoutCtx, name, opts...)

	return spanCtx, span, cancel
}
