package systeminfo

import (
	"context"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type Collector struct {
	diskPath string
}

func NewCollector() *Collector {
	return &Collector{
		diskPath: defaultDiskPath(),
	}
}

func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	memory, err := collectMemory(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	diskUsage, err := collectDisk(ctx, c.diskPath)
	if err != nil {
		return Snapshot{}, err
	}

	power := collectPower(ctx)

	return Snapshot{
		CollectedAt: time.Now(),
		Memory:      memory,
		Disk:        diskUsage,
		Power:       power,
	}, nil
}

func collectMemory(ctx context.Context) (Memory, error) {
	usage, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return Memory{}, err
	}

	return Memory{
		TotalBytes:     usage.Total,
		UsedBytes:      usage.Used,
		AvailableBytes: usage.Available,
		UsedPercent:    usage.UsedPercent,
	}, nil
}

func collectDisk(ctx context.Context, path string) (Disk, error) {
	usage, err := disk.UsageWithContext(ctx, path)
	if err != nil {
		return Disk{}, err
	}

	return Disk{
		Path:        path,
		TotalBytes:  usage.Total,
		UsedBytes:   usage.Used,
		FreeBytes:   usage.Free,
		UsedPercent: usage.UsedPercent,
	}, nil
}

func defaultDiskPath() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}

	return "/"
}
