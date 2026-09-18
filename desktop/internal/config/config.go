package config

import (
	"time"

	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/statussync"
)

// Config 是桌面端的总配置。
//
// 同步周期由 statussync/sync_config.go 统一定义，并在打包时编译进程序。
type Config struct {
	// BLE 保存扫描、连接 Status Deck 硬件所需的设备名和 UUID。
	BLE ble.Config

	// SyncPlan 为 CPU/GPU、内存、磁盘电源和 Codex 分别设置同步频率与最长静默时间。
	// 默认值来自编译期配置文件；此字段仍保留，供测试或未来明确的设置功能覆盖。
	SyncPlan statussync.Plan

	// ReconnectInterval 控制设备不可用时的重新扫描频率。
	// 这不是 BLE 底层连接超时，而是桌面端主动重连策略的间隔。
	ReconnectInterval time.Duration

	// HeartbeatInterval 控制已连接后发送 heartbeat 的频率。
	// ESP32-S3 用它判断“BLE 虽然还挂着，但桌面应用是否还正常工作”。
	HeartbeatInterval time.Duration
}

// Default 返回开发阶段默认配置。
//
// 这里的 UUID 必须和固件 StatusBleServer 以及 docs/ble-protocol.md 保持一致。
func Default() Config {
	return Config{
		BLE: ble.Config{
			DeviceName:  "Status Deck",
			ServiceUUID: ble.DefaultServiceUUID,
			RXUUID:      ble.DefaultRXUUID,
			TXUUID:      ble.DefaultTXUUID,
		},
		SyncPlan:          statussync.DefaultPlan(),
		ReconnectInterval: 5 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}
