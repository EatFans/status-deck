# 桌面客户端设计

桌面客户端默认是轻量状态栏应用：启动后挂载在 Windows 托盘或 macOS 菜单栏中，负责扫描和连接硬件设备，采集本机/网络/API 状态，并通过 BLE 实时发送给 Status Deck 设备。

## 运行方式

```text
status-deck        # 状态栏/托盘模式
```

开发调试命令：

```text
status-deck tray   # 显式启动状态栏/托盘模式
status-deck run    # CLI/后台同步模式
status-deck scan   # BLE 扫描调试
```

## 状态栏菜单

点击状态栏图标后显示菜单。

建议结构：

```text
状态栏图标
未连接 / 已连接：Status Deck
上次同步：14:32:10

设备管理
  扫描设备
  当前设备

设置
  开机自启
  调试日志

退出
```

## 设备管理

设备管理用于查看和控制硬件连接。

第一版需要：

- 显示当前连接状态
- 主动扫描附近的 Status Deck 设备
- 显示扫描结果
- 后续支持点击设备后连接
- 后续支持断开当前设备
- 后续支持忘记已保存设备

## 设置

第一版设置项：

- 开机自启
- 调试日志：勾上时打开系统自带终端并实时输出应用日志，取消勾选时关闭日志终端
- 启动后自动扫描设备
- 启动后自动连接上次设备

后续可扩展：

- 各数据域同步间隔
- 心跳间隔
- 数据源开关
- API Key 管理
- 服务器健康检查配置

## 退出应用

菜单提供 `退出`。

退出时应：

- 停止数据采集
- 停止 BLE 扫描
- 断开当前设备连接
- 保存配置
- 退出状态栏程序

## 同步调度

状态栏应用不使用一个全局的高频同步循环，而是为每个数据域独立配置任务。每个任务
都有两个值：`Interval` 表示检查/采集频率，`MaxSilence` 表示内容不变时最多多久仍
要重新发送一次。内容未变化时会跳过 BLE 写入。

默认策略：

| 数据域 | BLE 消息 | Interval | MaxSilence |
| --- | --- | --- | --- |
| CPU、GPU | `system.update` | 2 秒 | 10 秒 |
| 内存 | `system.update` | 5 秒 | 30 秒 |
| 磁盘、电源 | `system.update` | 30 秒 | 5 分钟 |
| Codex 用量 | `codex.update` | 15 秒 | 1 分钟 |
| 连接保活 | `heartbeat` | 10 秒 | - |

默认值定义在 `desktop/internal/statussync/sync_config.go` 的 `DefaultPlan()`，应用配置通过
`config.Config.SyncPlan` 持有该计划。未来增加 API 额度或服务器状态时，应新增一个
任务、一个 `xxx.update` 消息及设备端对应存储器，不与既有数据拼成总 payload。

## 技术方案

核心程序使用 Go。

状态栏 UI 使用：

```text
github.com/getlantern/systray
```

这套方案适合轻量常驻程序，资源占用低，能覆盖 Windows 托盘和 macOS 菜单栏。
