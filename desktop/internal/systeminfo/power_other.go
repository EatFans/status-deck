//go:build !darwin

package systeminfo

import "context"

// collectPower 是非 macOS 平台的占位实现。
//
// Windows/Linux 后续可以分别接入：
// - Windows: WMI / WinRT battery API
// - Linux: upower / /sys/class/power_supply
func collectPower(ctx context.Context) Power {
	return Power{Available: false}
}
