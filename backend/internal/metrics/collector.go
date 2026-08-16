package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type businessCollector struct {
	snapFn func() (BusinessSnapshot, error)
	descs  map[string]*prometheus.Desc
}

func newBusinessDesc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(name, help, labels, nil)
}

func (c *businessCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.descs == nil {
		c.initDescs()
	}
	for _, d := range c.descs {
		ch <- d
	}
}

func (c *businessCollector) initDescs() {
	c.descs = map[string]*prometheus.Desc{
		"fuze_gpu_total":                  newBusinessDesc("fuze_gpu_total", "集群 GPU 总数"),
		"fuze_gpu_used":                   newBusinessDesc("fuze_gpu_used", "已分配 GPU 数量"),
		"fuze_gpu_available":              newBusinessDesc("fuze_gpu_available", "可用 GPU 数量"),
		"fuze_gpu_utilization_percent":    newBusinessDesc("fuze_gpu_utilization_percent", "GPU 利用率(%)"),
		"fuze_memory_total_gb":            newBusinessDesc("fuze_memory_total_gb", "集群显存/内存总量(GB)"),
		"fuze_memory_used_gb":             newBusinessDesc("fuze_memory_used_gb", "已用显存/内存(GB)"),
		"fuze_memory_utilization_percent": newBusinessDesc("fuze_memory_utilization_percent", "显存/内存利用率(%)"),
		"fuze_jobs":                       newBusinessDesc("fuze_jobs", "各状态任务数量", "status"),
		"fuze_inference_services_total":   newBusinessDesc("fuze_inference_services_total", "推理服务总数"),
		"fuze_inference_services_ready":   newBusinessDesc("fuze_inference_services_ready", "就绪的推理服务数量"),
		"fuze_inference_ready_replicas":   newBusinessDesc("fuze_inference_ready_replicas", "推理服务就绪副本总数"),
		"fuze_dataset_cached_percent":     newBusinessDesc("fuze_dataset_cached_percent", "数据集缓存百分比", "dataset"),
		"fuze_events_clusters_discovered":   newBusinessDesc("fuze_events_clusters_discovered", "累计发现的集群数"),
		"fuze_events_gpus_discovered":       newBusinessDesc("fuze_events_gpus_discovered", "累计发现的 GPU 卡数"),
		"fuze_events_jobs_submitted":        newBusinessDesc("fuze_events_jobs_submitted", "累计提交的任务数"),
		"fuze_events_gpus_submitted":        newBusinessDesc("fuze_events_gpus_submitted", "累计提交的 GPU 卡数"),
		"fuze_events_assignments_completed": newBusinessDesc("fuze_events_assignments_completed", "累计完成的分配数"),
		"fuze_events_gpus_allocated":        newBusinessDesc("fuze_events_gpus_allocated", "累计分配的 GPU 卡数"),
		"fuze_workspaces_total":             newBusinessDesc("fuze_workspaces_total", "Notebook 工作空间总数"),
		"fuze_workspaces_running":           newBusinessDesc("fuze_workspaces_running", "处于 running 状态的工作空间数"),
		"fuze_workspaces":                   newBusinessDesc("fuze_workspaces", "各状态工作空间数量", "status"),
	}
}

func (c *businessCollector) Collect(ch chan<- prometheus.Metric) {
	if c.descs == nil {
		c.initDescs()
	}
	snap, err := c.snapFn()
	if err != nil {
		
		ch <- prometheus.MustNewConstMetric(c.descs["fuze_gpu_total"], prometheus.GaugeValue, 0)
		return
	}

	gauge := func(name string, v float64) {
		ch <- prometheus.MustNewConstMetric(c.descs[name], prometheus.GaugeValue, v)
	}
	
	labeledGauge := func(name, labelName, val string, v float64) {
		_ = labelName
		ch <- prometheus.MustNewConstMetric(c.descs[name], prometheus.GaugeValue, v, val)
	}
	counter := func(name string, v float64) {
		ch <- prometheus.MustNewConstMetric(c.descs[name], prometheus.CounterValue, v)
	}

	gauge("fuze_gpu_total", float64(snap.TotalGPUs))
	gauge("fuze_gpu_used", float64(snap.UsedGPUs))
	gauge("fuze_gpu_available", float64(snap.AvailableGPUs))
	gauge("fuze_gpu_utilization_percent", snap.GPUUtilization)
	gauge("fuze_memory_total_gb", float64(snap.TotalMemoryGB))
	gauge("fuze_memory_used_gb", float64(snap.UsedMemoryGB))
	gauge("fuze_memory_utilization_percent", snap.MemoryUtilization)

	labeledGauge("fuze_jobs", "status", "running", float64(snap.RunningJobs))
	labeledGauge("fuze_jobs", "status", "pending", float64(snap.PendingJobs))
	labeledGauge("fuze_jobs", "status", "completed", float64(snap.CompletedJobs))
	labeledGauge("fuze_jobs", "status", "total", float64(snap.TotalJobs))

	gauge("fuze_inference_services_total", float64(snap.InferenceTotal))
	gauge("fuze_inference_services_ready", float64(snap.InferenceReady))
	gauge("fuze_inference_ready_replicas", snap.InferenceReadyReplica)

	for _, d := range snap.Datasets {
		labeledGauge("fuze_dataset_cached_percent", "dataset", d.Name, d.CachedPercent)
	}

	counter("fuze_events_clusters_discovered", float64(snap.ClustersDiscovered))
	counter("fuze_events_gpus_discovered", float64(snap.GPUsDiscovered))
	counter("fuze_events_jobs_submitted", float64(snap.JobsSubmitted))
	counter("fuze_events_gpus_submitted", float64(snap.GPUsSubmitted))
	counter("fuze_events_assignments_completed", float64(snap.AssignmentsCompleted))
	counter("fuze_events_gpus_allocated", float64(snap.GPUsAllocated))

	gauge("fuze_workspaces_total", float64(snap.WorkspaceTotal))
	gauge("fuze_workspaces_running", float64(snap.WorkspaceRunning))
	for status, n := range snap.WorkspaceByStatus {
		labeledGauge("fuze_workspaces", "status", status, float64(n))
	}
}