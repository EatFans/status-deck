package ble

import (
	"context"
	"errors"
	"time"
)

const (
	DefaultServiceUUID = "7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultRXUUID      = "7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultTXUUID      = "7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000"
)

var ErrNotImplemented = errors.New("BLE backend is not implemented yet")

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

type Client struct {
	config Config
}

func NewClient(config Config) *Client {
	return &Client{config: config}
}

func (c *Client) Scan(ctx context.Context, timeout time.Duration) ([]Device, error) {
	return nil, ErrNotImplemented
}

func (c *Client) Connect(ctx context.Context) error {
	return ErrNotImplemented
}

func (c *Client) Write(ctx context.Context, payload []byte) error {
	return ErrNotImplemented
}

func (c *Client) Close() error {
	return nil
}
