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
			Interval:   1 * time.Minute,
			MaxSilence: 1 * time.Minute,
		},
	}
}
