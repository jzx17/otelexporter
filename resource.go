package otelexporter

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// createResource creates a resource describing the service with the given configuration.
func (e *Exporter) createResource(ctx context.Context) (*resource.Resource, error) {
	// Start with standard service attributes
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(e.config.ServiceName),
		semconv.ServiceVersionKey.String(e.config.ServiceVersion),
		semconv.DeploymentEnvironmentKey.String(e.config.Environment),
	}

	// Add any additional resource attributes from the configuration
	for key, value := range e.config.ResourceAttributes {
		attrs = append(attrs, attribute.String(key, value))
	}

	// Create the resource
	res, err := resource.New(ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(attrs...),
	)

	if err != nil {
		e.logger.Error("failed to create resource", zap.Error(err))
		return resource.Default(), err
	}

	return res, nil
}

// AddResourceAttribute adds a resource attribute to the exporter's configuration.
// This will affect new spans created after this call, but not existing spans.
func (e *Exporter) AddResourceAttribute(key, value string) {
	if e.config.ResourceAttributes == nil {
		e.config.ResourceAttributes = make(map[string]string)
	}
	e.config.ResourceAttributes[key] = value
}

// AddResourceAttributes adds multiple resource attributes to the exporter's configuration.
// This will affect new spans created after this call, but not existing spans.
func (e *Exporter) AddResourceAttributes(attrs map[string]string) {
	if e.config.ResourceAttributes == nil {
		e.config.ResourceAttributes = make(map[string]string)
	}
	for k, v := range attrs {
		e.config.ResourceAttributes[k] = v
	}
}

// GetResourceAttribute retrieves a resource attribute from the exporter's configuration.
// Returns the value and a boolean indicating if the attribute was found.
func (e *Exporter) GetResourceAttribute(key string) (string, bool) {
	if e.config.ResourceAttributes == nil {
		return "", false
	}
	value, ok := e.config.ResourceAttributes[key]
	return value, ok
}
