package otelexporter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// tracerProviderKey is the context key for storing a TracerProvider in a context.
type tracerProviderKey struct{}

// meterProviderKey is the context key for storing a MeterProvider in a context.
type meterProviderKey struct{}

// exporterKey is the context key for storing an Exporter in a context.
type exporterKey struct{}

// Config holds the configuration for the OpenTelemetry exporter.
type Config struct {
	ServiceName        string
	ServiceVersion     string
	Environment        string
	OTLPEndpoint       string // e.g., "localhost:4317"
	TraceSamplingRatio float64
	Timeout            time.Duration
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		ServiceName:        "default-service",
		ServiceVersion:     "1.0.0",
		Environment:        "production",
		OTLPEndpoint:       "localhost:4317",
		TraceSamplingRatio: 1.0, // 100% sampling rate
		Timeout:            5 * time.Second,
	}
}

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

// NewExporter creates a new OpenTelemetry exporter with the given options.
func NewExporter(ctx context.Context, opts ...Option) (*Exporter, error) {
	e := &Exporter{
		config: DefaultConfig(),
	}

	for _, opt := range opts {
		opt(e)
	}

	// Initialize default logger if not provided
	if e.logger == nil {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default logger: %w", err)
		}
		e.logger = logger
	}

	if err := e.initializeProviders(ctx); err != nil {
		return nil, err
	}

	// Set global providers for convenience
	otel.SetTracerProvider(e.tracerProvider)
	otel.SetMeterProvider(e.meterProvider)

	return e, nil
}

// initializeProviders sets up the trace and meter providers with appropriate exporters.
func (e *Exporter) initializeProviders(ctx context.Context) error {
	// Create a resource describing this service
	res, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(e.config.ServiceName),
			semconv.ServiceVersionKey.String(e.config.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(e.config.Environment),
		),
	)
	if err != nil {
		e.logger.Error("failed to create resource", zap.Error(err))
		res = resource.Default()
	}

	// Initialize tracer provider if not provided
	if e.tracerProvider == nil {
		// Configure how to export the traces
		traceClientOpts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(e.config.OTLPEndpoint),
			otlptracegrpc.WithTimeout(e.config.Timeout),
		}

		traceExporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(traceClientOpts...))
		if err != nil {
			return fmt.Errorf("failed to create trace exporter: %w", err)
		}
		e.traceExporter = traceExporter

		// Create a sampler based on the configured sampling ratio
		var sampler tracesdk.Sampler
		if e.config.TraceSamplingRatio >= 1.0 {
			sampler = tracesdk.AlwaysSample()
		} else if e.config.TraceSamplingRatio <= 0 {
			sampler = tracesdk.NeverSample()
		} else {
			sampler = tracesdk.TraceIDRatioBased(e.config.TraceSamplingRatio)
		}

		// Create the trace provider
		e.tracerProvider = tracesdk.NewTracerProvider(
			tracesdk.WithSampler(sampler),
			tracesdk.WithResource(res),
			tracesdk.WithBatcher(traceExporter),
		)
	}

	// Initialize meter provider if not provided
	if e.meterProvider == nil {
		metricExporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(e.config.OTLPEndpoint),
			otlpmetricgrpc.WithTimeout(e.config.Timeout),
		)
		if err != nil {
			return fmt.Errorf("failed to create metric exporter: %w", err)
		}
		e.metricExporter = metricExporter

		// Create the meter provider
		e.meterProvider = metricsdk.NewMeterProvider(
			metricsdk.WithResource(res),
			metricsdk.WithReader(metricsdk.NewPeriodicReader(metricExporter)),
		)
	}

	return nil
}

var (
	defaultExporter *Exporter
	once            sync.Once
	initErr         error
)

// InitDefault initializes the default exporter with the given options.
// This should be called early in the application lifecycle.
func InitDefault(ctx context.Context, opts ...Option) error {
	once.Do(func() {
		var err error
		defaultExporter, err = NewExporter(ctx, opts...)
		if err != nil {
			initErr = err
		}
	})
	return initErr
}

// Default returns the default exporter. If it hasn't been initialized,
// it will panic. Use InitDefault to initialize the default exporter.
func Default() *Exporter {
	if defaultExporter == nil {
		panic("default exporter not initialized, call InitDefault first")
	}
	return defaultExporter
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

// RecordMetric is a helper method to record a measurement with attributes.
func (e *Exporter) RecordMetric(ctx context.Context, name string, value interface{}, attrs ...attribute.KeyValue) error {
	meter := e.Meter("")

	switch v := value.(type) {
	case int64:
		counter, err := meter.Int64Counter(name)
		if err != nil {
			return err
		}
		counter.Add(ctx, v, otelmetric.WithAttributes(attrs...))
	case float64:
		counter, err := meter.Float64Counter(name)
		if err != nil {
			return err
		}
		counter.Add(ctx, v, otelmetric.WithAttributes(attrs...))
	default:
		return fmt.Errorf("unsupported metric value type: %T", value)
	}

	return nil
}

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

// Shutdown gracefully shuts down the exporter.
func (e *Exporter) Shutdown(ctx context.Context) error {
	var errs []error

	if e.tracerProvider != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
		defer cancel()
		if err := e.tracerProvider.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("tracer provider shutdown failed: %w", err))
		}
	}

	if e.meterProvider != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
		defer cancel()
		if err := e.meterProvider.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider shutdown failed: %w", err))
		}
	}

	if e.logger != nil {
		if err := e.logger.Sync(); err != nil {
			// Many logger implementations return errors on Sync() even when successful
			// Use string comparison to be more reliable than errors.Is() with errors.Join
			if !strings.Contains(err.Error(), "inappropriate ioctl for device") {
				errs = append(errs, fmt.Errorf("logger sync failed: %w", err))
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
