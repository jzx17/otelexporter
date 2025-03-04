package otelexporter

import (
	"context"
	"go.opentelemetry.io/otel"
	otelmetric "go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
	"time"
)

// tracerProviderKey is the context key for storing a TracerProvider in a context.
type tracerProviderKey struct{}

// meterProviderKey is the context key for storing a MeterProvider in a context.
type meterProviderKey struct{}

// exporterKey is the context key for storing an Exporter in a context.
type exporterKey struct{}

// WithContext returns a context that has this exporter's trace and meter providers set as the active ones.
// This implementation stores the providers directly in the context using context values.
func (e *Exporter) WithContext(ctx context.Context) context.Context {
	// Store the tracer provider in the context
	ctx = context.WithValue(ctx, tracerProviderKey{}, e.tracerProvider)

	// Store the meter provider in the context
	ctx = context.WithValue(ctx, meterProviderKey{}, e.meterProvider)

	// Store the exporter itself in the context for convenience
	ctx = context.WithValue(ctx, exporterKey{}, e)

	return ctx
}

// TracerProviderFromContext retrieves the TracerProvider from the context if available.
// If not available, it returns the global TracerProvider.
func TracerProviderFromContext(ctx context.Context) oteltrace.TracerProvider {
	if tp, ok := ctx.Value(tracerProviderKey{}).(oteltrace.TracerProvider); ok {
		return tp
	}
	return otel.GetTracerProvider()
}

// MeterProviderFromContext retrieves the MeterProvider from the context if available.
// If not available, it returns the global MeterProvider.
func MeterProviderFromContext(ctx context.Context) otelmetric.MeterProvider {
	if mp, ok := ctx.Value(meterProviderKey{}).(otelmetric.MeterProvider); ok {
		return mp
	}
	return otel.GetMeterProvider()
}

// TracerFromContext gets a Tracer from the TracerProvider in the context.
// This is a helper function for creating spans without having to manually extract
// the TracerProvider from the context.
func TracerFromContext(ctx context.Context, name string) oteltrace.Tracer {
	tp := TracerProviderFromContext(ctx)
	if name == "" {
		// Try to get the default exporter's service name
		if e, ok := ExporterFromContext(ctx); ok {
			name = e.config.ServiceName
		}
	}
	return tp.Tracer(name)
}

// MeterFromContext gets a Meter from the MeterProvider in the context.
// This is a helper function for creating metrics without having to manually extract
// the MeterProvider from the context.
func MeterFromContext(ctx context.Context, name string) otelmetric.Meter {
	mp := MeterProviderFromContext(ctx)
	if name == "" {
		// Try to get the default exporter's service name
		if e, ok := ExporterFromContext(ctx); ok {
			name = e.config.ServiceName
		}
	}
	return mp.Meter(name)
}

// ExporterFromContext retrieves the Exporter from the context if it was set with WithContext.
func ExporterFromContext(ctx context.Context) (*Exporter, bool) {
	e, ok := ctx.Value(exporterKey{}).(*Exporter)
	return e, ok
}

// ContextWithTimeout returns a new context with a timeout that respects the
// exporter's configuration. If the provided timeout is longer than the exporter's
// timeout, the exporter's timeout will be used instead.
func (e *Exporter) ContextWithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	// Use the shorter of the provided timeout or the exporter's timeout
	effectiveTimeout := timeout
	if e.config.Timeout < timeout {
		effectiveTimeout = e.config.Timeout
	}
	return context.WithTimeout(ctx, effectiveTimeout)
}

// ContextWithExporterTimeout returns a new context with a timeout set to the
// exporter's configured timeout.
func (e *Exporter) ContextWithExporterTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, e.config.Timeout)
}

// TimeoutContext is a utility function that creates a context with a timeout based
// on the config timeout but allows for a provided minimum timeout.
// This is useful for operations that should have at least a certain amount of time to complete.
func (e *Exporter) TimeoutContext(ctx context.Context, minTimeout time.Duration) (context.Context, context.CancelFunc) {
	// Use the larger of the minimum timeout or the exporter's timeout
	timeout := e.config.Timeout
	if minTimeout > timeout {
		timeout = minTimeout
	}
	return context.WithTimeout(ctx, timeout)
}
