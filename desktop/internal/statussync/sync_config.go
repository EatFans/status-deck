package statussync

import "time"

// DefaultPlan is the single compile-time configuration for desktop-to-device
// data delivery. It is deliberately a Go source file instead of a runtime
// JSON setting: packaged applications embed these values in the binary.
//
// Page mapping:
//   - System: Performance, Memory, StoragePower
//   - Codex:  Codex
//
// Interval controls how often a visible page's source is checked. MaxSilence
// forces a refresh for unchanged visible data so the device can recover from a
// missed update without requiring a reconnect.
func DefaultPlan() Plan {
	return Plan{
		Performance: TaskSchedule{
			Interval:   2 * time.Second,
			MaxSilence: 10 * time.Second,
		},
		Memory: TaskSchedule{
			Interval:   5 * time.Second,
			MaxSilence: 30 * time.Second,
		},
		StoragePower: TaskSchedule{
			Interval:   30 * time.Second,
			MaxSilence: 5 * time.Minute,
		},
		Codex: TaskSchedule{
			// 切到 Codex 页后，若本地会话刚跨过额度重置点，下一条有效快照可能
			// 稍后才写入。五秒检查可让恢复后的数据几乎立刻显示；同步器只在
			// Codex 页可见时运行此任务，且未变化时不会重复发送 BLE。
			Interval:   5 * time.Second,
			MaxSilence: 1 * time.Minute,
		},
	}
}
