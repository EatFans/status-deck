package codexusage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	maxSessionFiles   = 80
	tailChunkBytes    = int64(1 << 20)
	defaultCacheTTL   = 5 * time.Second
	maxJSONLLineBytes = 2 << 20
)

// LocalProvider 从 Codex CLI 已写入本机的会话事件中读取用量。
//
// Codex 在 ~/.codex/sessions/**/*.jsonl 中记录 event_msg.token_count 事件，
// 其中的 payload.rate_limits 包含服务端在该次 Codex 活动时下发的额度快照。
// 本实现只读这些本地文件：不读取 auth.json、不使用 access token，也不会发起
// 任何网络请求。
//
// 本地数据只会在 Codex 自身有新活动时更新。某个窗口已到重置时间、但本地没有
// 更新后的事件时，Provider 会返回不可用，避免把已跨重置周期的旧数字当成实时值。
type LocalProvider struct {
	cacheTTL time.Duration
	now      func() time.Time

	mu          sync.Mutex
	lastFetched time.Time
	cached      Snapshot
	cachedErr   error
}

// NewLocalProvider 创建默认的本地 Codex 用量读取器。
//
// 状态卡每 2 秒同步一次，但会话 JSONL 文件不需要每次都扫描。五秒缓存能让用量
// 在 Codex 输出新事件后很快更新，同时避免常驻应用反复读取大量历史会话文件。
func NewLocalProvider() *LocalProvider {
	return &LocalProvider{cacheTTL: defaultCacheTTL, now: time.Now}
}

// Collect 返回最近一条有效 rate_limits 事件中的 5 小时与周额度。
func (p *LocalProvider) Collect(context.Context) (Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.lastFetched.IsZero() && p.now().Sub(p.lastFetched) < p.cacheTTL {
		return p.cached, p.cachedErr
	}

	p.lastFetched = p.now()
	p.cached, p.cachedErr = p.readLatest(p.now())
	return p.cached, p.cachedErr
}

func (p *LocalProvider) readLatest(now time.Time) (Snapshot, error) {
	sessionsDir, err := codexSessionsDir()
	if err != nil {
		return Snapshot{Available: false}, err
	}

	files, err := recentSessionFiles(sessionsDir)
	if err != nil {
		return Snapshot{Available: false}, err
	}
	if len(files) == 0 {
		return Snapshot{Available: false}, errors.New("未找到 Codex 会话记录，请先使用 Codex 完成一次对话")
	}

	var latest *rateLimitEvent
	for _, file := range files {
		candidate, err := readLatestRateLimitEvent(file)
		if err != nil {
			// 单个历史文件损坏或正在被 Codex 写入不影响其他会话文件的读取。
			continue
		}
		if candidate != nil && (latest == nil || candidate.at.After(latest.at)) {
			latest = candidate
		}

		// 文件已按修改时间倒序排列；后续文件若比已找到事件还旧，通常不可能
		// 包含更新的限额事件，可以提前停止，避免扫描整段历史。
		if latest != nil && file.modified.Before(latest.at) {
			break
		}
	}
	if latest == nil {
		return Snapshot{Available: false}, errors.New("Codex 会话记录中没有 rate_limits 事件")
	}

	fiveHour, weekly, err := latest.snapshot(now)
	if err != nil {
		return Snapshot{Available: false}, err
	}
	return Snapshot{Available: true, FiveHour: fiveHour, Weekly: weekly}, nil
}

func codexSessionsDir() (string, error) {
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate Codex home: %w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}

	sessionsDir := filepath.Join(codexHome, "sessions")
	info, err := os.Stat(sessionsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("未找到 Codex 会话目录，请先使用 Codex 完成一次对话")
		}
		return "", fmt.Errorf("read Codex session directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("Codex sessions 路径不是目录")
	}
	return sessionsDir, nil
}

type sessionFile struct {
	path     string
	modified time.Time
}

func recentSessionFiles(root string) ([]sessionFile, error) {
	var files []sessionFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // 跳过没有权限的单个目录，继续寻找其他会话文件。
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files = append(files, sessionFile{path: path, modified: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Codex session files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].modified.After(files[j].modified) })
	if len(files) > maxSessionFiles {
		files = files[:maxSessionFiles]
	}
	return files, nil
}

type sessionEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type       string          `json:"type"`
		RateLimits json.RawMessage `json:"rate_limits"`
	} `json:"payload"`
}

type rateLimitEvent struct {
	at         time.Time
	rateLimits json.RawMessage
}

// readLatestRateLimitEvent 只读取文件末尾。Codex 的最新事件位于 JSONL 的尾部，
// 不需要把一个可能很大的完整会话加载进内存。
func readLatestRateLimitEvent(file sessionFile) (*rateLimitEvent, error) {
	handle, err := os.Open(file.path)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	start := info.Size() - tailChunkBytes
	if start < 0 {
		start = 0
	}
	if _, err := handle.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}

	var latest *rateLimitEvent
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), maxJSONLLineBytes)
	for scanner.Scan() {
		var event sessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			// tail 从一行中间开始时，首行不是完整 JSON；忽略即可。
			continue
		}
		if event.Type != "event_msg" || event.Payload.Type != "token_count" ||
			len(event.Payload.RateLimits) == 0 || string(event.Payload.RateLimits) == "null" {
			continue
		}

		at, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			at = file.modified
		}
		if latest == nil || at.After(latest.at) {
			latest = &rateLimitEvent{at: at, rateLimits: event.Payload.RateLimits}
		}
	}
	return latest, scanner.Err()
}

func (event rateLimitEvent) snapshot(now time.Time) (*Window, *Window, error) {
	var limits map[string]json.RawMessage
	if err := json.Unmarshal(event.rateLimits, &limits); err != nil {
		return nil, nil, fmt.Errorf("decode local Codex rate limits: %w", err)
	}

	// 同时兼容目前 session 事件的 primary/secondary 和其他版本可能出现的
	// primary_window/secondary_window、five_hour/weekly 命名。
	fiveHourSource := firstWindow(limits, "five_hour", "primary_window", "primary")
	weeklySource := firstWindow(limits, "weekly", "secondary_window", "secondary")
	if fiveHourSource == nil && weeklySource == nil {
		return nil, nil, errors.New("本地 Codex rate_limits 缺少额度窗口")
	}

	// 当接口调整窗口排序时，时长仍然是最可靠的分类依据。
	all := []*localUsageWindow{fiveHourSource, weeklySource}
	var fiveHour, weekly *localUsageWindow
	for _, window := range all {
		if window == nil {
			continue
		}
		if window.durationSeconds() >= 2*24*60*60 {
			weekly = window
		} else {
			fiveHour = window
		}
	}
	if fiveHour == nil || weekly == nil {
		return nil, nil, errors.New("本地 Codex 用量缺少 5 小时或周额度")
	}

	fiveHourSnapshot, err := fiveHour.toSnapshot(now)
	if err != nil {
		return nil, nil, fmt.Errorf("decode local 5-hour Codex limit: %w", err)
	}
	weeklySnapshot, err := weekly.toSnapshot(now)
	if err != nil {
		return nil, nil, fmt.Errorf("decode local weekly Codex limit: %w", err)
	}
	return fiveHourSnapshot, weeklySnapshot, nil
}

type localUsageWindow struct {
	usedPercent   *float64
	windowMinutes float64
	windowSeconds float64
	resetAt       json.RawMessage
}

func firstWindow(limits map[string]json.RawMessage, keys ...string) *localUsageWindow {
	for _, key := range keys {
		raw, exists := limits[key]
		if !exists || string(raw) == "null" {
			continue
		}
		window, err := decodeWindow(raw)
		if err == nil {
			return window
		}
	}
	return nil
}

func decodeWindow(raw json.RawMessage) (*localUsageWindow, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}

	usedPercent, err := floatField(fields, "used_percent")
	if err != nil {
		return nil, err
	}
	minutes, _ := floatField(fields, "window_minutes")
	seconds, _ := floatField(fields, "limit_window_seconds")
	resetAt := firstField(fields, "resets_at", "reset_at")
	if len(resetAt) == 0 {
		return nil, errors.New("missing reset timestamp")
	}
	return &localUsageWindow{
		usedPercent:   usedPercent,
		windowMinutes: valueOrZero(minutes),
		windowSeconds: valueOrZero(seconds),
		resetAt:       resetAt,
	}, nil
}

func floatField(fields map[string]json.RawMessage, key string) (*float64, error) {
	raw, exists := fields[key]
	if !exists || string(raw) == "null" {
		return nil, errors.New("missing " + key)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	return &value, nil
}

func firstField(fields map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		if raw, exists := fields[key]; exists && string(raw) != "null" {
			return raw
		}
	}
	return nil
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func (window localUsageWindow) durationSeconds() float64 {
	if window.windowSeconds > 0 {
		return window.windowSeconds
	}
	return window.windowMinutes * 60
}

func (window localUsageWindow) toSnapshot(now time.Time) (*Window, error) {
	if window.usedPercent == nil {
		return nil, errors.New("missing used percentage")
	}
	resetAt, err := parseResetAt(window.resetAt)
	if err != nil {
		return nil, err
	}
	if !resetAt.After(now) {
		return nil, errors.New("local Codex usage snapshot is stale after its reset time")
	}

	remaining := 100 - *window.usedPercent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return &Window{RemainingPercent: remaining, ResetLabel: resetLabel(resetAt, now)}, nil
}

func parseResetAt(raw json.RawMessage) (time.Time, error) {
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		if number > 10_000_000_000 { // 兼容 Unix 毫秒。
			number /= 1000
		}
		return time.Unix(int64(number), 0), nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return time.Time{}, errors.New("invalid reset timestamp")
	}
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse reset timestamp %q: %w", text, err)
	}
	return value, nil
}

func resetLabel(resetAt, now time.Time) string {
	localReset := resetAt.In(time.Local)
	localNow := now.In(time.Local)
	if localReset.Year() == localNow.Year() && localReset.YearDay() == localNow.YearDay() {
		return localReset.Format("15:04")
	}
	return strconv.Itoa(int(localReset.Month())) + "月" + strconv.Itoa(localReset.Day()) + "日"
}
