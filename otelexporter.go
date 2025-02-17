package otelexporter

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	otelmetric "go.opentelemetry.io/otel/metric"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
	Sync() error
}

type Exporter struct {
	logger         Logger
	tracerProvider *tracesdk.TracerProvider
	meterProvider  *metricsdk.MeterProvider
}

type Option func(*Exporter)

// WithLogger sets the logger for the Exporter.
func WithLogger(l Logger) Option {
	return func(e *Exporter) {
		e.logger = l
	}
}

func WithTracerProvider(tp *tracesdk.TracerProvider) Option {
	return func(e *Exporter) {
		e.tracerProvider = tp
	}
}

func WithMeterProvider(mp *metricsdk.MeterProvider) Option {
	return func(e *Exporter) {
		e.meterProvider = mp
	}
}

func NewExporter(opts ...Option) *Exporter {
	e := &Exporter{}
	for _, opt := range opts {
		opt(e)
	}

	// Initialize default logger if not provided
	if e.logger == nil {
		logger, err := zap.NewProduction()
		if err != nil {
			panic(fmt.Sprintf("failed to initialize default logger: %v", err))
		}
		e.logger = logger
	}

	// Initialize default tracer provider if not provided
	if e.tracerProvider == nil {
		res, err := resource.New(context.Background(),
			resource.WithSchemaURL(semconv.SchemaURL),
			resource.WithAttributes(
				semconv.ServiceNameKey.String("Default-Service"),
				semconv.ServiceVersionKey.String("1.0.0"),
				semconv.DeploymentEnvironmentKey.String("production"),
			),
		)
		if err != nil {
			e.logger.Error("failed to create resource", zap.Error(err))
			res = resource.Default()
		}

		e.tracerProvider = tracesdk.NewTracerProvider(
			tracesdk.WithSampler(tracesdk.AlwaysSample()),
			tracesdk.WithResource(res),
		)
	}

	// Initialize default meter provider if not provided
	if e.meterProvider == nil {
		e.meterProvider = metricsdk.NewMeterProvider()
	}

	return e
}

var (
	defaultExporter *Exporter
	once            sync.Once
)

func Default() *Exporter {
	once.Do(func() {
		defaultExporter = NewExporter()
	})
	return defaultExporter
}

func (e *Exporter) Logger() Logger {
	return e.logger
}

func (e *Exporter) Tracer() oteltrace.Tracer {
	return e.tracerProvider.Tracer("default")
}

func (e *Exporter) Meter() otelmetric.Meter {
	return e.meterProvider.Meter("default")
}

func (e *Exporter) Shutdown(ctx context.Context) error {
	var errs []error

	if e.tracerProvider != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := e.tracerProvider.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("tracer provider shutdown failed: %w", err))
		}
	}

	if e.meterProvider != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := e.meterProvider.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider shutdown failed: %w", err))
		}
	}

	if e.logger != nil {
		if err := e.logger.Sync(); err != nil {
			errs = append(errs, fmt.Errorf("logger sync failed: %w", err))
		}
	}

	return errors.Join(errs...)
}
