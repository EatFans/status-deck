package systeminfo

import (
	"context"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Collector 负责采集当前电脑的基础资源状态。
//
// 当前由桌面端的 statussync 集中同步服务调用，并通过 BLE system.update 发送。
// 采集范围只包含：
// - 内存使用情况
// - 磁盘使用情况
// - 电源/电池使用情况
type Collector struct {
	// diskPath 指定要统计的磁盘挂载点。
	// macOS/Linux 默认 "/"，Windows 默认 "C:\"。
	diskPath string

	// ioreg 的 GPU 查询比内存/磁盘查询更重，因此独立缓存，默认每 5 秒刷新一次。
	lastGPU          GPU
	lastGPUCollected time.Time
}

// NewCollector 创建默认系统信息采集器。
func NewCollector() *Collector {
	return &Collector{
		diskPath: defaultDiskPath(),
	}
}

// Collect 采集一次系统快照。
//
// 内存和磁盘采集失败时会返回 error。
// 电源信息在某些平台不可用，所以 collectPower 会返回 Available=false，
// 不把它当作整个采集流程的致命错误。
func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	cpuUsage, err := collectCPU(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	memory, err := collectMemory(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	diskUsage, err := collectDisk(ctx, c.diskPath)
	if err != nil {
		return Snapshot{}, err
	}

	power := collectPower(ctx)
	gpu := c.collectGPU(ctx)

	return Snapshot{
		CollectedAt: time.Now(),
		CPU:         cpuUsage,
		GPU:         gpu,
		Memory:      memory,
		Disk:        diskUsage,
		Power:       power,
	}, nil
}

// collectCPU 读取所有逻辑核心合并后的使用率。interval=0 不会主动 sleep，
// gopsutil 使用相邻两次读取的差值计算百分比，适合 2 秒的同步周期。
func collectCPU(ctx context.Context) (CPU, error) {
	percents, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return CPU{}, err
	}
	if len(percents) == 0 {
		return CPU{}, nil
	}
	return CPU{UsagePercent: percents[0]}, nil
}

func (c *Collector) collectGPU(ctx context.Context) GPU {
	if time.Since(c.lastGPUCollected) < 5*time.Second {
		return c.lastGPU
	}

	c.lastGPU = collectGPU(ctx)
	c.lastGPUCollected = time.Now()
	return c.lastGPU
}

// collectMemory 采集物理内存使用情况。
//
// 字节字段保留原始值，方便后续 UI 根据屏幕空间选择 GB/MB 显示。
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

// collectDisk 采集指定挂载点的磁盘使用情况。
//
// 第一版只看系统盘。后续如果要展示多个磁盘，可以把 Disk 改成 []Disk。
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

// defaultDiskPath 返回当前平台默认系统盘路径。
func defaultDiskPath() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}

	return "/"
}
