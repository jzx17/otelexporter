package otelexporter

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"time"
)

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

// RecordHistogram records a histogram measurement with attributes.
func (e *Exporter) RecordHistogram(ctx context.Context, name string, value interface{}, attrs ...attribute.KeyValue) error {
	meter := e.Meter("")

	switch v := value.(type) {
	case int64:
		histogram, err := meter.Int64Histogram(name)
		if err != nil {
			return err
		}
		histogram.Record(ctx, v, otelmetric.WithAttributes(attrs...))
	case float64:
		histogram, err := meter.Float64Histogram(name)
		if err != nil {
			return err
		}
		histogram.Record(ctx, v, otelmetric.WithAttributes(attrs...))
	default:
		return fmt.Errorf("unsupported histogram value type: %T", value)
	}

	return nil
}

// RecordDuration records the duration of an operation as a metric.
// The operation is executed within the provided function.
func (e *Exporter) RecordDuration(ctx context.Context, name string, fn func(context.Context) error, attrs ...attribute.KeyValue) error {
	startTime := time.Now()

	// Execute the function
	err := fn(ctx)

	// Record the duration as a metric
	duration := time.Since(startTime).Milliseconds()

	// Add error attribute if an error occurred
	if err != nil {
		attrs = append(attrs, attribute.Bool("error", true))
	}

	// Record the duration metric
	if metricErr := e.RecordHistogram(ctx, name+".duration_ms", duration, attrs...); metricErr != nil {
		// If we have both an operation error and a metric error, prioritize the operation error
		if err != nil {
			return err
		}
		return metricErr
	}

	return err
}

// RecordCounter increments a counter metric with the given name and attributes.
// This is a convenience wrapper around RecordMetric for counter-style metrics.
func (e *Exporter) RecordCounter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) error {
	return e.RecordMetric(ctx, name, value, attrs...)
}

// RecordGauge records a gauge metric with the given name, value, and attributes.
func (e *Exporter) RecordGauge(ctx context.Context, name string, value interface{}, attrs ...attribute.KeyValue) error {
	meter := e.Meter("")

	switch v := value.(type) {
	case int64:
		gauge, err := meter.Int64ObservableGauge(name)
		if err != nil {
			return err
		}
		// Create a callback to report the current value
		_, err = meter.RegisterCallback(
			func(ctx context.Context, o otelmetric.Observer) error {
				o.ObserveInt64(gauge, v, otelmetric.WithAttributes(attrs...))
				return nil
			},
			gauge,
		)
		return err
	case float64:
		gauge, err := meter.Float64ObservableGauge(name)
		if err != nil {
			return err
		}
		// Create a callback to report the current value
		_, err = meter.RegisterCallback(
			func(ctx context.Context, o otelmetric.Observer) error {
				o.ObserveFloat64(gauge, v, otelmetric.WithAttributes(attrs...))
				return nil
			},
			gauge,
		)
		return err
	default:
		return fmt.Errorf("unsupported gauge value type: %T", value)
	}
}
