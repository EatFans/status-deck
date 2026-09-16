package config

import (
	"time"

	"status-deck/desktop/internal/ble"
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
	// 当前 run 模式会用到，状态栏模式后续也会复用。
	SyncInterval time.Duration
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
		SyncInterval: 2 * time.Second,
	}
}
