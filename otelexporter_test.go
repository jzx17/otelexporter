package otelexporter_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jzx17/otelexporter"
)

// A simple logger implementation for testing
type testLogger struct {
	infoMessages  []string
	errorMessages []string
	debugMessages []string
	warnMessages  []string
	syncError     error
}

func newTestLogger() *testLogger {
	return &testLogger{
		infoMessages:  make([]string, 0),
		errorMessages: make([]string, 0),
		debugMessages: make([]string, 0),
		warnMessages:  make([]string, 0),
	}
}

func (l *testLogger) Info(msg string, fields ...zap.Field) {
	l.infoMessages = append(l.infoMessages, msg)
}

func (l *testLogger) Error(msg string, fields ...zap.Field) {
	l.errorMessages = append(l.errorMessages, msg)
}

func (l *testLogger) Debug(msg string, fields ...zap.Field) {
	l.debugMessages = append(l.debugMessages, msg)
}

func (l *testLogger) Warn(msg string, fields ...zap.Field) {
	l.warnMessages = append(l.warnMessages, msg)
}

func (l *testLogger) Sync() error {
	return l.syncError
}

// Set the error to be returned by Sync
func (l *testLogger) WithSyncError(err error) *testLogger {
	l.syncError = err
	return l
}

var _ = Describe("Exporter", func() {
	var (
		exporter *otelexporter.Exporter
	)

	Describe("Creating a new exporter", func() {
		It("should create an exporter with default configuration", func() {
			var err error
			exporter, err = otelexporter.NewExporter()
			Expect(err).NotTo(HaveOccurred())
			Expect(exporter).NotTo(BeNil())
		})

		It("should use the provided logger", func() {
			// Create a test logger instead of a mock
			testLogger := newTestLogger()

			var err error
			exporter, err = otelexporter.NewExporter(
				otelexporter.WithLogger(testLogger),
			)
			Expect(err).NotTo(HaveOccurred())

			// Verify the logger is set correctly
			Expect(exporter.Logger()).To(Equal(testLogger))

			// Log a message and verify it was recorded
			exporter.Logger().Info("test log")
			Expect(testLogger.infoMessages).To(ContainElement("test log"))
		})

		It("should create an exporter with custom configuration", func() {
			testLogger := newTestLogger()

			cfg := otelexporter.DefaultConfig()
			cfg.ServiceName = "test-service"
			cfg.Environment = "test-env"
			cfg.ServiceVersion = "1.2.3"
			cfg.TraceSamplingRatio = 0.5

			var err error
			exporter, err = otelexporter.NewExporter(
				otelexporter.WithLogger(testLogger),
				otelexporter.WithConfig(cfg),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(exporter).NotTo(BeNil())

			// We can't easily verify the config was set, but we can
			// check that we can use the exporter successfully
			tracer := exporter.Tracer("test-component")
			Expect(tracer).NotTo(BeNil())
		})
	})

	Describe("Tracer and Meter functions", func() {
		BeforeEach(func() {
			var err error
			exporter, err = otelexporter.NewExporter()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return a valid tracer", func() {
			tracer := exporter.Tracer("test-component")
			Expect(tracer).NotTo(BeNil())

			// Start a span to verify the tracer works
			_, span := tracer.Start(context.Background(), "test-span")
			Expect(span).NotTo(BeNil())
			Expect(span.SpanContext().IsValid()).To(BeTrue())
			span.End()
		})

		It("should return a valid meter", func() {
			meter := exporter.Meter("test-component")
			Expect(meter).NotTo(BeNil())
		})

		It("should use the service name for tracer when no name provided", func() {
			tracer := exporter.Tracer("")
			Expect(tracer).NotTo(BeNil())

			// We can't easily verify the name internally, but we can
			// check that the tracer works
			_, span := tracer.Start(context.Background(), "test-span")
			Expect(span).NotTo(BeNil())
			span.End()
		})
	})

	Describe("Shutdown behavior", func() {
		It("should shutdown without error", func() {
			testLogger := newTestLogger()

			var err error
			exporter, err = otelexporter.NewExporter(
				otelexporter.WithLogger(testLogger),
			)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = exporter.Shutdown(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle common logger sync errors", func() {
			// Create a test logger that returns the common sync error
			testLogger := newTestLogger().WithSyncError(
				errors.New("sync /dev/stdout: inappropriate ioctl for device"),
			)

			var err error
			exporter, err = otelexporter.NewExporter(
				otelexporter.WithLogger(testLogger),
			)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = exporter.Shutdown(ctx)

			// With our fixed implementation, this error should be properly handled
			Expect(err).NotTo(HaveOccurred())
		})

		It("should report real logger sync errors", func() {
			// Create a test logger that returns a real error
			testLogger := newTestLogger().WithSyncError(
				errors.New("real error"),
			)

			var err error
			exporter, err = otelexporter.NewExporter(
				otelexporter.WithLogger(testLogger),
			)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = exporter.Shutdown(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger sync failed"))
		})
	})

	Describe("Default Exporter", func() {
		BeforeEach(func() {
			otelexporter.ResetDefaultForTest()
		})

		It("should initialize a default exporter", func() {
			err := otelexporter.InitDefault()
			Expect(err).NotTo(HaveOccurred())

			defaultExp := otelexporter.Default()
			Expect(defaultExp).NotTo(BeNil())
		})

		It("should return the same instance for multiple Default() calls", func() {
			err := otelexporter.InitDefault()
			Expect(err).NotTo(HaveOccurred())

			defaultExp1 := otelexporter.Default()
			defaultExp2 := otelexporter.Default()

			Expect(defaultExp1).To(BeIdenticalTo(defaultExp2))
		})

		It("should panic if Default() called before InitDefault()", func() {
			otelexporter.ResetDefaultForTest()

			Expect(func() {
				otelexporter.Default()
			}).To(Panic())
		})

		It("should initialize with provided options", func() {
			// Use our test logger instead of a mock
			testLogger := newTestLogger()

			err := otelexporter.InitDefault(
				otelexporter.WithLogger(testLogger),
			)
			Expect(err).NotTo(HaveOccurred())

			defaultExp := otelexporter.Default()
			Expect(defaultExp).NotTo(BeNil())

			// Verify the logger was set correctly
			Expect(defaultExp.Logger()).To(Equal(testLogger))

			// Log a test message and verify it was recorded
			defaultExp.Logger().Info("test message")
			Expect(testLogger.infoMessages).To(ContainElement("test message"))
		})
	})
})
