package config

import (
	"time"

	"status-deck/desktop/internal/ble"
)

type Config struct {
	BLE          ble.Config
	SyncInterval time.Duration
}

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
