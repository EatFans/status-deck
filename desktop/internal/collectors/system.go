package collectors

import (
	"context"
	"runtime"
	"time"
)

// SystemSnapshot 是早期占位采集结果。
//
// 注意：新的系统资源采集已经拆到 internal/systeminfo，
// 这里后续会被替换或删除。
type SystemSnapshot struct {
	Hostname  string    `json:"hostname,omitempty"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Timestamp time.Time `json:"timestamp"`
}

// SystemCollector 是旧的占位采集器。
//
// 当前 run 模式仍然引用它，避免过早改动主流程；
// 后续接 BLE 发送时应改用 internal/systeminfo.Collector。
type SystemCollector struct{}

func NewSystemCollector() *SystemCollector {
	return &SystemCollector{}
}

func (c *SystemCollector) Collect(ctx context.Context) (SystemSnapshot, error) {
	return SystemSnapshot{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Timestamp: time.Now(),
	}, nil
}
