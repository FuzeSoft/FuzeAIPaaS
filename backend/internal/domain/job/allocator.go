
package job

func CanSchedule(job *Job, resources []Resource) bool {
	availableMemory := 0
	for _, res := range resources {
		if res.IsAvailable() {
			availableMemory += res.AvailableMemory
		}
	}
	return availableMemory >= job.Memory
}

func Allocate(job *Job, resources []Resource) {
	remainingMemory := job.Memory
	for i := range resources {
		if remainingMemory <= 0 {
			break
		}
		remainingMemory -= resources[i].Allocate(remainingMemory)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}