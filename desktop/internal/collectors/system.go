package collectors

import (
	"context"
	"runtime"
	"time"
)

type SystemSnapshot struct {
	Hostname  string    `json:"hostname,omitempty"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Timestamp time.Time `json:"timestamp"`
}

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
