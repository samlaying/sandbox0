# JuiceFS Initialization Package

This package provides automatic initialization for JuiceFS filesystems.

## Features

- **Automatic Bucket Creation**: Attempts to create S3 bucket if it doesn't exist (for S3-compatible storage that supports auto-creation like MinIO/RustFS)
- **Metadata Initialization**: Formats JuiceFS metadata store if not already initialized
- **Idempotent**: Safe to run multiple times - will skip if already initialized

## How It Works

1. **Check Bucket Existence**: Verifies if the S3 bucket exists
2. **Create Bucket**: If bucket doesn't exist, creates a marker file to trigger auto-creation (for MinIO/RustFS)
3. **Check Format**: Checks if JuiceFS metadata is already formatted
4. **Format Filesystem**: If not formatted, initializes JuiceFS metadata with proper configuration

## Integration

The initialization is automatically called when `storage-proxy` starts up, in `cmd/storage-proxy/main.go`:

```go
// Initialize JuiceFS filesystem if not already initialized
if err := initializeJuiceFS(cfg, zapLogger); err != nil {
    zapLogger.Fatal("Failed to initialize JuiceFS", zap.Error(err))
}
```

## Configuration

All configuration is read from the storage-proxy config file:

```yaml
meta_url: "postgres://juicefs:juicefs@postgresql:5432/juicefs?sslmode=disable"
s3_bucket: "sandbox0-data"
s3_region: "us-east-1"
s3_endpoint: "http://rustfs:9000"
s3_access_key: "rustfsadmin"
s3_secret_key: "rustfsadmin"
```

## Behavior Notes

### S3 Bucket Creation

**Important**: JuiceFS SDK does **NOT** automatically create S3 buckets. This package handles bucket creation in the following ways:

1. **For MinIO/RustFS**: These S3-compatible storage systems often support auto-bucket-creation on first PUT operation. The initializer creates a marker file to trigger this.

2. **For AWS S3**: Buckets must be pre-created. The initialization will fail if the bucket doesn't exist.

3. **Manual Creation**: If auto-creation fails, you'll need to manually create the bucket:
   ```bash
   # For MinIO/RustFS
   mc mb minio/sandbox0-data
   
   # For AWS S3
   aws s3 mb s3://sandbox0-data --region us-east-1
   ```

### JuiceFS Format

The format is created with these defaults:
- **Name**: `sandbox0`
- **Block Size**: 4MB (4096 KB)
- **Compression**: LZ4
- **Trash Days**: 1 day

## Error Handling

- If bucket creation fails with a warning, the process continues (bucket may already exist)
- If metadata initialization fails, the process stops and returns an error
- All errors are logged with appropriate context

## Manual Initialization

If you prefer to manually initialize JuiceFS, you can use the JuiceFS CLI:

```bash
juicefs format \
  --storage s3 \
  --bucket http://rustfs:9000/sandbox0-data \
  --access-key rustfsadmin \
  --secret-key rustfsadmin \
  "postgres://juicefs:juicefs@postgresql:5432/juicefs?sslmode=disable" \
  sandbox0
```

If manually initialized, the automatic initialization will detect this and skip the process.

## Testing

See `init_test.go` for unit tests. Full integration testing requires:
- Running PostgreSQL instance
- Running S3-compatible storage (MinIO/RustFS/AWS S3)

## Troubleshooting

### "bucket does not exist" Error

**Solution**: Create the bucket manually before starting storage-proxy:
```bash
mc mb minio/sandbox0-data
```

### "failed to load juicefs format" Error

This usually means the metadata database is empty or unreachable. Check:
1. PostgreSQL is running and accessible
2. The `juicefs` database exists
3. Credentials are correct

### "initialize metadata: already formatted" Info

This is normal - it means JuiceFS is already initialized. No action needed.
