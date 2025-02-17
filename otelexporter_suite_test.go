package otelexporter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOtelExporter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OtelExporter Suite")
}
