package clients

import (
	"net/http"
	"time"
)

// GatewayClient is a thin wrapper around the internal-gateway API.
type GatewayClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewGatewayClient creates an internal-gateway client with defaults.
func NewGatewayClient(baseURL, token string, timeout time.Duration) *GatewayClient {
	return &GatewayClient{
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			Timeout: timeout,
		},
	}
}
