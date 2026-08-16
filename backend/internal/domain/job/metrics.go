package job

import "math"

type Metrics struct {
	ClusterID         string
	TotalGPUs         int
	UsedGPUs          int
	AvailableGPUs     int
	GPUUtilization    float64
	TotalJobs         int
	RunningJobs       int
	PendingJobs       int
	CompletedJobs     int
	TotalMemory       int
	UsedMemory        int
	MemoryUtilization float64
}

func ComputeMetrics(resources []Resource, jobs []Job, clusterID string) *Metrics {
	m := &Metrics{ClusterID: clusterID}
	for _, res := range resources {
		if res.Type == ResourceTypeGPU {
			m.TotalGPUs += res.GPUCount()
			m.UsedGPUs += res.GPUAllocated()
		}
		if res.Type == ResourceTypeGPU || res.Type == ResourceTypeNPU {
			m.TotalMemory += res.TotalMemory
			m.UsedMemory += res.TotalMemory - res.AvailableMemory
		}
	}
	m.AvailableGPUs = m.TotalGPUs - m.UsedGPUs
	if m.AvailableGPUs < 0 {
		m.AvailableGPUs = 0
	}
	if m.TotalGPUs > 0 {
		m.GPUUtilization = math.Round(float64(m.UsedGPUs)/float64(m.TotalGPUs)*1000) / 10
	}
	if m.TotalMemory > 0 {
		m.MemoryUtilization = math.Round(float64(m.UsedMemory)/float64(m.TotalMemory)*1000) / 10
	}
	for _, job := range jobs {
		if job.ClusterID != clusterID {
			continue
		}
		m.TotalJobs++
		switch job.Status {
		case JobStatusRunning:
			m.RunningJobs++
		case JobStatusPending:
			m.PendingJobs++
		case JobStatusCompleted:
			m.CompletedJobs++
		}
	}
	return m
}