# 数据同步调度指南

Status Deck 不会把 CPU、内存、磁盘、Codex 等所有数据拼成一条大消息后高频发送。
桌面端会把它们拆成独立任务：每种数据按自己的节奏采集，只在数据变化或需要保活时
写入 BLE。这样既能让 CPU/GPU 看起来实时，也能减少低频数据对蓝牙、ESP32 和电脑的
无意义消耗。

当前调度实现位于：

```text
desktop/internal/statussync/system.go
```

## 先理解两个参数

每个同步任务都使用 `TaskSchedule`：

```go
type TaskSchedule struct {
    Interval   time.Duration
    MaxSilence time.Duration
}
```

### `Interval`

`Interval` 是“多久检查一次”。到达这个时间时，桌面端才会采集该数据、与上一次成功
发送的内容比较。

它不等于“每隔多久必定写一次 BLE”。例如内存任务设为 `5 * time.Second`，内存数值
没有变化时，这轮不会发送。

### `MaxSilence`

`MaxSilence` 是“数据长期没有变化时，最久允许多久不发送”。超过这个时间，即使内容
相同也会补发一次。它有两个作用：

- 让设备端知道桌面端仍然活着，显示的数据不是停在旧画面上
- 在设备短暂重连、固件重启后，让静态数据最终重新对齐

`MaxSilence` 必须大于或等于 `Interval`。如果设置无效，程序会自动退回对应任务的
默认值。

## 当前默认策略

默认值由 `desktop/internal/statussync/system.go` 中的 `DefaultPlan()` 定义：

```go
func DefaultPlan() Plan {
    return Plan{
        Performance:  TaskSchedule{Interval: 2 * time.Second, MaxSilence: 10 * time.Second},
        Memory:       TaskSchedule{Interval: 5 * time.Second, MaxSilence: 30 * time.Second},
        StoragePower: TaskSchedule{Interval: 30 * time.Second, MaxSilence: 5 * time.Minute},
        Codex:        TaskSchedule{Interval: 15 * time.Second, MaxSilence: time.Minute},
    }
}
```

| 任务 | 包含字段或消息 | 检查频率 | 最长静默 | 选择原因 |
| --- | --- | --- | --- | --- |
| `Performance` | `cpu`、`gpu`，使用 `system.update` | 2 秒 | 10 秒 | 数值变化快，适合做实时仪表 |
| `Memory` | `memory`，使用 `system.update` | 5 秒 | 30 秒 | 变化较慢，不需要高频查询 |
| `StoragePower` | `disk`、`power`，使用 `system.update` | 30 秒 | 5 分钟 | 磁盘和供电通常变化很慢 |
| `Codex` | `codex.update` | 15 秒 | 1 分钟 | 本地会话文件不需要每秒扫描 |
| 心跳 | `heartbeat` | 10 秒 | - | BLE 连接保活，由 BLE 客户端管理 |

## 自己修改频率

最简单的方式是修改 `DefaultPlan()`。例如你觉得开发阶段 CPU/GPU 每秒刷新更直观，
而 Codex 用量每 30 秒检查一次就足够：

```go
func DefaultPlan() Plan {
    return Plan{
        Performance:  TaskSchedule{Interval: time.Second, MaxSilence: 5 * time.Second},
        Memory:       TaskSchedule{Interval: 5 * time.Second, MaxSilence: 30 * time.Second},
        StoragePower: TaskSchedule{Interval: time.Minute, MaxSilence: 10 * time.Minute},
        Codex:        TaskSchedule{Interval: 30 * time.Second, MaxSilence: 2 * time.Minute},
    }
}
```

不要把所有任务都改成 1 秒：这会增加 BLE 写入排队、CPU 查询和日志量，屏幕本身也不
需要那么快刷新。推荐只为肉眼可见且确实变化快的数据缩短周期。

应用启动时的默认配置在 `desktop/internal/config/config.go`：

```go
SyncPlan: statussync.DefaultPlan(),
```

以后做“设置”页面或配置文件时，不应改同步器内部代码；只需要从设置读出数值，构造
一个 `statussync.Plan` 后赋给 `config.Config.SyncPlan` 即可。

## 连接时为什么会立即发送

设备刚连上时，桌面端会调用 `SyncNow()`，依次补发性能、内存、磁盘/电源和 Codex
的当前状态。这样设备不会因为磁盘任务的 30 秒周期而长时间空白。

这次初始发送会记录为最近一次发送；紧接着的定时任务发现内容未变化时会自动跳过，
不会因为“首次同步”和“定时同步”同时写两遍。

## `system.update` 是局部更新

每个 `system.update` 只包含负责的数据字段，例如：

```json
{
  "cpu": { "usagePercent": 18.5 },
  "gpu": { "available": true, "usagePercent": 38.0 }
}
```

或：

```json
{
  "disk": { "totalMB": 953674, "usedMB": 400543, "usedPercent": 42.0 },
  "power": { "available": true, "percent": 86, "charging": true, "onBattery": false }
}
```

固件的 `DeviceStatusStore` 会合并出现的字段；未出现在 payload 的内存、磁盘、电源
等字段会保留原值。不要在局部更新中主动发送空对象或零值来代表“这次没采集”，否则
会覆盖设备端已有数据。

完整字段定义见 [BLE 通信协议](./ble-protocol.md)。

## 新增一个数据域

以“服务器状态”为例。建议它拥有独立的采集器、消息类型、设备端存储器和调度任务，
而不是塞进 `system.update`。

### 1. 定义消息和设备端存储

新增 `server.update`，payload 只描述服务器，例如：

```json
{
  "id": "prod-api",
  "online": true,
  "latencyMs": 42
}
```

固件创建 `ServerStatusStore`，只处理 `server.update`，显示 UI 从该存储器读取。协议
新增字段时优先使用可选字段，避免新桌面端与旧固件互相卡住。

### 2. 为计划增加调度项

在 `Plan` 中增加：

```go
Server TaskSchedule
```

然后在 `DefaultPlan()` 填入一个合适默认值，例如 HTTP 健康检查可以是：

```go
Server: TaskSchedule{Interval: 15 * time.Second, MaxSilence: time.Minute},
```

### 3. 编写独立同步函数

新增 `syncServer(ctx)`。它的结构应与现有 `syncCodex(ctx)` 相同：

1. 未连接时立即返回，不采集、不发包
2. 调用服务器采集器
3. 失败时发送明确的离线/不可用状态，或记录错误
4. 调用 `publishIfDue(ctx, "server", s.plan.Server, "server.update", payload)`

`publishIfDue` 会自动完成“内容变化检测”和“最长静默补发”，新模块不用再手写比较
逻辑。

### 4. 注册任务和首次同步

在 `Start()` 的任务列表中注册 `syncServer`，并在 `SyncNow()` 中调用它。这样它既会
按计划运行，也会在 BLE 重连后立即给屏幕一份初始状态。

## 调频建议

- 屏幕上需要动态刷新的百分比或速度：`1-3 秒`
- 一般系统状态：`5-15 秒`
- 本地文件、API 额度、服务器健康检查：`10-60 秒`
- 磁盘容量、电源接入状态：`30-60 秒`，并配合更长的 `MaxSilence`
- 网络请求类采集器：不要仅因屏幕需要刷新就高频请求；应缓存结果、设置超时，并把刷新频率单独控制

当 BLE 日志出现写入超时、缓冲区不足或设备端处理跟不上时，先增加对应任务的
`Interval`，不要只把消息拆得更碎。每个任务的 payload 也应尽量只传屏幕需要的字段。
