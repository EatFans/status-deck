package ble

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	DefaultServiceUUID = "7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultRXUUID      = "7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultTXUUID      = "7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000"
)

type Config struct {
	DeviceName  string
	ServiceUUID string
	RXUUID      string
	TXUUID      string
}

type Device struct {
	ID   string
	Name string
	RSSI int
}

type ScanOptions struct {
	Timeout       time.Duration
	IncludeAll    bool
	IncludeUnnamed bool
}

type Client struct {
	config Config
	adapter *bluetooth.Adapter
}

func NewClient(config Config) *Client {
	return &Client{
		config:  config,
		adapter: bluetooth.DefaultAdapter,
	}
}

func (c *Client) Scan(ctx context.Context, timeout time.Duration) ([]Device, error) {
	return c.ScanWithOptions(ctx, ScanOptions{Timeout: timeout})
}

func (c *Client) ScanWithOptions(ctx context.Context, options ScanOptions) ([]Device, error) {
	if err := c.adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable BLE adapter: %w", err)
	}

	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Second
	}

	serviceUUID, err := bluetooth.ParseUUID(c.config.ServiceUUID)
	if err != nil {
		return nil, fmt.Errorf("parse service UUID: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	var (
		mu      sync.Mutex
		devices []Device
		seen    = map[string]bool{}
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			name := result.LocalName()
			matchesService := result.HasServiceUUID(serviceUUID)
			matchesName := c.config.DeviceName != "" && name == c.config.DeviceName

			if !options.IncludeAll && !matchesService && !matchesName {
				return
			}

			if options.IncludeAll && !options.IncludeUnnamed && name == "" {
				return
			}

			id := result.Address.String()

			mu.Lock()
			defer mu.Unlock()

			if seen[id] {
				return
			}

			seen[id] = true
			devices = append(devices, Device{
				ID:   id,
				Name: name,
				RSSI: int(result.RSSI),
			})
		})
	}()

	select {
	case <-ctx.Done():
		if err := c.adapter.StopScan(); err != nil {
			return nil, fmt.Errorf("stop BLE scan: %w", err)
		}

		mu.Lock()
		defer mu.Unlock()
		return append([]Device(nil), devices...), nil
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("scan BLE devices: %w", err)
		}
		return nil, nil
	}
}

func (c *Client) Connect(ctx context.Context) error {
	return fmt.Errorf("BLE connect is not implemented yet")
}

func (c *Client) Write(ctx context.Context, payload []byte) error {
	return fmt.Errorf("BLE write is not implemented yet")
}

func (c *Client) Close() error {
	return nil
}
