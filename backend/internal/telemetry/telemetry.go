
package telemetry

import (
	"sync/atomic"

	domainevent "fuze-ai-paas/backend/internal/domain/event"
)

type Snapshot struct {
	ClustersDiscovered int64
	GPUsDiscovered     int64
	JobsSubmitted      int64
	GPUsSubmitted      int64
	AssignmentsDone    int64
	GPUsAllocated      int64
}

type Collector struct {
	clustersDiscovered int64
	gpusDiscovered     int64
	jobsSubmitted      int64
	gpusSubmitted      int64
	assignmentsDone    int64
	gpusAllocated      int64
}

func NewCollector() *Collector { return &Collector{} }

func (c *Collector) RecordCluster(e domainevent.ClusterDiscovered) {
	atomic.AddInt64(&c.clustersDiscovered, 1)
	atomic.AddInt64(&c.gpusDiscovered, int64(e.TotalGPUs))
}

func (c *Collector) RecordSubmit(e domainevent.JobSubmitted) {
	atomic.AddInt64(&c.jobsSubmitted, 1)
	atomic.AddInt64(&c.gpusSubmitted, int64(e.GPUs))
}

func (c *Collector) RecordAssign(e domainevent.AssignmentCompleted) {
	atomic.AddInt64(&c.assignmentsDone, 1)
	atomic.AddInt64(&c.gpusAllocated, int64(e.AllocatedGPUs))
}

func (c *Collector) Snapshot() Snapshot {
	return Snapshot{
		ClustersDiscovered: atomic.LoadInt64(&c.clustersDiscovered),
		GPUsDiscovered:     atomic.LoadInt64(&c.gpusDiscovered),
		JobsSubmitted:      atomic.LoadInt64(&c.jobsSubmitted),
		GPUsSubmitted:      atomic.LoadInt64(&c.gpusSubmitted),
		AssignmentsDone:    atomic.LoadInt64(&c.assignmentsDone),
		GPUsAllocated:      atomic.LoadInt64(&c.gpusAllocated),
	}
}