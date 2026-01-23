package clients

import (
	"net/http"
	"time"
)

// StorageClient is a thin wrapper around the storage-proxy API.
type StorageClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewStorageClient creates a storage-proxy client with defaults.
func NewStorageClient(baseURL, token string, timeout time.Duration) *StorageClient {
	return &StorageClient{
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			Timeout: timeout,
		},
	}
}
