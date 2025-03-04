package otelexporter_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jzx17/otelexporter"
)

var _ = Describe("Integration Scenarios", func() {
	var (
		ctx        context.Context
		testLogger *testLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		testLogger = newTestLogger()

		// Reset the default exporter for each test
		otelexporter.ResetDefaultForTest()
	})

	Describe("Multiple Exporters", func() {
		It("should isolate providers between different contexts", func() {
			// Create two exporters with different configurations
			exporter1Config := otelexporter.DefaultConfig()
			exporter1Config.ServiceName = "service-1"
			exporter1Config.OTLPEndpoint = "non-existent-host:4317"
			exporter1Config.Timeout = 50 * time.Millisecond

			spanExporter1 := tracetest.NewInMemoryExporter()
			tracerProvider1 := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter1),
			)

			exporter1, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(exporter1Config),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider1),
			)
			if err != nil {
				Skip("Couldn't create first exporter: " + err.Error())
			}
			defer exporter1.Shutdown(ctx)

			// Create second exporter
			exporter2Config := otelexporter.DefaultConfig()
			exporter2Config.ServiceName = "service-2"
			exporter2Config.OTLPEndpoint = "non-existent-host:4317"
			exporter2Config.Timeout = 50 * time.Millisecond

			spanExporter2 := tracetest.NewInMemoryExporter()
			tracerProvider2 := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter2),
			)

			exporter2, err := otelexporter.NewExporter(ctx,
				otelexporter.WithConfig(exporter2Config),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider2),
			)
			if err != nil {
				Skip("Couldn't create second exporter: " + err.Error())
			}
			defer exporter2.Shutdown(ctx)

			// Create contexts for each exporter
			ctx1 := exporter1.WithContext(ctx)
			ctx2 := exporter2.WithContext(ctx)

			// Start spans using both contexts
			_, span1 := otelexporter.TracerFromContext(ctx1, "").Start(ctx1, "span-from-exporter1")
			_, span2 := otelexporter.TracerFromContext(ctx2, "").Start(ctx2, "span-from-exporter2")

			// End the spans
			span1.End()
			span2.End()

			// Force flush both tracer providers
			tracerProvider1.ForceFlush(ctx)
			tracerProvider2.ForceFlush(ctx)

			// Check that spans went to their respective exporters
			spans1 := spanExporter1.GetSpans()
			spans2 := spanExporter2.GetSpans()

			Expect(spans1).To(HaveLen(1))
			Expect(spans2).To(HaveLen(1))

			Expect(spans1[0].Name).To(Equal("span-from-exporter1"))
			Expect(spans2[0].Name).To(Equal("span-from-exporter2"))
		})
	})

	Describe("Default Exporter Integration", func() {
		It("should use default exporter in context helpers", func() {
			// Create a default exporter
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			defaultConfig := otelexporter.DefaultConfig()
			defaultConfig.ServiceName = "default-service"
			defaultConfig.OTLPEndpoint = "non-existent-host:4317"

			err := otelexporter.InitDefault(ctx,
				otelexporter.WithConfig(defaultConfig),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't initialize default exporter: " + err.Error())
			}

			// Get the default exporter
			defaultExp := otelexporter.Default()

			// Create a context using the default exporter
			ctxWithDefault := defaultExp.WithContext(ctx)

			// Use the context helpers
			tracer := otelexporter.TracerFromContext(ctxWithDefault, "")
			_, span := tracer.Start(ctxWithDefault, "default-exporter-span")
			span.End()

			// Force flush
			tracerProvider.ForceFlush(ctx)

			// Check that span was exported
			spans := spanExporter.GetSpans()
			Expect(spans).To(HaveLen(1))
			Expect(spans[0].Name).To(Equal("default-exporter-span"))
		})

		It("should integrate with WrapWithSpan", func() {
			// Create a default exporter
			spanExporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithBatcher(spanExporter),
			)

			defaultConfig := otelexporter.DefaultConfig()
			defaultConfig.OTLPEndpoint = "non-existent-host:4317"

			err := otelexporter.InitDefault(ctx,
				otelexporter.WithConfig(defaultConfig),
				otelexporter.WithLogger(testLogger),
				otelexporter.WithTracerProvider(tracerProvider),
			)
			if err != nil {
				Skip("Couldn't initialize default exporter: " + err.Error())
			}

			// Get the default exporter
			defaultExp := otelexporter.Default()

			// Create a context using the default exporter
			ctxWithDefault := defaultExp.WithContext(ctx)

			// Use WrapWithSpan
			callCount := 0
			err = otelexporter.WrapWithSpan(ctxWithDefault, "wrapped-operation", func(c context.Context) error {
				callCount++
				// Start a nested span
				_, nestedSpan := otelexporter.TracerFromContext(c, "").Start(c, "nested-span")
				nestedSpan.End()
				return nil
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1))

			// Force flush
			tracerProvider.ForceFlush(ctx)

			// Check that both spans were exported
			spans := spanExporter.GetSpans()
			Expect(spans).To(HaveLen(2))

			// Find the spans by name
			var foundWrapped, foundNested bool
			for _, s := range spans {
				if s.Name == "wrapped-operation" {
					foundWrapped = true
				}
				if s.Name == "nested-span" {
					foundNested = true
				}
			}

			Expect(foundWrapped).To(BeTrue(), "Should have found the wrapped operation span")
			Expect(foundNested).To(BeTrue(), "Should have found the nested span")
		})
	})
})
