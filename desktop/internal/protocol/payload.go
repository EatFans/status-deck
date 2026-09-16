package protocol

import (
	"encoding/json"
	"time"

	"status-deck/desktop/internal/collectors"
)

// StatusPayload 是早期占位协议结构。
//
// 注意：docs/ble-protocol.md 已经定义了更通用的消息信封：
// { v, id, type, ts, source, target, payload }
//
// 这个结构后续会被替换或调整为协议文档里的正式格式。
type StatusPayload struct {
	Type      string                    `json:"type"`
	Version   int                       `json:"version"`
	Timestamp time.Time                 `json:"timestamp"`
	System    collectors.SystemSnapshot `json:"system"`
}

// NewStatusPayload 根据采集到的系统状态创建一条待发送消息。
//
// 当前使用旧 collectors.SystemSnapshot，新的内存/磁盘/电源采集在
// internal/systeminfo 中，暂时还没有接到 BLE 发送链路里。
func NewStatusPayload(system collectors.SystemSnapshot) StatusPayload {
	return StatusPayload{
		Type:      "status",
		Version:   1,
		Timestamp: time.Now(),
		System:    system,
	}
}

// Encode 把协议结构编码成 JSON 字节，供 BLE RX 写入。
func Encode(payload StatusPayload) ([]byte, error) {
	return json.Marshal(payload)
}
