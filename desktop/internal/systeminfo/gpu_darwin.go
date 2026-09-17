//go:build darwin

package systeminfo

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
)

// Apple Silicon 的 IOAccelerator PerformanceStatistics 包含设备总体忙碌率。
// 使用正则是因为 ioreg 的人类可读输出在不同 macOS 小版本间字段嵌套会变化，
// 但该键名长期保持稳定。取第一个匹配项即当前主要集成 GPU。
var gpuUtilizationPattern = regexp.MustCompile(`"Device Utilization %"\s*=\s*(\d+(?:\.\d+)?)`)

func collectGPU(ctx context.Context) GPU {
	output, err := exec.CommandContext(ctx, "ioreg", "-r", "-d", "1", "-c", "IOAccelerator", "-w", "0").Output()
	if err != nil {
		return GPU{Available: false}
	}

	matches := gpuUtilizationPattern.FindStringSubmatch(string(output))
	if len(matches) != 2 {
		return GPU{Available: false}
	}
	usage, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return GPU{Available: false}
	}
	return GPU{Available: true, UsagePercent: usage, Source: "ioreg"}
}
