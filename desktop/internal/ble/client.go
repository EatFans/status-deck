package ble

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"status-deck/desktop/internal/pages"
	"tinygo.org/x/bluetooth"
)

const (
	// 这三个 UUID 必须和 ESP32-S3 固件里的 StatusBleServer 配置保持一致。
	DefaultServiceUUID = "7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultRXUUID      = "7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000"
	DefaultTXUUID      = "7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000"

	// macOS 当前连接上的 RX 特征值对较长的 Write With Response 会直接拒绝。
	// 值故意留出余量；真正的大消息通过 chunk 信封拆成更小的 UTF-8 片段发送。
	maxDirectWriteBytes = 480
	chunkDataBytes      = 128
)

// Config 是 BLE 客户端配置。UUID 分别对应设备的 GATT 服务、RX 和 TX 特征值。
type Config struct {
	DeviceName  string
	ServiceUUID string
	RXUUID      string
	TXUUID      string
}

// Device 是设备管理菜单或开发命令展示的轻量扫描结果。
type Device struct {
	ID   string
	Name string
	RSSI int
}

// ScanOptions 控制一次扫描的范围。
type ScanOptions struct {
	Timeout        time.Duration
	IncludeAll     bool
	IncludeUnnamed bool
}

// EventType 表示连接层发生的事件。UI 只需要订阅 Events，就不必直接碰 BLE 对象。
type EventType string

const (
	EventConnecting    EventType = "connecting"
	EventConnected     EventType = "connected"
	EventDisconnected  EventType = "disconnected"
	EventNotification  EventType = "notification"
	EventPageChanged   EventType = "page_changed"
	EventConnectFailed EventType = "connect_failed"
)

// Event 是 BLE 层向上层报告的状态或设备主动通知。
type Event struct {
	Type    EventType
	Device  Device
	Message []byte
	Page    pages.Status
	Err     error
}

// Client 把 BLE Central 的扫描、连接、通知订阅和自动重连收在一起。
//
// ESP32 是 Peripheral，无法反过来主动连接电脑；因此桌面端启动后必须由本类
// 持续扫描、主动连接。Start 会启动该维护循环，直到传入的 ctx 被取消或 Close 被调用。
type Client struct {
	config  Config
	adapter *bluetooth.Adapter

	// scanMu 让手动扫描和自动重连扫描串行，避免同一蓝牙适配器并发扫描。
	scanMu sync.Mutex
	// writeMu 保证任何时刻只有一条 GATT 写入在飞行中。心跳、首次 hello 和
	// 后续各类业务同步都经由同一个 Client，因此不能让不同 goroutine 并发写 RX。
	writeMu sync.Mutex
	mu      sync.RWMutex

	addresses map[string]bluetooth.Address
	device    *bluetooth.Device
	rx        *bluetooth.DeviceCharacteristic
	tx        *bluetooth.DeviceCharacteristic
	current   Device
	page      pages.Status

	events chan Event
	closed chan struct{}
	once   sync.Once
}

func NewClient(config Config) *Client {
	c := &Client{
		config:    config,
		adapter:   bluetooth.DefaultAdapter,
		addresses: make(map[string]bluetooth.Address),
		events:    make(chan Event, 32),
		closed:    make(chan struct{}),
	}

	// tinygo bluetooth 在支持的平台会在底层连接状态变化时回调。心跳循环也会
	// 再检查一次 Connected，作为跨平台的兜底检测。
	c.adapter.SetConnectHandler(c.handleConnectionChange)
	return c
}

// Events 返回只读事件流。消费者必须尽快处理事件，避免 UI 卡住 BLE 回调。
func (c *Client) Events() <-chan Event { return c.events }

func (c *Client) Scan(ctx context.Context, timeout time.Duration) ([]Device, error) {
	return c.ScanWithOptions(ctx, ScanOptions{Timeout: timeout})
}

// ScanWithOptions 扫描附近设备，并记住本次扫描得到的 BLE 地址供 Connect 使用。
func (c *Client) ScanWithOptions(ctx context.Context, options ScanOptions) ([]Device, error) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()

	if err := enableAdapter(c.adapter); err != nil {
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
		errCh <- c.adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
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
			device := Device{ID: id, Name: name, RSSI: int(result.RSSI)}
			devices = append(devices, device)

			c.mu.Lock()
			c.addresses[id] = result.Address
			c.mu.Unlock()
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

// Connect 扫描并连接第一台 Status Deck。已经连接时直接返回，不会重复建链。
func (c *Client) Connect(ctx context.Context) error {
	if c.IsConnected() {
		return nil
	}
	c.emit(Event{Type: EventConnecting})

	devices, err := c.Scan(ctx, 5*time.Second)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return fmt.Errorf("no Status Deck devices found")
	}

	target := devices[0]
	c.mu.RLock()
	address, ok := c.addresses[target.ID]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("BLE address for %s was not retained", target.ID)
	}

	device, err := c.adapter.Connect(address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("connect to %s: %w", target.ID, err)
	}

	if err := c.discover(device, target); err != nil {
		_ = device.Disconnect()
		return err
	}

	// 先报告连接建立，确保随后 hello 触发的 page.status 在事件队列中排在其后。
	// 否则快速设备可能先回 page.status，再被 EventConnected 的 UI 文案覆盖。
	c.emit(Event{Type: EventConnected, Device: target})

	// 协议规定：订阅通知后，桌面端先主动发送 hello，设备据此确认会话已经就绪。
	if err := c.Write(ctx, newEnvelope("hello", map[string]any{
		"client": "status-deck", "protocol": 1,
		"features": []string{"json", "ack", "status", "heartbeat", "pages"},
	})); err != nil {
		_ = device.Disconnect()
		c.clearConnection(target, false)
		return fmt.Errorf("send hello: %w", err)
	}

	return nil
}

// Start 启动自动连接与保活循环。它可安全重复调用，只有第一次会生效。
// 连接失败不会退出：ESP32 未上电、超出范围或暂时断开都是正常场景，下一轮会重试。
func (c *Client) Start(ctx context.Context, reconnectInterval, heartbeatInterval time.Duration) {
	if reconnectInterval <= 0 {
		reconnectInterval = 5 * time.Second
	}
	if heartbeatInterval <= 0 {
		heartbeatInterval = 10 * time.Second
	}
	c.once.Do(func() {
		go c.maintain(ctx, reconnectInterval, heartbeatInterval)
	})
}

func (c *Client) maintain(ctx context.Context, reconnectInterval, heartbeatInterval time.Duration) {
	retry := time.NewTimer(0)
	defer retry.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = c.Close()
			return
		case <-c.closed:
			return
		case <-retry.C:
			if !c.IsConnected() {
				attemptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				err := c.Connect(attemptCtx)
				cancel()
				if err != nil {
					c.emit(Event{Type: EventConnectFailed, Err: err})
				}
			}
			retry.Reset(reconnectInterval)
		case <-heartbeat.C:
			if !c.IsConnected() {
				continue
			}
			if err := c.Write(ctx, newEnvelope("heartbeat", map[string]any{"interval": heartbeatInterval.Milliseconds()})); err != nil {
				c.markDisconnected(err)
			}
		}
	}
}

func (c *Client) discover(device bluetooth.Device, target Device) error {
	serviceUUID, err := bluetooth.ParseUUID(c.config.ServiceUUID)
	if err != nil {
		return fmt.Errorf("parse service UUID: %w", err)
	}
	rxUUID, err := bluetooth.ParseUUID(c.config.RXUUID)
	if err != nil {
		return fmt.Errorf("parse RX UUID: %w", err)
	}
	txUUID, err := bluetooth.ParseUUID(c.config.TXUUID)
	if err != nil {
		return fmt.Errorf("parse TX UUID: %w", err)
	}

	services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})
	if err != nil || len(services) != 1 {
		return fmt.Errorf("discover Status Deck service: %w", err)
	}
	characteristics, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{rxUUID, txUUID})
	if err != nil || len(characteristics) != 2 {
		return fmt.Errorf("discover Status Deck characteristics: %w", err)
	}

	c.mu.Lock()
	c.device = &device
	c.rx = &characteristics[0]
	c.tx = &characteristics[1]
	c.current = target
	c.mu.Unlock()

	// EnableNotifications 会写入 CCCD；此后设备端调用 notify 时，桌面端会立刻收到。
	if err := characteristics[1].EnableNotifications(func(message []byte) {
		copyMessage := append([]byte(nil), message...)
		if page, ok := parsePageStatus(copyMessage); ok {
			c.mu.Lock()
			c.page = page
			c.mu.Unlock()
			c.emit(Event{Type: EventPageChanged, Device: target, Page: page})
			return
		}
		c.emit(Event{Type: EventNotification, Device: target, Message: copyMessage})
	}); err != nil {
		c.clearConnection(target, false)
		return fmt.Errorf("subscribe TX notifications: %w", err)
	}
	return nil
}

// Write 通过 RX 特征值写入一条 JSON 消息。
//
// 小消息直接写入；超过单次安全长度的大消息会自动封装为多个 chunk。每个 chunk
// 都采用带响应写入并由 writeMu 串行化，设备端按 index 重组后再解析原始 JSON。
// 这同时规避 macOS 的单次 GATT 写入上限和 Write Without Response 的缓冲拥塞。
func (c *Client) Write(_ context.Context, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.RLock()
	rx := c.rx
	c.mu.RUnlock()
	if rx == nil || !c.IsConnected() {
		return fmt.Errorf("Status Deck is not connected")
	}
	if len(payload) <= maxDirectWriteBytes {
		return c.writeRaw(rx, payload)
	}
	return c.writeChunked(rx, payload)
}

func (c *Client) writeRaw(rx *bluetooth.DeviceCharacteristic, payload []byte) error {
	if _, err := rx.Write(payload); err != nil {
		return fmt.Errorf("write RX characteristic: %w", err)
	}
	return nil
}

// writeChunked 将一个完整 UTF-8 JSON 信封拆成多个独立 JSON chunk 信封。
//
// data 直接是原 JSON 的字符串片段，因此必须在 UTF-8 字符边界分割；否则像
// "9月19日" 这样的中文重置标签可能在 json.Marshal 时被替换，导致设备端无法
// 重组原始消息。分片大小保守设置为 128 bytes，连同 chunk 信封也远低于当前
// GATT 写入上限。
func (c *Client) writeChunked(rx *bluetooth.DeviceCharacteristic, payload []byte) error {
	parts := splitUTF8(payload, chunkDataBytes)
	if len(parts) == 0 {
		return errors.New("cannot split empty BLE payload")
	}

	var original struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &original); err != nil || original.ID == "" {
		return fmt.Errorf("decode original BLE message for chunking: %w", err)
	}

	for index, part := range parts {
		chunk := struct {
			Version int    `json:"v"`
			ID      string `json:"id"`
			Type    string `json:"type"`
			TS      int64  `json:"ts"`
			Source  string `json:"source"`
			Target  string `json:"target"`
			Payload struct {
				Ref      string `json:"ref"`
				Index    int    `json:"index"`
				Total    int    `json:"total"`
				Encoding string `json:"encoding"`
				Data     string `json:"data"`
			} `json:"payload"`
		}{
			Version: 1,
			ID:      fmt.Sprintf("%s_chunk_%d", original.ID, index),
			Type:    "chunk",
			TS:      time.Now().UnixMilli(),
			Source:  "desktop",
			Target:  "device",
		}
		chunk.Payload.Ref = original.ID
		chunk.Payload.Index = index
		chunk.Payload.Total = len(parts)
		chunk.Payload.Encoding = "json"
		chunk.Payload.Data = string(part)

		encoded, err := json.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("encode BLE chunk %d: %w", index, err)
		}
		if err := c.writeRaw(rx, encoded); err != nil {
			return fmt.Errorf("write BLE chunk %d/%d: %w", index+1, len(parts), err)
		}
	}
	return nil
}

// splitUTF8 返回可独立转换为 string 的字节片段。payload 来自 json.Marshal，
// 因而本身是有效 UTF-8；只需避免切在一个多字节 rune 的中间。
func splitUTF8(payload []byte, size int) [][]byte {
	if size <= 0 || len(payload) == 0 {
		return nil
	}
	parts := make([][]byte, 0, (len(payload)+size-1)/size)
	for start := 0; start < len(payload); {
		end := start + size
		if end >= len(payload) {
			parts = append(parts, payload[start:])
			break
		}
		for end > start && payload[end]&0xc0 == 0x80 {
			end--
		}
		if end == start {
			// 当前 size 比一个 rune 还短时宁可原样切分。本项目的 128 bytes
			// 远大于 UTF-8 最大 rune 长度，此分支只是避免未来配置误用死循环。
			end = start + size
		}
		parts = append(parts, payload[start:end])
		start = end
	}
	return parts
}

// Send 用统一的协议信封发送一条业务消息。
//
// 上层同步服务不应该自行拼装 v/id/ts/source/target，这样未来新增 AI 用量、
// API 额度或服务器状态时，仍然会使用同一份版本与追踪规则。
func (c *Client) Send(ctx context.Context, messageType string, payload any) error {
	return c.Write(ctx, newEnvelope(messageType, payload))
}

// IsConnected 同时检查本地连接资源和底层链路状态。
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	device := c.device
	c.mu.RUnlock()
	if device == nil {
		return false
	}
	connected, err := device.Connected()
	return err == nil && connected
}

// ActivePage returns the last page.status reported by the connected device.
// A zero index means the connection is ready but the hello response has not
// arrived yet, so callers must not send page-specific payloads.
func (c *Client) ActivePage() pages.Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.page
}

func (c *Client) handleConnectionChange(device bluetooth.Device, connected bool) {
	if connected {
		return
	}
	c.mu.RLock()
	current := c.current
	known := c.device != nil && c.device.Address.String() == device.Address.String()
	c.mu.RUnlock()
	if known {
		c.clearConnection(current, true)
	}
}

func (c *Client) markDisconnected(err error) {
	c.mu.RLock()
	current := c.current
	c.mu.RUnlock()
	c.clearConnection(current, true)
	c.emit(Event{Type: EventConnectFailed, Device: current, Err: err})
}

func (c *Client) clearConnection(device Device, notify bool) {
	c.mu.Lock()
	c.device = nil
	c.rx = nil
	c.tx = nil
	c.current = Device{}
	c.page = pages.Status{}
	c.mu.Unlock()
	if notify {
		c.emit(Event{Type: EventDisconnected, Device: device})
	}
}

func (c *Client) emit(event Event) {
	select {
	case c.events <- event:
	default:
		// UI 短暂卡顿不能反过来阻塞 CoreBluetooth 回调；丢弃的是非关键展示事件。
	}
}

// Close 断开当前设备，并让后台自动连接循环停止。
func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}

	c.mu.Lock()
	device := c.device
	current := c.current
	c.device = nil
	c.rx = nil
	c.tx = nil
	c.current = Device{}
	c.page = pages.Status{}
	c.mu.Unlock()
	if device == nil {
		return nil
	}
	err := device.Disconnect()
	c.emit(Event{Type: EventDisconnected, Device: current})
	return err
}

func parsePageStatus(message []byte) (pages.Status, bool) {
	var notification struct {
		Type    string `json:"type"`
		Payload struct {
			Index int    `json:"index"`
			ID    string `json:"id"`
			Count int    `json:"count"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(message, &notification); err != nil || notification.Type != "page.status" {
		return pages.Status{}, false
	}
	page := pages.Status{Index: notification.Payload.Index, ID: notification.Payload.ID, Count: notification.Payload.Count}
	return page, page.Known() && page.Count >= page.Index
}

func newEnvelope(messageType string, payload any) []byte {
	message := struct {
		Version int    `json:"v"`
		ID      string `json:"id"`
		Type    string `json:"type"`
		TS      int64  `json:"ts"`
		Source  string `json:"source"`
		Target  string `json:"target"`
		Payload any    `json:"payload"`
	}{
		Version: 1,
		ID:      fmt.Sprintf("desktop_%d", time.Now().UnixNano()),
		Type:    messageType,
		TS:      time.Now().UnixMilli(),
		Source:  "desktop",
		Target:  "device",
		Payload: payload,
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return []byte(`{}`)
	}
	return encoded
}
