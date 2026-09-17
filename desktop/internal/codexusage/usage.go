// Package codexusage 定义桌面端发送给状态卡的 Codex 用量数据边界。
//
// 本包目前只提供数据结构和不可用占位实现，不依赖 Codex 的本地文件、命令或
// 网络接口。以后确认可靠的数据来源后，只需要新增一个 Provider 实现，不必修改
// BLE 协议、状态同步调度或设备端存储逻辑。
package codexusage

import "context"

// Window 表示一个可独立重置的 Codex 用量窗口。
//
// RemainingPercent 的范围是 0 到 100。ResetLabel 直接保存适合显示屏使用的本地
// 文本，例如短窗口可为 "14:10"，周窗口可为 "9月19日"；这样固件无需处理时区和
// 日期格式化。后续若需要机器可读的重置时间，可在不破坏现有字段的前提下新增
// resetAtUnixMs。
type Window struct {
	RemainingPercent float64 `json:"remainingPercent"`
	ResetLabel       string  `json:"resetLabel"`
}

// Snapshot 是 status.update.payload.codex 的完整数据。
//
// 当前界面有两个窗口：fiveHour 对应截图中的“5小时”，weekly 对应“1周”。
// Available 为 false 时，两个窗口会省略，设备端应显示“数据暂不可用”而不是把它
// 误当成剩余 0%。
type Snapshot struct {
	Available bool    `json:"available"`
	FiveHour  *Window `json:"fiveHour,omitempty"`
	Weekly    *Window `json:"weekly,omitempty"`
}

// Provider 是 Codex 用量的可替换数据源。
//
// 采集过程可能涉及读取本地登录状态或请求服务端，因此带上 context 以便应用退出
// 时及时取消。返回的数据只描述用量，不处理 BLE 或设备 UI。
type Provider interface {
	Collect(ctx context.Context) (Snapshot, error)
}

// UnavailableProvider 是第一版的安全占位实现。
//
// 在尚未接入可信 Codex 数据来源时，它会显式告知设备端“不可用”。这能完整跑通
// 协议和渲染状态，同时避免把演示数据或截图数值当作用户的真实用量。
type UnavailableProvider struct{}

// Collect 返回没有可用用量数据的快照。
func (UnavailableProvider) Collect(context.Context) (Snapshot, error) {
	return Snapshot{Available: false}, nil
}
