# Status Deck Desktop

Status Deck Desktop 是运行在电脑上的轻量上位机程序。

第一版先做成 CLI：采集电脑和网络状态数据，通过 BLE 实时发送给 ESP32-S3 状态卡。后续可以在同一个 Go 程序上增加 Windows 托盘 / macOS 菜单栏模式。

## 目标

- 轻量常驻，尽量少占用电脑资源
- 通过 BLE 自动发现并连接 Status Deck 硬件
- 定时采集本机状态、API 用量和服务器健康状态
- 将数据打包成统一 JSON 协议发送给 ESP32-S3
- 支持断线自动重连
- 后续支持状态栏小 UI

## 项目结构

```text
desktop/
├── cmd/status-deck/           # CLI 入口
├── internal/ble/              # BLE 扫描、连接、写入和重连
├── internal/collectors/       # 本机/API/服务器状态采集
├── internal/config/           # 配置加载
├── internal/protocol/         # 发给硬件的数据结构
└── go.mod
```

## 计划命令

```bash
status-deck run
status-deck scan
status-deck scan --all --timeout 10s
status-deck version
```

当前 `scan` 已接入 `tinygo.org/x/bluetooth`，会扫描设备名或 Service UUID 匹配 Status Deck 的 BLE 设备。`run` 的连接、订阅和写入还在实现中。

## BLE 协议

固件侧当前使用以下 UUID：

```text
Service UUID: 7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000
RX UUID:      7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000
TX UUID:      7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000
```

电脑端写入 RX，ESP32-S3 通过 TX notify 回传确认或状态。

## 开发状态

当前是项目骨架，BLE 连接层还没有接入具体系统库。下一步会先验证 macOS/Windows 上的 Go BLE 能力，再决定是否使用 `tinygo.org/x/bluetooth` 或平台专用实现。
