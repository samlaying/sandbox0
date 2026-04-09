package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// RegionalGatewayMetrics holds Prometheus metrics for regional-gateway routing decisions.
type RegionalGatewayMetrics struct {
	SandboxRouteTotal *prometheus.CounterVec
	FallbacksTotal    *prometheus.CounterVec
}

func NewRegionalGateway(registry prometheus.Registerer) *RegionalGatewayMetrics {
	if registry == nil {
		return nil
	}

	factory := promauto.With(registry)
	return &RegionalGatewayMetrics{
		SandboxRouteTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "regional_gateway_sandbox_route_total",
			Help: "Total number of sandbox routing decisions",
		}, []string{"target", "mode"}),
		FallbacksTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "regional_gateway_fallback_total",
			Help: "Total number of regional-gateway routing fallbacks",
		}, []string{"from", "to", "reason"}),
	}
}
