package otelexporter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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
