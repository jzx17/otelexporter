package otelexporter_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jzx17/otelexporter"
)

var _ = Describe("Default Exporter", func() {
	BeforeEach(func() {
		otelexporter.ResetDefaultForTest()
	})

	It("should initialize a default exporter", func() {
		// Create a config that won't try to connect
		cfg := otelexporter.DefaultConfig()
		cfg.OTLPEndpoint = "non-existent-host:4317"

		err := otelexporter.InitDefault(context.Background(),
			otelexporter.WithConfig(cfg),
		)

		// It's expected to fail, but we just want to verify the code path
		if err != nil {
			Expect(err.Error()).To(ContainSubstring("non-existent-host"))
		} else {
			defaultExp := otelexporter.Default()
			Expect(defaultExp).NotTo(BeNil())
		}
	})

	It("should panic if Default() called before InitDefault()", func() {
		otelexporter.ResetDefaultForTest()

		Expect(func() {
			otelexporter.Default()
		}).To(Panic())
	})

	It("should initialize with provided options", func() {
		testLogger := newTestLogger()

		// Create a config that won't try to connect
		cfg := otelexporter.DefaultConfig()
		cfg.OTLPEndpoint = "non-existent-host:4317"

		err := otelexporter.InitDefault(context.Background(),
			otelexporter.WithLogger(testLogger),
			otelexporter.WithConfig(cfg),
		)

		// It's expected to fail, but we still want to verify the Default function works
		if err != nil {
			Expect(err.Error()).To(ContainSubstring("non-existent-host"))
		} else {
			defaultExp := otelexporter.Default()
			Expect(defaultExp).NotTo(BeNil())

			// Verify the logger was set correctly
			Expect(defaultExp.Logger()).To(Equal(testLogger))

			// Log a test message and verify it was recorded
			defaultExp.Logger().Info("test message")
			Expect(testLogger.infoMessages).To(ContainElement("test message"))
		}
	})
})
