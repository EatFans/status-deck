// Package statussync 负责把多个本地数据源汇总为对硬件的状态同步。
//
// 采集器只负责“拿到数据”，本包负责“何时发送、以什么协议发送”。这样以后增加
// AI 用量或服务器健康检查时，不会让每个采集器直接竞争 BLE 写入权限。
package statussync

import (
	"context"
	"time"

	"status-deck/desktop/internal/codexusage"
	"status-deck/desktop/internal/systeminfo"
)

// Publisher 是同步服务依赖的最小 BLE 能力。
// ble.Client 满足该接口；保留接口能让该包在不依赖真实蓝牙硬件时单独测试。
type Publisher interface {
	IsConnected() bool
	Send(ctx context.Context, messageType string, payload any) error
}

// SystemPayload 是 system.update 的 payload。
//
// 这里不直接序列化 systeminfo.Snapshot：CollectedAt 已由协议信封 ts 表达，
// 避免每条 BLE 消息携带重复时间字段；系统名称也按项目约定不发送。
type SystemPayload struct {
	CPU    CPUWire      `json:"cpu"`
	GPU    GPUWire      `json:"gpu"`
	Memory CapacityWire `json:"memory"`
	Disk   CapacityWire `json:"disk"`
	Power  PowerWire    `json:"power"`
}

// CapacityWire 以 MB 传输容量。屏幕展示不需要 bytes 级精度，使用 MB 可以显著
// 缩小 BLE 包；固件收到后会换算回 bytes 存入状态变量。
type CapacityWire struct {
	TotalMB     uint64  `json:"totalMB"`
	UsedMB      uint64  `json:"usedMB"`
	UsedPercent float64 `json:"usedPercent"`
}

type CPUWire struct {
	UsagePercent float64 `json:"usagePercent"`
}

type GPUWire struct {
	Available    bool    `json:"available"`
	UsagePercent float64 `json:"usagePercent,omitempty"`
}

type PowerWire struct {
	Available bool  `json:"available"`
	Percent   *int  `json:"percent,omitempty"`
	Charging  *bool `json:"charging,omitempty"`
	OnBattery *bool `json:"onBattery,omitempty"`
}

// SystemService 负责集中调度多个独立数据域的同步。
// 每个数据域各用一条 BLE 业务消息，互不挤占单个 payload 的长度预算；后续 API、
// 服务器等模块也应采用各自的 xxx.update 类型，而不是重新拼回一个总状态包。
type SystemService struct {
	publisher      Publisher
	collector      *systeminfo.Collector
	codexProvider  codexusage.Provider
	interval       time.Duration
	logf           func(format string, args ...any)
	lastCodexError string
}

// NewSystemService 创建集中状态同步服务。
//
// collector 和 codexProvider 分别负责不同数据域；服务按顺序发送 system.update
// 与 codex.update。这样一个模块变大或暂时失败，不会阻断其他模块的独立演进。
func NewSystemService(publisher Publisher, collector *systeminfo.Collector, codexProvider codexusage.Provider, interval time.Duration, logf func(string, ...any)) *SystemService {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if codexProvider == nil {
		codexProvider = codexusage.UnavailableProvider{}
	}
	return &SystemService{
		publisher:     publisher,
		collector:     collector,
		codexProvider: codexProvider,
		interval:      interval,
		logf:          logf,
	}
}

// Start 在后台周期同步系统信息。
//
// 未连接时不采集、不发送：既减少无意义的 pmset/磁盘查询，也避免日志被“未连接”
// 错误刷屏。连接恢复后的下一次 tick 会自动发送完整快照，使硬件状态重新对齐。
func (s *SystemService) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.syncOnce(ctx)
			}
		}
	}()
}

func (s *SystemService) syncOnce(ctx context.Context) {
	if !s.publisher.IsConnected() {
		return
	}

	snapshot, err := s.collector.Collect(ctx)
	if err != nil {
		s.log("系统信息采集失败：%v", err)
		return
	}

	// LocalProvider 从 Codex 已写入本机的会话事件读取最近额度；若尚未产生会话、
	// 本地记录跨过重置时间或格式变化，仍发送“不可用”状态，避免设备端长期展示
	// 已经过期的额度数字。
	codexSnapshot, err := s.codexProvider.Collect(ctx)
	if err != nil {
		// 缓存 Provider 在失效时可能连续返回相同错误。只记录首次或错误变化，
		// 让状态栏日志仍保留问题线索而不会每个 2 秒同步周期刷屏。
		if message := err.Error(); message != s.lastCodexError {
			s.log("Codex 用量采集失败：%v", err)
			s.lastCodexError = message
		}
		codexSnapshot = codexusage.Snapshot{Available: false}
	} else if s.lastCodexError != "" {
		s.log("Codex 用量采集已恢复")
		s.lastCodexError = ""
	}

	systemPayload := SystemPayload{
		CPU: CPUWire{UsagePercent: roundPercent(snapshot.CPU.UsagePercent)},
		GPU: GPUWire{Available: snapshot.GPU.Available, UsagePercent: roundPercent(snapshot.GPU.UsagePercent)},
		Memory: CapacityWire{
			TotalMB:     bytesToMB(snapshot.Memory.TotalBytes),
			UsedMB:      bytesToMB(snapshot.Memory.UsedBytes),
			UsedPercent: roundPercent(snapshot.Memory.UsedPercent),
		},
		Disk: CapacityWire{
			TotalMB:     bytesToMB(snapshot.Disk.TotalBytes),
			UsedMB:      bytesToMB(snapshot.Disk.UsedBytes),
			UsedPercent: roundPercent(snapshot.Disk.UsedPercent),
		},
		Power: PowerWire{
			Available: snapshot.Power.Available,
			Percent:   snapshot.Power.Percent,
			Charging:  snapshot.Power.Charging,
			OnBattery: snapshot.Power.OnBattery,
		},
	}
	if err := s.publisher.Send(ctx, "system.update", systemPayload); err != nil {
		s.log("系统信息推送失败：%v", err)
	} else {
		s.log("系统信息已推送：CPU %.1f%%，GPU %.1f%%，内存 %.1f%%，磁盘 %.1f%%", snapshot.CPU.UsagePercent, snapshot.GPU.UsagePercent, snapshot.Memory.UsedPercent, snapshot.Disk.UsedPercent)
	}

	if err := s.publisher.Send(ctx, "codex.update", codexSnapshot); err != nil {
		s.log("Codex 用量推送失败：%v", err)
		return
	}
	s.log("Codex 用量已推送：可用=%t", codexSnapshot.Available)
}

func bytesToMB(value uint64) uint64 { return value / (1024 * 1024) }

func roundPercent(value float64) float64 { return float64(int(value*10+0.5)) / 10 }

func (s *SystemService) log(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}
