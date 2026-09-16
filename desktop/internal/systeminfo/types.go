package systeminfo

import "time"

// Snapshot 是一次系统信息采集结果。
//
// 这里刻意不包含系统名称、OS、架构等信息；
// 当前只服务状态卡展示需要的资源状态。
type Snapshot struct {
	CollectedAt time.Time `json:"collectedAt"`
	Memory      Memory    `json:"memory"`
	Disk        Disk      `json:"disk"`
	Power       Power     `json:"power"`
}

// Memory 表示内存使用情况。
//
// 所有容量字段都用 bytes，避免在采集层提前决定显示单位。
type Memory struct {
	TotalBytes     uint64  `json:"totalBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsedPercent    float64 `json:"usedPercent"`
}

// Disk 表示一个磁盘挂载点的使用情况。
//
// 第一版只采集系统盘；后续可以扩展为多个 Disk。
type Disk struct {
	Path        string  `json:"path"`
	TotalBytes  uint64  `json:"totalBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

// Power 表示电源/电池状态。
//
// Percent/Charging/OnBattery 用指针是为了区分：
// - 字段不可用：nil
// - 字段可用且值为 false/0：非 nil
//
// 例如台式机可能没有电池百分比，但仍可以显示 Available=false。
type Power struct {
	Available bool   `json:"available"`
	Percent   *int   `json:"percent,omitempty"`
	Charging  *bool  `json:"charging,omitempty"`
	OnBattery *bool  `json:"onBattery,omitempty"`
	State     string `json:"state,omitempty"`
	Source    string `json:"source,omitempty"`
}
