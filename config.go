package otelexporter

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the OpenTelemetry exporter.
type Config struct {
	ServiceName        string
	ServiceVersion     string
	Environment        string
	OTLPEndpoint       string // e.g., "localhost:4317"
	TraceSamplingRatio float64
	Timeout            time.Duration
	// New fields for enhanced configuration
	ResourceAttributes map[string]string
	BatchTimeout       time.Duration
	MaxExportBatchSize int
	MaxQueueSize       int
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
		ResourceAttributes: make(map[string]string),
		BatchTimeout:       5 * time.Second,
		MaxExportBatchSize: 512,
		MaxQueueSize:       2048,
	}
}

// FromEnv loads configuration values from environment variables, if present.
// Environment variables take precedence over the default values.
func (c *Config) FromEnv() {
	// Basic configuration
	if value := os.Getenv("OTEL_SERVICE_NAME"); value != "" {
		c.ServiceName = value
	}
	if value := os.Getenv("OTEL_SERVICE_VERSION"); value != "" {
		c.ServiceVersion = value
	}
	if value := os.Getenv("OTEL_ENVIRONMENT"); value != "" {
		c.Environment = value
	}
	if value := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); value != "" {
		c.OTLPEndpoint = value
	}

	// Sampling configuration
	if value := os.Getenv("OTEL_TRACE_SAMPLER_ARG"); value != "" {
		if ratio, err := strconv.ParseFloat(value, 64); err == nil {
			c.TraceSamplingRatio = ratio
		}
	}

	// Timeout configuration
	if value := os.Getenv("OTEL_EXPORTER_OTLP_TIMEOUT"); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			c.Timeout = duration
		}
	}

	// Batch configuration
	if value := os.Getenv("OTEL_BSP_SCHEDULE_DELAY"); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			c.BatchTimeout = duration
		}
	}
	if value := os.Getenv("OTEL_BSP_MAX_EXPORT_BATCH_SIZE"); value != "" {
		if size, err := strconv.Atoi(value); err == nil {
			c.MaxExportBatchSize = size
		}
	}
	if value := os.Getenv("OTEL_BSP_MAX_QUEUE_SIZE"); value != "" {
		if size, err := strconv.Atoi(value); err == nil {
			c.MaxQueueSize = size
		}
	}

	// Load any OTEL_RESOURCE_ATTRIBUTES
	if value := os.Getenv("OTEL_RESOURCE_ATTRIBUTES"); value != "" {
		// Parse comma-separated key=value pairs
		attributes := parseResourceAttributes(value)
		for k, v := range attributes {
			c.ResourceAttributes[k] = v
		}
	}
}

// parseResourceAttributes parses a comma-separated list of key=value pairs
// into a map.
func parseResourceAttributes(attributesStr string) map[string]string {
	attributes := make(map[string]string)

	// If empty, return empty map
	if attributesStr == "" {
		return attributes
	}

	// Split on commas
	pairs := splitAndTrim(attributesStr, ',')

	// Parse each pair
	for _, pair := range pairs {
		// Split on equals
		kv := splitAndTrim(pair, '=')
		if len(kv) == 2 {
			attributes[kv[0]] = kv[1]
		} else {
			// Log a warning or handle invalid format
			fmt.Printf("Warning: invalid attribute format: %s\n", pair)
		}
	}

	return attributes
}

// splitAndTrim splits a string on the given separator and trims whitespace from each part
func splitAndTrim(s string, sep rune) []string {
	var result []string
	current := ""

	for _, c := range s {
		if c == sep {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	// Trim whitespace
	for i, v := range result {
		result[i] = trim(v)
	}

	return result
}

// trim removes leading and trailing whitespace from a string
func trim(s string) string {
	start := 0
	end := len(s)

	// Find start
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}

	// Find end
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}
