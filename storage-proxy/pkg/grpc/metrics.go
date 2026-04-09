package grpc

import (
	"context"
	"time"

	obsmetrics "github.com/sandbox0-ai/sandbox0/pkg/observability/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryMetricsInterceptor records request count and latency for storage-proxy gRPC methods.
func UnaryMetricsInterceptor(metrics *obsmetrics.StorageProxyMetrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if metrics == nil {
			return handler(ctx, req)
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		metrics.GRPCRequestsTotal.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		metrics.GRPCRequestDuration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		return resp, err
	}
}
