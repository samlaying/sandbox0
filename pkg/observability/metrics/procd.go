package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ProcdMetrics holds Prometheus metrics for the procd service.
type ProcdMetrics struct {
	ContextOperationsTotal   *prometheus.CounterVec
	ContextOperationDuration *prometheus.HistogramVec
	ContextsActive           prometheus.Gauge
	ContextsPaused           prometheus.Gauge
	VolumeOperationsTotal    *prometheus.CounterVec
	VolumeOperationDuration  *prometheus.HistogramVec
	InitializeTotal          *prometheus.CounterVec
	InitializeDuration       prometheus.Histogram
	CleanupRunsTotal         *prometheus.CounterVec
}

func NewProcd(registry prometheus.Registerer) *ProcdMetrics {
	if registry == nil {
		return nil
	}

	factory := promauto.With(registry)
	return &ProcdMetrics{
		ContextOperationsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "procd_context_operations_total",
			Help: "Total number of procd context operations",
		}, []string{"operation", "status", "type"}),
		ContextOperationDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "procd_context_operation_duration_seconds",
			Help:    "Duration of procd context operations",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation", "type"}),
		ContextsActive: factory.NewGauge(prometheus.GaugeOpts{
			Name: "procd_contexts_active",
			Help: "Number of active procd contexts",
		}),
		ContextsPaused: factory.NewGauge(prometheus.GaugeOpts{
			Name: "procd_contexts_paused",
			Help: "Number of paused procd contexts",
		}),
		VolumeOperationsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "procd_volume_operations_total",
			Help: "Total number of procd volume operations",
		}, []string{"operation", "status"}),
		VolumeOperationDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "procd_volume_operation_duration_seconds",
			Help:    "Duration of procd volume operations",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
		InitializeTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "procd_initialize_total",
			Help: "Total number of procd initialize requests",
		}, []string{"status"}),
		InitializeDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "procd_initialize_duration_seconds",
			Help:    "Duration of procd initialize requests",
			Buckets: prometheus.DefBuckets,
		}),
		CleanupRunsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "procd_cleanup_runs_total",
			Help: "Total number of procd cleanup loop runs",
		}, []string{"status"}),
	}
}
