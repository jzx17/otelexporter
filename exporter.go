// exporter.go - Add propagator field to Exporter struct
package otelexporter

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Logger interface abstracts logging operations.
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Sync() error
}

// Exporter provides OpenTelemetry instrumentation capabilities.
type Exporter struct {
	config         Config
	logger         Logger
	tracerProvider *tracesdk.TracerProvider
	meterProvider  *metricsdk.MeterProvider
	traceExporter  *otlptrace.Exporter
	metricExporter *otlpmetricgrpc.Exporter
	propagator     propagation.TextMapPropagator // Added propagator field
}

// Option defines a function that configures the Exporter.
type Option func(*Exporter)

// WithLogger sets the logger for the Exporter.
func WithLogger(l Logger) Option {
	return func(e *Exporter) {
		e.logger = l
	}
}

// WithTracerProvider sets a custom TracerProvider.
func WithTracerProvider(tp *tracesdk.TracerProvider) Option {
	return func(e *Exporter) {
		e.tracerProvider = tp
	}
}

// WithMeterProvider sets a custom MeterProvider.
func WithMeterProvider(mp *metricsdk.MeterProvider) Option {
	return func(e *Exporter) {
		e.meterProvider = mp
	}
}

// WithConfig sets the configuration for the Exporter.
func WithConfig(cfg Config) Option {
	return func(e *Exporter) {
		e.config = cfg
	}
}

// WithPropagator sets a custom propagator for the Exporter.
func WithPropagator(p propagation.TextMapPropagator) Option {
	return func(e *Exporter) {
		e.propagator = p
	}
}

// NewExporter creates a new OpenTelemetry exporter with the given options.
func NewExporter(ctx context.Context, opts ...Option) (*Exporter, error) {
	e := &Exporter{
		config: DefaultConfig(),
		// Default to W3C trace context and baggage propagation
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	}

	// Apply options
	for _, opt := range opts {
		opt(e)
	}

	// Load environment variables (if any)
	e.config.FromEnv()

	// Initialize default logger if not provided
	if e.logger == nil {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default logger: %w", err)
		}
		e.logger = logger
	}

	// Initialize providers
	if err := e.initializeProviders(ctx); err != nil {
		return nil, err
	}

	// Set global providers for convenience
	otel.SetTracerProvider(e.tracerProvider)
	otel.SetMeterProvider(e.meterProvider)
	otel.SetTextMapPropagator(e.propagator) // Set global propagator

	return e, nil
}

// Propagator returns the configured context propagator.
func (e *Exporter) Propagator() propagation.TextMapPropagator {
	return e.propagator
}

// Tracer returns a new tracer with the given name.
func (e *Exporter) Tracer(name string) oteltrace.Tracer {
	if name == "" {
		name = e.config.ServiceName
	}
	return e.tracerProvider.Tracer(name)
}

// Meter returns a new meter with the given name.
func (e *Exporter) Meter(name string) otelmetric.Meter {
	if name == "" {
		name = e.config.ServiceName
	}
	return e.meterProvider.Meter(name)
}

// Logger returns the configured logger.
func (e *Exporter) Logger() Logger {
	return e.logger
}

// TracerProvider returns the configured tracer provider.
func (e *Exporter) TracerProvider() oteltrace.TracerProvider {
	return e.tracerProvider
}

// MeterProvider returns the configured meter provider.
func (e *Exporter) MeterProvider() otelmetric.MeterProvider {
	return e.meterProvider
}
