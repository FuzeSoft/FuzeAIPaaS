package models

import "time"

type DatasetStatus string

const (
	DatasetStatusPending DatasetStatus = "pending"
	DatasetStatusBound   DatasetStatus = "bound" 
	DatasetStatusFailed  DatasetStatus = "failed"
	DatasetStatusUnknown DatasetStatus = "unknown"
)

type RuntimeType string

const (
	RuntimeAlluxio  RuntimeType = "alluxio"
	RuntimeJuiceFS  RuntimeType = "juicefs"
	RuntimeGooseFS  RuntimeType = "goosefs"
	RuntimeVineyard RuntimeType = "vineyard"
)

type CacheMedium string

const (
	CacheMediumMEM CacheMedium = "MEM" 
	CacheMediumSSD CacheMedium = "SSD"
	CacheMediumHDD CacheMedium = "HDD"
)

type Dataset struct {
	ID         string      `gorm:"primaryKey" json:"id"`
	TenantID   string      `gorm:"index" json:"tenant_id,omitempty"` 
	ClusterID  string      `gorm:"index" json:"cluster_id"`          
	Name       string      `json:"name"`
	MountPoint string      `json:"mount_point"` 
	SubPath    string      `json:"sub_path,omitempty"`
	Runtime    RuntimeType `json:"runtime"` 

	Replicas      int         `json:"replicas"`       
	CacheCapacity string      `json:"cache_capacity"` 
	CacheMedium   CacheMedium `json:"cache_medium"`   
	CachePath     string      `json:"cache_path,omitempty"`
	AccessMode    string      `json:"access_mode"` 

	Status         DatasetStatus `json:"status"`
	CachedPercent  float64       `json:"cached_percent"`      
	UFSTotal       string        `json:"ufs_total,omitempty"` 
	CachedCapacity string        `json:"cached_capacity,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func FluidPhaseToStatus(phase string) DatasetStatus {
	switch phase {
	case "Bound":
		return DatasetStatusBound
	case "Failed":
		return DatasetStatusFailed
	case "NotBound", "Pending", "":
		return DatasetStatusPending
	default:
		return DatasetStatusUnknown
	}
}