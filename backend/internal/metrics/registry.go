
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type BusinessSnapshot struct {
	
	TotalGPUs           int
	UsedGPUs            int
	AvailableGPUs       int
	GPUUtilization      float64
	TotalMemoryGB       int
	UsedMemoryGB        int
	MemoryUtilization   float64
	
	RunningJobs   int
	PendingJobs   int
	CompletedJobs int
	TotalJobs     int
	
	InferenceTotal        int
	InferenceReady        int
	InferenceReadyReplica float64
	
	Datasets []DatasetCache
	
	WorkspaceTotal  int
	WorkspaceRunning int
	WorkspaceByStatus map[string]int
	
	ClustersDiscovered   int64
	GPUsDiscovered       int64
	JobsSubmitted        int64
	GPUsSubmitted        int64
	AssignmentsCompleted int64
	GPUsAllocated        int64
}

type DatasetCache struct {
	Name          string
	CachedPercent float64
}

type Registry struct {
	prom     *prometheus.Registry
	duration *prometheus.HistogramVec
	total    *prometheus.CounterVec
}

func NewRegistry(snapFn func() (BusinessSnapshot, error)) *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{
		Namespace:    "",
		ReportErrors: false,
	}))
	if snapFn != nil {
		reg.MustRegister(&businessCollector{snapFn: snapFn})
	}

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP 请求处理耗时（秒）",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status"})
	total := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP 请求总数（按方法/路径/状态码）",
	}, []string{"method", "path", "status"})
	reg.MustRegister(duration, total)

	return &Registry{prom: reg, duration: duration, total: total}
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.prom, promhttp.HandlerOpts{
		Registry:          r.prom,
		EnableOpenMetrics: true,
		Timeout:           10 * time.Second,
	})
}

func (r *Registry) ObserveRequest(method, path, status string, seconds float64) {
	r.duration.WithLabelValues(method, path, status).Observe(seconds)
	r.total.WithLabelValues(method, path, status).Inc()
}