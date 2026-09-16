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
Status Deck
Disconnected / Connected: Status Deck
Last Sync: 14:32:10

Device Manager
  Scan Devices
  Current Device

Settings
  Launch at Login
  Debug Logs

Quit
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
- 调试日志
- 启动后自动扫描设备
- 启动后自动连接上次设备

后续可扩展：

- 同步间隔
- 心跳间隔
- 数据源开关
- API Key 管理
- 服务器健康检查配置

## 退出应用

菜单提供 `Quit`。

退出时应：

- 停止数据采集
- 停止 BLE 扫描
- 断开当前设备连接
- 保存配置
- 退出状态栏程序

## 技术方案

核心程序使用 Go。

状态栏 UI 使用：

```text
github.com/getlantern/systray
```

这套方案适合轻量常驻程序，资源占用低，能覆盖 Windows 托盘和 macOS 菜单栏。
