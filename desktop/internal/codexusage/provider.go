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
	maxSessionFiles    = 80
	tailChunkBytes     = int64(1 << 20)
	defaultCacheTTL    = 5 * time.Second
	maxJSONLLineBytes  = 2 << 20
	cacheSchemaVersion = 1
)

// LocalProvider 从 Codex CLI 已写入本机的会话事件中读取用量。
//
// Codex 在 ~/.codex/sessions/**/*.jsonl 中记录 event_msg.token_count 事件，
// 其中的 payload.rate_limits 包含服务端在该次 Codex 活动时下发的额度快照。
// 本实现只读这些本地文件：不读取 auth.json、不使用 access token，也不会发起
// 任何网络请求。
//
// 本地数据只会在 Codex 自身有新活动时更新。每次成功读取都会将最后一次可信额度
// 写入系统缓存目录；之后即使会话文件暂时读不到（例如额度耗尽后没有新的终端输出），
// 只要尚未跨过该额度窗口的重置时间，仍会返回缓存中的值。这样 0% 是有效状态，
// 不会被错误地显示成“暂无本地数据”。
//
// 一旦缓存跨过任一窗口的重置时间，就不再使用旧数值：新周期的实际额度无法从旧事件
// 推断，必须等 Codex 产生新的 rate_limits 事件后才恢复显示。
type LocalProvider struct {
	cacheTTL  time.Duration
	now       func() time.Time
	cachePath string

	mu              sync.Mutex
	lastFetched     time.Time
	cached          Snapshot
	cachedErr       error
	lastKnown       usageCache
	loadedDiskCache bool
}

// usageCache 是落盘的“最近一次可信额度快照”。expiresAt 取两个额度窗口中较早
// 的重置时间，避免其中一个窗口已经重置后还向屏幕展示旧周期数据。
type usageCache struct {
	Version   int       `json:"version"`
	StoredAt  time.Time `json:"storedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Snapshot  Snapshot  `json:"snapshot"`
}

// NewLocalProvider 创建默认的本地 Codex 用量读取器。
//
// Codex 页面当前每 5 秒检查一次，但会话 JSONL 文件不需要每次都扫描。五秒缓存能让
// 用量在 Codex 输出新事件后很快更新，同时避免常驻应用反复读取大量历史会话文件。
func NewLocalProvider() *LocalProvider {
	cachePath, err := codexUsageCachePath()
	if err != nil {
		// 无法定位系统缓存目录不影响会话文件读取；本进程内仍会保留最后一次成功值。
		cachePath = ""
	}
	return &LocalProvider{
		cacheTTL:  defaultCacheTTL,
		now:       time.Now,
		cachePath: cachePath,
	}
}

// Collect 返回最近一条有效 rate_limits 事件中的 5 小时与周额度。
func (p *LocalProvider) Collect(context.Context) (Snapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if !p.lastFetched.IsZero() && now.Sub(p.lastFetched) < p.cacheTTL {
		return p.cached, p.cachedErr
	}

	p.lastFetched = now

	cache, err := p.readLatest(now)
	if err == nil {
		shouldPersist := !sameUsageCache(p.lastKnown, cache)
		p.lastKnown = cache
		if shouldPersist {
			p.writeDiskCache(cache)
		}
		p.cached = cache.Snapshot
		p.cachedErr = nil
		return p.cached, nil
	}

	// 首次失败时才读取磁盘缓存；后续周期直接复用内存对象，避免每 5 秒访问磁盘。
	if !p.loadedDiskCache {
		p.loadedDiskCache = true
		if diskCache, loadErr := p.readDiskCache(); loadErr == nil && diskCache.validAt(now) {
			p.lastKnown = diskCache
		}
	}
	if p.lastKnown.validAt(now) {
		p.cached = p.lastKnown.Snapshot
		p.cachedErr = nil
		return p.cached, nil
	}

	p.cached = Snapshot{Available: false}
	p.cachedErr = err
	return p.cached, p.cachedErr
}

func (p *LocalProvider) readLatest(now time.Time) (usageCache, error) {
	sessionsDir, err := codexSessionsDir()
	if err != nil {
		return usageCache{}, err
	}

	files, err := recentSessionFiles(sessionsDir)
	if err != nil {
		return usageCache{}, err
	}
	if len(files) == 0 {
		return usageCache{}, errors.New("未找到 Codex 会话记录，请先使用 Codex 完成一次对话")
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
		return usageCache{}, errors.New("Codex 会话记录中没有 rate_limits 事件")
	}

	snapshot, expiresAt, err := latest.snapshot(now)
	if err != nil {
		return usageCache{}, err
	}
	return usageCache{
		Version:   cacheSchemaVersion,
		StoredAt:  now,
		ExpiresAt: expiresAt,
		Snapshot:  snapshot,
	}, nil
}

func (cache usageCache) validAt(now time.Time) bool {
	return cache.Version == cacheSchemaVersion && cache.Snapshot.Available &&
		cache.Snapshot.FiveHour != nil && cache.Snapshot.Weekly != nil &&
		cache.ExpiresAt.After(now)
}

// sameUsageCache 忽略 StoredAt：仅仅重新扫描到同一份 session 事件不应造成额外
// 磁盘写入。额度、显示标签或任一窗口的重置时间变化时才需要覆盖缓存文件。
func sameUsageCache(left, right usageCache) bool {
	return left.Version == right.Version && left.ExpiresAt.Equal(right.ExpiresAt) &&
		sameSnapshot(left.Snapshot, right.Snapshot)
}

func sameSnapshot(left, right Snapshot) bool {
	if left.Available != right.Available {
		return false
	}
	return sameWindow(left.FiveHour, right.FiveHour) && sameWindow(left.Weekly, right.Weekly)
}

func sameWindow(left, right *Window) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.RemainingPercent == right.RemainingPercent && left.ResetLabel == right.ResetLabel
}

// codexUsageCachePath 使用操作系统推荐的缓存目录。macOS 通常为
// ~/Library/Caches/Status Deck/codex-usage.json；它不是认证信息，删除后仅会让
// 状态卡等待下一条本地 Codex 会话事件重新生成快照。
func codexUsageCachePath() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache directory: %w", err)
	}
	return filepath.Join(root, "Status Deck", "codex-usage.json"), nil
}

func (p *LocalProvider) readDiskCache() (usageCache, error) {
	if p.cachePath == "" {
		return usageCache{}, errors.New("Codex usage cache path is unavailable")
	}
	data, err := os.ReadFile(p.cachePath)
	if err != nil {
		return usageCache{}, err
	}
	var cache usageCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return usageCache{}, fmt.Errorf("decode Codex usage cache: %w", err)
	}
	return cache, nil
}

// writeDiskCache 使用临时文件再原子替换，避免应用被强制退出时留下半截 JSON。
// 缓存写入失败不影响刚从本地会话成功读出的额度，也不应让状态卡变成不可用。
func (p *LocalProvider) writeDiskCache(cache usageCache) {
	if p.cachePath == "" {
		return
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.cachePath), 0700); err != nil {
		return
	}
	temporary := p.cachePath + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return
	}
	_ = os.Rename(temporary, p.cachePath)
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

func (event rateLimitEvent) snapshot(now time.Time) (Snapshot, time.Time, error) {
	var limits map[string]json.RawMessage
	if err := json.Unmarshal(event.rateLimits, &limits); err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("decode local Codex rate limits: %w", err)
	}

	// 同时兼容目前 session 事件的 primary/secondary 和其他版本可能出现的
	// primary_window/secondary_window、five_hour/weekly 命名。
	fiveHourSource := firstWindow(limits, "five_hour", "primary_window", "primary")
	weeklySource := firstWindow(limits, "weekly", "secondary_window", "secondary")
	if fiveHourSource == nil && weeklySource == nil {
		return Snapshot{}, time.Time{}, errors.New("本地 Codex rate_limits 缺少额度窗口")
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
		return Snapshot{}, time.Time{}, errors.New("本地 Codex 用量缺少 5 小时或周额度")
	}

	fiveHourSnapshot, fiveHourResetAt, err := fiveHour.toSnapshot(now)
	if err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("decode local 5-hour Codex limit: %w", err)
	}
	weeklySnapshot, weeklyResetAt, err := weekly.toSnapshot(now)
	if err != nil {
		return Snapshot{}, time.Time{}, fmt.Errorf("decode local weekly Codex limit: %w", err)
	}
	expiresAt := fiveHourResetAt
	if weeklyResetAt.Before(expiresAt) {
		expiresAt = weeklyResetAt
	}
	return Snapshot{Available: true, FiveHour: fiveHourSnapshot, Weekly: weeklySnapshot}, expiresAt, nil
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

func (window localUsageWindow) toSnapshot(now time.Time) (*Window, time.Time, error) {
	if window.usedPercent == nil {
		return nil, time.Time{}, errors.New("missing used percentage")
	}
	resetAt, err := parseResetAt(window.resetAt)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !resetAt.After(now) {
		return nil, time.Time{}, errors.New("local Codex usage snapshot is stale after its reset time")
	}

	remaining := 100 - *window.usedPercent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return &Window{RemainingPercent: remaining, ResetLabel: resetLabel(resetAt, now)}, resetAt, nil
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
