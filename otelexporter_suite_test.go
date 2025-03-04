package otelexporter_test

import (
	"context"
	"net"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockNetDialer is a test dialer that immediately fails all connections
type mockNetDialer struct{}

func (d *mockNetDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Immediately return connection refused error for all attempts
	return nil, &net.OpError{
		Op:     "dial",
		Net:    network,
		Source: nil,
		Addr:   nil,
		Err:    &net.DNSError{Err: "connection refused", Name: address, IsTimeout: false},
	}
}

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
	originalDialer := net.DefaultResolver.Dial

	// Override with our mock dialer that fails immediately
	mockDialer := &mockNetDialer{}
	net.DefaultResolver.Dial = mockDialer.DialContext

	// Return a cleanup function to restore the original settings
	return func() {
		net.DefaultResolver.Dial = originalDialer
	}
}

var _ = BeforeSuite(func() {
	// Apply short timeouts for all tests in the suite
	cleanup := setupShortTimeouts()
	DeferCleanup(cleanup)

	// Add a short delay to ensure all resources are properly initialized
	time.Sleep(1 * time.Millisecond)
})
