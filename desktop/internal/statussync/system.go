// Package statussync 负责按不同数据域的节奏向硬件同步状态。
//
// 采集器只负责拿到数据，本包负责何时采集、何时发送和内容变化检测。新的数据源
// 只需新增一个同步任务，不必与其他模块竞争同一条“大而全”的状态消息。
package statussync

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"status-deck/desktop/internal/codexusage"
	"status-deck/desktop/internal/systeminfo"
)

// Publisher 是同步服务依赖的最小 BLE 能力。
// ble.Client 满足该接口；保留接口能让本包在不依赖真实蓝牙硬件时单独测试。
type Publisher interface {
	IsConnected() bool
	Send(ctx context.Context, messageType string, payload any) error
}

// TaskSchedule 定义一个独立数据域的同步策略。
//
// Interval 是采集检查频率；MaxSilence 则避免数据长期不变时设备端无法确认状态仍
// 新鲜。发送内容不变时会跳过，直到 MaxSilence 到期才强制补发一次。
type TaskSchedule struct {
	Interval   time.Duration
	MaxSilence time.Duration
}

// Plan 是 Status Deck 当前内置数据域的可配置调度表。
// 以后新增 api、server 等模块时，为 Plan 增加一个 TaskSchedule 并注册任务即可。
type Plan struct {
	Performance  TaskSchedule
	Memory       TaskSchedule
	StoragePower TaskSchedule
	Codex        TaskSchedule
}

// DefaultPlan 返回兼顾桌面实时感、功耗和 BLE 稳定性的默认策略。
func DefaultPlan() Plan {
	return Plan{
		Performance:  TaskSchedule{Interval: 2 * time.Second, MaxSilence: 10 * time.Second},
		Memory:       TaskSchedule{Interval: 5 * time.Second, MaxSilence: 30 * time.Second},
		StoragePower: TaskSchedule{Interval: 30 * time.Second, MaxSilence: 5 * time.Minute},
		Codex:        TaskSchedule{Interval: 15 * time.Second, MaxSilence: time.Minute},
	}
}

// SystemPayload 是 system.update 的局部 payload。各任务只填充自己负责的字段，
// 指针配合 omitempty 确保不会把未采集字段序列化成空对象或错误的零值。
type SystemPayload struct {
	CPU    *CPUWire      `json:"cpu,omitempty"`
	GPU    *GPUWire      `json:"gpu,omitempty"`
	Memory *CapacityWire `json:"memory,omitempty"`
	Disk   *CapacityWire `json:"disk,omitempty"`
	Power  *PowerWire    `json:"power,omitempty"`
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

type syncTask struct {
	name     string
	schedule TaskSchedule
	run      func(context.Context)
}

type sentState struct {
	payload []byte
	at      time.Time
}

// SystemService 负责启动和维护所有独立同步任务。
type SystemService struct {
	publisher     Publisher
	collector     *systeminfo.Collector
	codexProvider codexusage.Provider
	plan          Plan
	logf          func(format string, args ...any)

	stateMu        sync.Mutex
	lastSent       map[string]sentState
	lastCodexError string

	// SyncNow 与定时 tick 可能在连接刚建立的瞬间重叠。每个数据域各自串行，
	// 防止同一采集器并发访问、重复 BLE 写入或同时修改 Codex 错误状态。
	performanceMu  sync.Mutex
	memoryMu       sync.Mutex
	storagePowerMu sync.Mutex
	codexMu        sync.Mutex
}

// NewSystemService 创建集中同步服务。传入的 Plan 可由配置文件、设置页面或测试
// 覆盖；无效的任务时长会按默认值补齐。
func NewSystemService(publisher Publisher, collector *systeminfo.Collector, codexProvider codexusage.Provider, plan Plan, logf func(string, ...any)) *SystemService {
	if codexProvider == nil {
		codexProvider = codexusage.UnavailableProvider{}
	}
	return &SystemService{
		publisher:     publisher,
		collector:     collector,
		codexProvider: codexProvider,
		plan:          normalizePlan(plan),
		logf:          logf,
		lastSent:      make(map[string]sentState),
	}
}

// Start 为每个数据域启动独立轻量定时器。
// 未连接时任务不会采集，避免磁盘、GPU 和会话文件查询白白消耗资源；设备重新连接
// 后，每个任务在下一次 tick 自动恢复同步。
func (s *SystemService) Start(ctx context.Context) {
	for _, task := range []syncTask{
		{name: "performance", schedule: s.plan.Performance, run: s.syncPerformance},
		{name: "memory", schedule: s.plan.Memory, run: s.syncMemory},
		{name: "storage_power", schedule: s.plan.StoragePower, run: s.syncStoragePower},
		{name: "codex", schedule: s.plan.Codex, run: s.syncCodex},
	} {
		task := task
		go s.runTask(ctx, task)
	}
}

// SyncNow 在设备刚建立连接后补齐一份完整初始状态。
//
// 这不是额外的高频循环：各任务随后仍遵循自己的 Interval。publishIfDue 会记录这次
// 初始发送，因此紧接着到来的定时 tick 若内容未变会自动跳过，不会造成重复写入。
func (s *SystemService) SyncNow(ctx context.Context) {
	go func() {
		s.syncPerformance(ctx)
		s.syncMemory(ctx)
		s.syncStoragePower(ctx)
		s.syncCodex(ctx)
	}()
}

func (s *SystemService) runTask(ctx context.Context, task syncTask) {
	// 首次连接后的下一轮无需等待完整周期，先尝试同步一份初始状态。
	task.run(ctx)
	ticker := time.NewTicker(task.schedule.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			task.run(ctx)
		}
	}
}

func (s *SystemService) syncPerformance(ctx context.Context) {
	s.performanceMu.Lock()
	defer s.performanceMu.Unlock()
	if !s.publisher.IsConnected() {
		return
	}
	performance, err := s.collector.CollectPerformance(ctx)
	if err != nil {
		s.log("性能信息采集失败：%v", err)
		return
	}
	payload := SystemPayload{
		CPU: &CPUWire{UsagePercent: roundPercent(performance.CPU.UsagePercent)},
		GPU: &GPUWire{Available: performance.GPU.Available, UsagePercent: roundPercent(performance.GPU.UsagePercent)},
	}
	if sent, err := s.publishIfDue(ctx, "performance", s.plan.Performance, "system.update", payload); err != nil {
		s.log("性能信息推送失败：%v", err)
	} else if sent {
		s.log("性能信息已推送：CPU %.1f%%，GPU %.1f%%", performance.CPU.UsagePercent, performance.GPU.UsagePercent)
	}
}

func (s *SystemService) syncMemory(ctx context.Context) {
	s.memoryMu.Lock()
	defer s.memoryMu.Unlock()
	if !s.publisher.IsConnected() {
		return
	}
	memory, err := s.collector.CollectMemory(ctx)
	if err != nil {
		s.log("内存信息采集失败：%v", err)
		return
	}
	payload := SystemPayload{Memory: &CapacityWire{
		TotalMB: bytesToMB(memory.TotalBytes), UsedMB: bytesToMB(memory.UsedBytes), UsedPercent: roundPercent(memory.UsedPercent),
	}}
	if sent, err := s.publishIfDue(ctx, "memory", s.plan.Memory, "system.update", payload); err != nil {
		s.log("内存信息推送失败：%v", err)
	} else if sent {
		s.log("内存信息已推送：%.1f%%", memory.UsedPercent)
	}
}

func (s *SystemService) syncStoragePower(ctx context.Context) {
	s.storagePowerMu.Lock()
	defer s.storagePowerMu.Unlock()
	if !s.publisher.IsConnected() {
		return
	}
	disk, err := s.collector.CollectDisk(ctx)
	if err != nil {
		s.log("磁盘信息采集失败：%v", err)
		return
	}
	power := s.collector.CollectPower(ctx)
	payload := SystemPayload{
		Disk:  &CapacityWire{TotalMB: bytesToMB(disk.TotalBytes), UsedMB: bytesToMB(disk.UsedBytes), UsedPercent: roundPercent(disk.UsedPercent)},
		Power: &PowerWire{Available: power.Available, Percent: power.Percent, Charging: power.Charging, OnBattery: power.OnBattery},
	}
	if sent, err := s.publishIfDue(ctx, "storage_power", s.plan.StoragePower, "system.update", payload); err != nil {
		s.log("磁盘和电源信息推送失败：%v", err)
	} else if sent {
		s.log("磁盘和电源信息已推送：磁盘 %.1f%%", disk.UsedPercent)
	}
}

func (s *SystemService) syncCodex(ctx context.Context) {
	s.codexMu.Lock()
	defer s.codexMu.Unlock()
	if !s.publisher.IsConnected() {
		return
	}
	snapshot, err := s.codexProvider.Collect(ctx)
	if err != nil {
		if message := err.Error(); message != s.lastCodexError {
			s.log("Codex 用量采集失败：%v", err)
			s.lastCodexError = message
		}
		snapshot = codexusage.Snapshot{Available: false}
	} else if s.lastCodexError != "" {
		s.log("Codex 用量采集已恢复")
		s.lastCodexError = ""
	}
	if sent, err := s.publishIfDue(ctx, "codex", s.plan.Codex, "codex.update", snapshot); err != nil {
		s.log("Codex 用量推送失败：%v", err)
	} else if sent {
		s.log("Codex 用量已推送：可用=%t", snapshot.Available)
	}
}

// publishIfDue 通过序列化后的 payload 判断内容是否变化。这样无需为每种新模块
// 手写比较器；MaxSilence 仍会确保长期静止的数据偶尔重新发送以修正连接恢复后的
// 显示状态。
func (s *SystemService) publishIfDue(ctx context.Context, key string, schedule TaskSchedule, messageType string, payload any) (bool, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	now := time.Now()

	s.stateMu.Lock()
	previous, exists := s.lastSent[key]
	unchanged := exists && string(previous.payload) == string(encoded)
	due := !exists || !unchanged || now.Sub(previous.at) >= schedule.MaxSilence
	s.stateMu.Unlock()
	if !due {
		return false, nil
	}

	if err := s.publisher.Send(ctx, messageType, payload); err != nil {
		return false, err
	}

	s.stateMu.Lock()
	s.lastSent[key] = sentState{payload: append([]byte(nil), encoded...), at: now}
	s.stateMu.Unlock()
	return true, nil
}

func normalizePlan(plan Plan) Plan {
	defaults := DefaultPlan()
	plan.Performance = normalizeSchedule(plan.Performance, defaults.Performance)
	plan.Memory = normalizeSchedule(plan.Memory, defaults.Memory)
	plan.StoragePower = normalizeSchedule(plan.StoragePower, defaults.StoragePower)
	plan.Codex = normalizeSchedule(plan.Codex, defaults.Codex)
	return plan
}

func normalizeSchedule(schedule, fallback TaskSchedule) TaskSchedule {
	if schedule.Interval <= 0 {
		schedule.Interval = fallback.Interval
	}
	if schedule.MaxSilence < schedule.Interval {
		schedule.MaxSilence = fallback.MaxSilence
	}
	return schedule
}

func bytesToMB(value uint64) uint64 { return value / (1024 * 1024) }

func roundPercent(value float64) float64 { return float64(int(value*10+0.5)) / 10 }

func (s *SystemService) log(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}
