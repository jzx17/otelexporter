package otelexporter_test

import (
	"net"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOtelExporter(t *testing.T) {
	RegisterFailHandler(Fail)

	// Set more restrictive test timeouts to catch hanging tests
	SetDefaultEventuallyTimeout(50 * time.Millisecond)
	SetDefaultEventuallyPollingInterval(5 * time.Millisecond)
	SetDefaultConsistentlyDuration(50 * time.Millisecond)
	SetDefaultConsistentlyPollingInterval(5 * time.Millisecond)

	RunSpecs(t, "OtelExporter Suite")
}

// setupShortTimeouts configures global network timeouts to be very short
// to prevent long waits when connecting to unreachable endpoints
func setupShortTimeouts() (cleanup func()) {
	// Save original settings
	originalDialContext := net.DefaultResolver.Dial

	// Override with much shorter timeouts
	dialer := &net.Dialer{
		Timeout:   5 * time.Millisecond, // Reduced from 10ms to 5ms
		KeepAlive: -1,                   // Disable keep-alive to avoid lingering connections
	}
	net.DefaultResolver.Dial = dialer.DialContext

	// Return a cleanup function to restore the original settings
	return func() {
		net.DefaultResolver.Dial = originalDialContext
	}
}

var _ = BeforeSuite(func() {
	// Apply short timeouts for all tests in the suite
	cleanup := setupShortTimeouts()
	DeferCleanup(cleanup)

	// Add a short delay to ensure all resources are properly initialized
	time.Sleep(1 * time.Millisecond)
})
