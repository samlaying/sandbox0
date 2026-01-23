package clients

import (
	"net/http"
	"time"
)

// ManagerClient is a thin wrapper around the manager API.
type ManagerClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewManagerClient creates a manager client with defaults.
func NewManagerClient(baseURL, token string, timeout time.Duration) *ManagerClient {
	return &ManagerClient{
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			Timeout: timeout,
		},
	}
}
