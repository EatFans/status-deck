package config

import (
	"time"

	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/statussync"
)

// Config 是桌面端的总配置。
//
// 现在先用代码里的默认值，后续可以扩展为读取配置文件：
// - macOS: ~/Library/Application Support/Status Deck/config.json
// - Windows: %APPDATA%/Status Deck/config.json
type Config struct {
	// BLE 保存扫描、连接 Status Deck 硬件所需的设备名和 UUID。
	BLE ble.Config

	// SyncInterval 是向硬件同步状态数据的间隔。
	// 当前仅旧 run 调试命令使用；状态栏应用使用下面按数据域配置的 SyncPlan。
	SyncInterval time.Duration

	// SyncPlan 为 CPU/GPU、内存、磁盘电源和 Codex 分别设置同步频率与最长静默时间。
	// 后续设置页面可直接读写这些字段，而不需要修改同步器实现。
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
		SyncInterval:      2 * time.Second,
		SyncPlan:          statussync.DefaultPlan(),
		ReconnectInterval: 5 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}
