// Package app 负责组装桌面端的非 UI 运行时服务。
//
// 本包刻意不处理状态栏菜单、系统信息采集细节或 BLE 底层通信，而是将这些
// 独立模块按正确依赖关系组合起来。这样 tray 图形模式和 run 无界面调试模式
// 可以复用同一套桌面 Agent，避免随着功能增长出现两条行为不一致的运行链路。
package app

import (
	"context"

	"status-deck/desktop/internal/applog"
	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/codexusage"
	"status-deck/desktop/internal/config"
	"status-deck/desktop/internal/statussync"
	"status-deck/desktop/internal/systeminfo"
)

// Agent 是桌面端运行时的统一入口。
//
// 它持有 BLE 客户端与状态同步服务，负责启动自动重连和定时同步；展示层通过
// Events 接收连接事件，通过 ScanWithOptions 发起用户主动扫描。Agent 不持有
// 状态栏菜单对象，因此可以安全复用于 CLI、测试或未来的设置窗口。
type Agent struct {
	// client 负责与 ESP32 建立 BLE 连接、维护心跳，并提供消息发送能力。
	client *ble.Client
	// syncer 根据配置的同步计划，采集并推送系统状态与 Codex 用量。
	syncer *statussync.SystemService
	// config 保存运行时连接和同步策略；后续设置页修改配置后可据此重建 Agent。
	config config.Config
}

// New 使用生产环境的数据提供者创建 Agent。
//
// 当前会注册本机系统信息和本地 Codex 用量提供者。未来增加 Git、进程、Docker
// 等数据源时，应在这里完成依赖组装，并由 statussync 注册对应同步任务，而不是
// 直接让 UI 或数据源模块调用 BLE 写入。
func New(cfg config.Config) *Agent {
	client := ble.NewClient(cfg.BLE)
	return &Agent{
		client: client,
		syncer: statussync.NewSystemService(
			client,
			systeminfo.NewCollector(),
			codexusage.NewLocalProvider(),
			cfg.SyncPlan,
			applog.Printf,
		),
		config: cfg,
	}
}

// Start 启动后台运行循环。
//
// BLE 客户端会持续扫描并尝试连接设备；同步服务则按 SyncPlan 启动各数据域的
// 定时任务。两者均受 ctx 控制，调用方在应用退出时取消 ctx 即可结束后台工作。
func (a *Agent) Start(ctx context.Context) {
	a.client.Start(ctx, a.config.ReconnectInterval, a.config.HeartbeatInterval)
	a.syncer.Start(ctx)
}

// SyncNow 在设备刚连接成功后立即补齐一份初始状态。
//
// 定时任务本身仍会按照各自周期继续运行；状态同步服务会进行变化检测，避免这次
// 立即同步与下一次定时同步产生无意义的重复 BLE 写入。
func (a *Agent) SyncNow(ctx context.Context) { a.syncer.SyncNow(ctx) }

// Events 向展示层暴露 BLE 生命周期事件和设备主动通知。
//
// 托盘界面或 CLI 应尽快消费该通道，用它更新连接状态，并在收到连接成功事件后
// 调用 SyncNow。事件所有权仍属于 BLE 层，Agent 只负责向上转发。
func (a *Agent) Events() <-chan ble.Event { return a.client.Events() }

// IsConnected 返回当前是否仍与 ESP32 保持有效 BLE 连接。
func (a *Agent) IsConnected() bool { return a.client.IsConnected() }

// ScanWithOptions 执行一次由用户触发的前台设备扫描。
//
// 自动重连由 Start 内部维护；本方法主要供状态栏“扫描设备”菜单和 CLI scan
// 命令使用，底层会与自动扫描串行，避免同时占用系统蓝牙适配器。
func (a *Agent) ScanWithOptions(ctx context.Context, options ble.ScanOptions) ([]ble.Device, error) {
	return a.client.ScanWithOptions(ctx, options)
}

// Close 主动断开当前 BLE 连接，并停止客户端的后台连接维护循环。
//
// 调用方仍应先取消传入 Start 的 ctx，让同步任务先自然退出；Close 负责释放
// 最后的蓝牙资源，可安全在应用退出路径中调用。
func (a *Agent) Close() error { return a.client.Close() }
