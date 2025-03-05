# OTELExporter

A lightweight, flexible OpenTelemetry exporter for Go applications, providing simplified telemetry capabilities including tracing, metrics, and logging.

## Overview

OTELExporter is a Go package that simplifies the integration of OpenTelemetry instrumentation into your Go applications. It provides a unified interface for adding tracing, metrics, and structured logging with minimal configuration.

Key benefits:
- 🚀 Simple API with sensible defaults
- 🔄 Environment-based configuration
- 📊 Unified interface for traces and metrics
- 🔌 Easy integration with existing applications
- ⏱️ Timeout and context control utilities

## Installation

```bash
go get github.com/jzx17/otelexporter
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "time"
    
    "github.com/jzx17/otelexporter"
    "go.opentelemetry.io/otel/attribute"
)

func main() {
    // Initialize with environment variables
    ctx := context.Background()
    if err := otelexporter.InitDefaultWithEnvVars(ctx); err != nil {
        panic(err)
    }
    defer otelexporter.ShutdownDefault(ctx)
    
    // Get the default exporter
    exporter := otelexporter.Default()
    
    // Record a metric
    exporter.RecordCounter(ctx, "app.requests.count", 1, 
        attribute.String("endpoint", "/users"),
    )
    
    // Wrap a function execution with a traced span
    err := otelexporter.WrapWithSpan(ctx, "process-data", func(ctx context.Context) error {
        // Your business logic here
        time.Sleep(100 * time.Millisecond)
        return nil
    })
    if err != nil {
        // Handle error
    }
}
```

### Configuration

You can configure the exporter through environment variables:

```bash
# Basic configuration
export OTEL_SERVICE_NAME=my-service
export OTEL_SERVICE_VERSION=1.2.3
export OTEL_ENVIRONMENT=production
export OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317

# Sampling configuration
export OTEL_TRACE_SAMPLER_ARG=0.5  # 50% sampling rate

# Timeout and batch configurations
export OTEL_EXPORTER_OTLP_TIMEOUT=10s
export OTEL_BSP_SCHEDULE_DELAY=5s
export OTEL_BSP_MAX_EXPORT_BATCH_SIZE=512
export OTEL_BSP_MAX_QUEUE_SIZE=2048

# Custom resource attributes
export OTEL_RESOURCE_ATTRIBUTES=team=backend,region=us-west-2
```

## Detailed Features

### Context Management

The exporter provides context utilities to manage tracing and metrics providers:

```go
// Create a context with the exporter's trace and meter providers
ctx = exporter.WithContext(context.Background())

// Get providers from context
tracer := otelexporter.TracerFromContext(ctx, "component-name")
meter := otelexporter.MeterFromContext(ctx, "meter-name")

// Create a span
ctx, span := tracer.Start(ctx, "operation-name")
defer span.End()
```

### Timeout Utilities

The package offers several timeout-related helpers:

```go
// Create a context with the exporter's configured timeout
ctx, cancel := exporter.ContextWithExporterTimeout(ctx)
defer cancel()

// Create a context with a minimum timeout
ctx, cancel := exporter.TimeoutContext(ctx, 500*time.Millisecond)
defer cancel()

// Start a span with timeout
ctx, span, cancel := exporter.StartSpanWithTimeout(ctx, "timed-operation", 1*time.Second)
defer span.End()
defer cancel()

// Execute a function with span and timeout
err := exporter.WrapWithSpanAndTimeout(ctx, "process-data", 1*time.Second, func(ctx context.Context) error {
    // Operation that needs a timeout
    return nil
})
```

### Metrics Recording

The package provides several methods for recording metrics:

```go
// Record a counter
exporter.RecordCounter(ctx, "app.requests.total", 1,
    attribute.String("method", "GET"),
    attribute.String("endpoint", "/users"),
)

// Record a histogram 
exporter.RecordHistogram(ctx, "app.request.duration_ms", 235.7,
    attribute.String("endpoint", "/users"),
)

// Record a gauge
exporter.RecordGauge(ctx, "app.connections.active", 42,
    attribute.String("pool", "main"),
)

// Record duration of operation
exporter.RecordDuration(ctx, "app.process", func(ctx context.Context) error {
    // Operation to measure
    time.Sleep(50 * time.Millisecond)
    return nil
}, attribute.String("operation", "calculate"))
```

### Span Helpers

Helpful utilities for working with spans:

```go
// Add event to current span
otelexporter.AddSpanEvent(ctx, "cache-miss", 
    attribute.String("key", "user-123"),
)

// Record error in current span
if err != nil {
    otelexporter.RecordSpanError(ctx, err, "Failed to process item")
}

// Get IDs for logging correlation
traceID := otelexporter.GetTraceID(ctx)
spanID := otelexporter.GetSpanID(ctx)
```

### Resource Attributes

Add additional metadata to your telemetry:

```go
// Add a single attribute
exporter.AddResourceAttribute("deployment.id", "b1ff937a")

// Add multiple attributes
exporter.AddResourceAttributes(map[string]string{
    "team": "platform",
    "region": "us-west-2",
})
```

## Multiple Exporters

You can create multiple exporters with different configurations:

```go
// Create a custom exporter for a specific component
componentConfig := otelexporter.DefaultConfig()
componentConfig.ServiceName = "auth-service"
componentConfig.OTLPEndpoint = "localhost:4317"

componentExporter, err := otelexporter.NewExporter(ctx, 
    otelexporter.WithConfig(componentConfig),
)
if err != nil {
    // Handle error
}
defer componentExporter.Shutdown(ctx)

// Use the component-specific context
ctxWithComponent := componentExporter.WithContext(ctx)

// Operations using this context will be tagged with "auth-service"
```

## Advanced Configuration

For advanced use cases, you can configure every aspect of the exporter:

```go
// Create a custom logger
logger, _ := zap.NewProduction()

// Configure a custom exporter
exporter, err := otelexporter.NewExporter(ctx,
    otelexporter.WithLogger(logger),
    otelexporter.WithConfig(otelexporter.Config{
        ServiceName: "custom-service",
        ServiceVersion: "2.0.0",
        Environment: "staging",
        OTLPEndpoint: "collector:4317",
        TraceSamplingRatio: 0.25, // 25% sampling
        Timeout: 10 * time.Second,
        ResourceAttributes: map[string]string{
            "deployment.id": "abc123",
            "team": "platform",
        },
        BatchTimeout: 5 * time.Second,
        MaxExportBatchSize: 512,
        MaxQueueSize: 2048,
    }),
)
```

## Testing Utilities

The package provides functions to help with testing:

```go
// Reset the default exporter for testing isolation
otelexporter.ResetDefaultForTest()

// Mock tracers for testing
spanExporter := tracetest.NewInMemoryExporter()
tracerProvider := sdktrace.NewTracerProvider(
    sdktrace.WithSampler(sdktrace.AlwaysSample()),
    sdktrace.WithSyncer(spanExporter),
)

exporter, _ := otelexporter.NewExporter(ctx,
    otelexporter.WithTracerProvider(tracerProvider),
)

// Later check the captured spans
spans := spanExporter.GetSpans()
```
# HTTP Instrumentation

The `otelexporter` package provides built-in HTTP instrumentation capabilities for both servers and clients.

## HTTP Server Middleware

The package includes middleware for HTTP servers that automatically:

- Captures request metrics including request count and duration
- Creates and manages trace spans for each HTTP request
- Adds HTTP request attributes to spans (method, path, user-agent, etc.)
- Marks 5xx responses as errors
- Handles trace context propagation from incoming requests

Example usage:

```go
import (
    "net/http"
    "github.com/jzx17/otelexporter"
)

func setupServer() {
    // Initialize the exporter
    exporter, err := otelexporter.NewExporter(context.Background())
    if err != nil {
        log.Fatalf("Failed to create exporter: %v", err)
    }

    // Create your handler
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // The request context already contains the active span
        // You can create child spans if needed:
        ctx, span := otelexporter.TracerFromContext(r.Context(), "").Start(r.Context(), "handler-operation")
        defer span.End()

        // Handle the request...
        w.Write([]byte("Hello, world!"))
    })

    // Wrap the handler with the middleware
    wrappedHandler := exporter.HTTPMiddleware("my-service")(handler)

    // Start the server with the wrapped handler
    http.ListenAndServe(":8080", wrappedHandler)
}
```

## HTTP Client Instrumentation

The package also provides instrumentation for HTTP clients:

- Automatically traces outgoing HTTP requests
- Injects trace context into outgoing request headers
- Captures HTTP client metrics
- Marks errors and 5xx responses with error status

Example usage:

```go
import (
    "net/http"
    "github.com/jzx17/otelexporter"
)

func makeRequest(ctx context.Context, exporter *otelexporter.Exporter) error {
    // Create or wrap an HTTP client
    client := exporter.WrapHTTPClient(http.DefaultClient, "my-client")

    // Create a request with the current context
    req, err := http.NewRequestWithContext(ctx, "GET", "https://example.com", nil)
    if err != nil {
        return err
    }

    // Make the request - it will be automatically traced
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // Process the response...
    return nil
}
```

## Context Propagation

The HTTP instrumentation automatically handles trace context propagation:

- For servers, it extracts context from incoming request headers
- For clients, it injects context into outgoing request headers

The default propagator uses W3C Trace Context, but you can configure a custom propagator:

```go
import (
    "go.opentelemetry.io/otel/propagation"
    "github.com/jzx17/otelexporter"
)

func configureExporter() {
    // Create a custom propagator
    propagator := propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    )

    // Create an exporter with the custom propagator
    exporter, err := otelexporter.NewExporter(
        context.Background(),
        otelexporter.WithPropagator(propagator),
    )
    if err != nil {
        log.Fatalf("Failed to create exporter: %v", err)
    }

    // Use the exporter...
}
```


## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.