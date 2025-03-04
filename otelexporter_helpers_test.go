package otelexporter_test

import (
	"context"
	"time"

	"github.com/jzx17/otelexporter"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Constants for testing
const (
	ultraShortTimeout = 1 * time.Millisecond
	shortTimeout      = 5 * time.Millisecond
	testTimeout       = 25 * time.Millisecond
)

// NoopMetricExporter is a metric exporter that does nothing
type NoopMetricExporter struct{}

func (e *NoopMetricExporter) Temporality(k metricsdk.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (e *NoopMetricExporter) Export(ctx context.Context, md *metricdata.ResourceMetrics) error {
	return nil // Always succeed without network calls
}

func (e *NoopMetricExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (e *NoopMetricExporter) Shutdown(ctx context.Context) error {
	return nil
}

// CreateTestExporter creates an exporter configured for testing with no network dependencies
func CreateTestExporter(ctx context.Context) (*otelexporter.Exporter, *tracetest.InMemoryExporter, error) {
	// Create a memory-only trace exporter with no network dependencies
	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(spanExporter), // Use syncer mode to avoid batch operations
	)

	// Create a meter provider with manual reader to avoid periodic network calls
	// Just create a manual reader - we don't need to attach the exporter explicitly
	// The ManualReader doesn't use network connections by default
	reader := metricsdk.NewManualReader()
	meterProvider := metricsdk.NewMeterProvider(
		metricsdk.WithReader(reader),
	)

	// Create test configuration that won't attempt real connections
	cfg := otelexporter.DefaultConfig()
	cfg.OTLPEndpoint = "localhost:1" // Invalid endpoint
	cfg.Timeout = ultraShortTimeout
	cfg.BatchTimeout = ultraShortTimeout
	cfg.MaxExportBatchSize = 1
	cfg.MaxQueueSize = 1

	// Create the test exporter
	exporter, err := otelexporter.NewExporter(ctx,
		otelexporter.WithTracerProvider(tracerProvider),
		otelexporter.WithMeterProvider(meterProvider),
		otelexporter.WithConfig(cfg),
	)

	return exporter, spanExporter, err
}
