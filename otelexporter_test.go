package otelexporter_test

import (
	"context"
	"otel-exporter/pkg/otelexporter"
	"otel-exporter/pkg/otelexporter/mock"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"go.uber.org/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exporter", func() {
	var (
		exporter   *otelexporter.Exporter
		ctrl       *gomock.Controller
		mockLogger *mocks.MockLogger
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLogger = mocks.NewMockLogger(ctrl)
		// Pass the custom (mock) logger so we can intercept log calls.
		exporter = otelexporter.NewExporter(otelexporter.WithLogger(mockLogger))
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("should use the provided logger", func() {
		msg := "test log"
		// Expect that the Info method will be called exactly once with the message.
		mockLogger.EXPECT().Info(msg, gomock.Any()).Times(1)
		exporter.Logger().Info(msg, zap.Any("key", "value"))
	})

	It("should return a valid tracer", func() {
		var tracer trace.Tracer = exporter.Tracer()
		Expect(tracer).NotTo(BeNil())
		// Optionally, start a span to ensure the tracer works.
		_, span := tracer.Start(context.Background(), "test-span")
		Expect(span).NotTo(BeNil())
		span.End()
	})

	It("should return a valid meter", func() {
		meter := exporter.Meter()
		Expect(meter).NotTo(BeNil())
	})

	It("should shutdown without error", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mockLogger.EXPECT().Sync().Return(nil).Times(1)
		err := exporter.Shutdown(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Default Exporter", func() {
		It("should return a singleton default exporter", func() {
			def1 := otelexporter.Default()
			def2 := otelexporter.Default()
			Expect(def1).To(Equal(def2))
		})
	})
})
