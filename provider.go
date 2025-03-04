package otelexporter

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"strings"
)

// initializeProviders sets up the trace and meter providers with appropriate exporters.
func (e *Exporter) initializeProviders(ctx context.Context) error {
	// Create a resource describing this service
	res, err := e.createResource(ctx)
	if err != nil {
		e.logger.Error("failed to create resource", zap.Error(err))
		// Continue with default resource
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
		var sampler trace.Sampler
		if e.config.TraceSamplingRatio >= 1.0 {
			sampler = trace.AlwaysSample()
		} else if e.config.TraceSamplingRatio <= 0 {
			sampler = trace.NeverSample()
		} else {
			sampler = trace.TraceIDRatioBased(e.config.TraceSamplingRatio)
		}

		// Create the trace provider with batch processing options
		e.tracerProvider = trace.NewTracerProvider(
			trace.WithSampler(sampler),
			trace.WithResource(res),
			trace.WithBatcher(
				traceExporter,
				trace.WithMaxExportBatchSize(e.config.MaxExportBatchSize),
				trace.WithBatchTimeout(e.config.BatchTimeout),
				trace.WithMaxQueueSize(e.config.MaxQueueSize),
			),
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

		// Create the meter provider with periodic reader for batch export
		e.meterProvider = metricsdk.NewMeterProvider(
			metricsdk.WithResource(res),
			metricsdk.WithReader(
				metricsdk.NewPeriodicReader(
					metricExporter,
					metricsdk.WithInterval(e.config.BatchTimeout),
				),
			),
		)
	}

	return nil
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
