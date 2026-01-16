package juicefs

import (
	"testing"

	"go.uber.org/zap"
)

func TestInitializer_Initialize(t *testing.T) {
	// This is a basic test that verifies the initializer can be created
	// Real testing would require a running PostgreSQL and S3-compatible storage

	logger := zap.NewNop()

	config := &InitConfig{
		MetaURL:     "postgres://test:test@localhost:5432/test",
		S3Bucket:    "test-bucket",
		S3Region:    "us-east-1",
		S3Endpoint:  "http://localhost:9000",
		S3AccessKey: "test",
		S3SecretKey: "test",
	}

	initializer := NewInitializer(config, logger)
	if initializer == nil {
		t.Fatal("Failed to create initializer")
	}

	// Note: We can't actually run Initialize() without real infrastructure
	// In production, this would be tested with integration tests
}

func TestInitializer_buildBucketURL(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name     string
		config   *InitConfig
		expected string
	}{
		{
			name: "with custom endpoint",
			config: &InitConfig{
				S3Endpoint: "http://rustfs:9000",
				S3Bucket:   "sandbox0-data",
			},
			expected: "http://rustfs:9000/sandbox0-data",
		},
		{
			name: "with trailing slash in endpoint",
			config: &InitConfig{
				S3Endpoint: "http://rustfs:9000/",
				S3Bucket:   "sandbox0-data",
			},
			expected: "http://rustfs:9000/sandbox0-data",
		},
		{
			name: "with AWS S3",
			config: &InitConfig{
				S3Region: "us-west-2",
				S3Bucket: "my-bucket",
			},
			expected: "https://s3.us-west-2.amazonaws.com/my-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initializer := NewInitializer(tt.config, logger)
			result := initializer.buildBucketURL()
			if result != tt.expected {
				t.Errorf("buildBucketURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}
