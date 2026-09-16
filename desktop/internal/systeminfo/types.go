package systeminfo

import "time"

type Snapshot struct {
	CollectedAt time.Time `json:"collectedAt"`
	Memory      Memory    `json:"memory"`
	Disk        Disk      `json:"disk"`
	Power       Power     `json:"power"`
}

type Memory struct {
	TotalBytes     uint64  `json:"totalBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	UsedPercent    float64 `json:"usedPercent"`
}

type Disk struct {
	Path        string  `json:"path"`
	TotalBytes  uint64  `json:"totalBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type Power struct {
	Available bool    `json:"available"`
	Percent   *int    `json:"percent,omitempty"`
	Charging  *bool   `json:"charging,omitempty"`
	OnBattery *bool   `json:"onBattery,omitempty"`
	State     string  `json:"state,omitempty"`
	Source    string  `json:"source,omitempty"`
}
