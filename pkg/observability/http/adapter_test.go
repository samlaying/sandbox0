package http

import (
	"net/http"
	"testing"
)

func TestInternalServiceTransportDisablesProxyFromEnvironment(t *testing.T) {
	transport, ok := InternalServiceTransport().(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", InternalServiceTransport())
	}
	if transport.Proxy != nil {
		t.Fatal("expected internal service transport to bypass environment proxy resolution")
	}
}
