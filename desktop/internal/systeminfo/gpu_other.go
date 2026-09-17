//go:build !darwin

package systeminfo

import "context"

// 非 macOS 平台暂不猜测 GPU 使用率。后续可分别接入 Windows PDH/NVAPI、
// Linux DRM 或 vendor 工具；协议层已有 Available 字段，不需要改变 BLE 格式。
func collectGPU(context.Context) GPU { return GPU{Available: false} }
