package power

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	operationsTotal    *prometheus.CounterVec
	operationDuration  *prometheus.HistogramVec
	resourceUsageBytes *prometheus.GaugeVec
}

func NewMetrics(registry prometheus.Registerer) *Metrics {
	if registry == nil {
		return nil
	}

	factory := promauto.With(registry)
	return &Metrics{
		operationsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "ctld_power_operations_total",
			Help: "Total number of ctld pause/resume operations",
		}, []string{"operation", "status"}),
		operationDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ctld_power_operation_duration_seconds",
			Help:    "Duration of ctld pause/resume operations",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		resourceUsageBytes: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ctld_power_resource_usage_bytes",
			Help: "Latest sandbox resource usage reported during ctld pause",
		}, []string{"metric"}),
	}
}

func (m *Metrics) observeOperation(operation, status string, duration time.Duration) {
	if m == nil {
		return
	}
	m.operationsTotal.WithLabelValues(operation, status).Inc()
	m.operationDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

func (m *Metrics) recordPauseUsage(usageBytes, workingSetBytes, rssBytes int64) {
	if m == nil {
		return
	}
	m.resourceUsageBytes.WithLabelValues("container_memory_usage").Set(float64(usageBytes))
	m.resourceUsageBytes.WithLabelValues("container_memory_working_set").Set(float64(workingSetBytes))
	m.resourceUsageBytes.WithLabelValues("total_memory_rss").Set(float64(rssBytes))
}
