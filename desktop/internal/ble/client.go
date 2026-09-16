package ble

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	// 这三个 UUID 必须和 ESP32-S3 固件里的 StatusBleServer 配置保持一致。
	// 桌面端靠 Service UUID 找设备，靠 RX/TX UUID 和设备交换数据。
	DefaultServiceUUID = "7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultRXUUID      = "7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultTXUUID      = "7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000"
)

// Config 是 BLE 客户端配置。
//
// DeviceName 用作兼容性兜底：有些系统扫描时不一定能拿到 Service UUID，
// 这时可以退回按设备名匹配。
//
// ServiceUUID/RXUUID/TXUUID 分别对应协议文档里的 GATT 服务和特征值。
type Config struct {
	DeviceName  string
	ServiceUUID string
	RXUUID      string
	TXUUID      string
}

// Device 是扫描结果里展示给设备管理菜单或 CLI 的轻量设备信息。
type Device struct {
	ID   string
	Name string
	RSSI int
}

// ScanOptions 控制扫描行为。
//
// IncludeAll 用于调试：显示附近所有 BLE 设备，而不是只显示 Status Deck。
// IncludeUnnamed 用于更深一层排查：很多 BLE 广播没有设备名，默认隐藏掉。
type ScanOptions struct {
	Timeout        time.Duration
	IncludeAll     bool
	IncludeUnnamed bool
}

// Client 封装桌面端 BLE Central 能力。
//
// 当前已经实现扫描；连接、订阅 TX notify、写 RX 还在后续补齐。
type Client struct {
	config  Config
	adapter *bluetooth.Adapter
}

func NewClient(config Config) *Client {
	return &Client{
		config:  config,
		adapter: bluetooth.DefaultAdapter,
	}
}

func (c *Client) Scan(ctx context.Context, timeout time.Duration) ([]Device, error) {
	// 保留一个简单入口，常规扫描只需要传 timeout。
	return c.ScanWithOptions(ctx, ScanOptions{Timeout: timeout})
}

func (c *Client) ScanWithOptions(ctx context.Context, options ScanOptions) ([]Device, error) {
	// 启用本机蓝牙适配器。macOS 第一次运行时可能会弹权限确认。
	if err := c.adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable BLE adapter: %w", err)
	}

	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Second
	}

	// 把字符串 UUID 转成 tinygo bluetooth 使用的 UUID 类型。
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
		// tinygo bluetooth 的 Scan 是阻塞式调用，所以放到 goroutine 里运行。
		// 超时后外层会调用 StopScan 停止扫描。
		errCh <- c.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			name := result.LocalName()
			matchesService := result.HasServiceUUID(serviceUUID)
			matchesName := c.config.DeviceName != "" && name == c.config.DeviceName

			// 默认只保留 Status Deck 设备；--all 调试模式下才显示其他设备。
			if !options.IncludeAll && !matchesService && !matchesName {
				return
			}

			// 附近会有很多匿名 BLE 广播，默认隐藏，避免设备管理列表被刷屏。
			if options.IncludeAll && !options.IncludeUnnamed && name == "" {
				return
			}

			id := result.Address.String()

			mu.Lock()
			defer mu.Unlock()

			// 同一个设备可能在扫描期间多次广播，这里只保留第一次结果。
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
		// 扫描超时或用户取消时，主动停止 BLE 扫描并返回已发现的设备。
		if err := c.adapter.StopScan(); err != nil {
			return nil, fmt.Errorf("stop BLE scan: %w", err)
		}

		mu.Lock()
		defer mu.Unlock()
		return append([]Device(nil), devices...), nil
	case err := <-errCh:
		// 如果底层扫描提前退出，说明蓝牙栈或权限可能出了问题。
		if err != nil {
			return nil, fmt.Errorf("scan BLE devices: %w", err)
		}
		return nil, nil
	}
}

func (c *Client) Connect(ctx context.Context) error {
	// TODO: 实现：
	// 1. 扫描 Status Deck Service UUID
	// 2. 连接设备
	// 3. 发现 RX/TX 特征值
	// 4. 订阅 TX notify
	// 5. 保存设备 ID 供下次自动重连
	return fmt.Errorf("BLE connect is not implemented yet")
}

func (c *Client) Write(ctx context.Context, payload []byte) error {
	// TODO: 连接完成后，把 JSON 协议消息写入固件侧 RX 特征值。
	return fmt.Errorf("BLE write is not implemented yet")
}

func (c *Client) Close() error {
	// TODO: 连接实现后，这里负责取消扫描、断开连接、释放平台资源。
	return nil
}
