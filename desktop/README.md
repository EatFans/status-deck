# Status Deck Desktop

Status Deck Desktop 是运行在电脑上的轻量上位机程序。

第一版就按状态栏应用来做：启动后挂载到 Windows 托盘或 macOS 菜单栏中，采集电脑和网络状态数据，通过 BLE 实时发送给 ESP32-S3 状态卡。开发调试命令保留在同一个 Go 程序里，详见 [桌面客户端设计](../docs/desktop-app.md)。

## 目标

- 轻量常驻，尽量少占用电脑资源
- 通过 BLE 自动发现并连接 Status Deck 硬件
- 定时采集本机状态、API 用量和服务器健康状态
- 将数据打包成统一 JSON 协议发送给 ESP32-S3
- 支持断线自动重连
- 默认以状态栏/托盘应用运行

## 项目结构

```text
desktop/
├── cmd/status-deck/           # CLI 入口
├── internal/app/              # 无界面 Agent 编排，供 tray/run 共享
├── internal/ble/              # BLE 扫描、连接、写入和重连
├── internal/config/           # 配置加载
├── internal/statussync/       # 按数据域调度采集与发送
├── internal/systeminfo/       # CPU、内存、磁盘、电量、GPU 采集
└── go.mod
```

`internal/app/` 负责组装正式运行链路；状态栏模式和 `run` 调试模式都会使用它，避免维护两套采集和协议实现。

## 启动方式

```bash
status-deck
```

直接启动 `status-deck` 会进入状态栏/托盘模式。

开发调试命令：

```bash
status-deck tray
status-deck run
status-deck scan
status-deck scan --all --timeout 10s
status-deck version
```

`scan` 会扫描设备名或 Service UUID 匹配 Status Deck 的 BLE 设备。`run` 会启动与状态栏模式一致的自动连接、系统状态和 Codex 用量同步流程，但不创建状态栏菜单。

状态栏菜单里的“调试日志”会打开系统终端并实时跟踪应用日志。macOS 当前使用 Terminal + `tail -f`。

## macOS 打包与开机自启

在 macOS 上执行以下命令，会生成状态栏应用和可分发 zip：

```bash
./scripts/package-macos.sh
```

输出文件位于：

```text
desktop/dist/Status Deck.app
desktop/dist/Status-Deck-macos.zip
```

该脚本使用 ad-hoc 签名，适合本机测试。向其他用户分发时，需要使用 Apple Developer ID 签名并完成 notarization，才能避免 Gatekeeper 的来源警告。

打包过程会从代码生成 `StatusDeck.icns`，并将它放入 `.app/Contents/Resources`。托盘图标与 Finder/Dock 应用图标来自同一份图标定义，不需要额外携带图片文件。

状态栏菜单的“设置 > 开机自启”会管理当前用户的 LaunchAgent：

```text
~/Library/LaunchAgents/com.statusdeck.desktop.plist
```

启用后，Status Deck 会在下次登录 macOS 时启动。请先将 `Status Deck.app` 放在稳定位置，例如 `/Applications`；若移动或删除应用，需要关闭后重新开启一次开机自启，以更新记录的可执行文件路径。

## Windows 打包与开机自启

请在 Windows 本机的 PowerShell 中执行以下命令，生成无控制台窗口的桌面端可执行文件和 zip：

```powershell
.\scripts\package-windows.ps1
```

输出文件位于：

```text
desktop\dist\Status Deck.exe
desktop\dist\Status-Deck-windows.zip
```

脚本会从同一份代码生成 `.ico`，再使用 `rsrc` 临时生成 Windows 资源文件并嵌入 `Status Deck.exe`。最终 `.exe` 不依赖 `.ico` 文件；首次打包时 Go 会下载固定版本的 `github.com/akavel/rsrc` 工具。当前脚本面向 Windows amd64。

“设置 > 开机自启”会在当前用户的注册表中创建或删除以下值，不需要管理员权限：

```text
HKCU\Software\Microsoft\Windows\CurrentVersion\Run
值名称：Status Deck
```

请在启用前将 `Status Deck.exe` 放在稳定位置，例如 `%LOCALAPPDATA%\Status Deck`。若移动或删除可执行文件，需要关闭后重新开启一次开机自启，以更新启动路径。Windows 可能在登录后延后启动后台程序，这是系统的正常行为。

## GitHub Actions 手动打包

仓库包含 `Package Desktop Apps` 工作流。将代码推送到 `main` 后，在 GitHub 仓库的 **Actions** 页面选择该工作流，点击 **Run workflow**，并保持分支为 `main`。工作流会并行打包 macOS 和 Windows，并在运行完成后提供两个可下载构建产物：

```text
status-deck-macos    -> Status-Deck-macos.zip
status-deck-windows  -> Status-Deck-windows.zip
```

工作流只允许 `main` 分支执行；从其他分支手动触发时，打包任务会被跳过。

## BLE 协议

固件侧当前使用以下 UUID：

```text
Service UUID: 7f3a0001-6c21-4b7d-9d5d-1f4f2b3a9000
RX UUID:      7f3a0002-6c21-4b7d-9d5d-1f4f2b3a9000
TX UUID:      7f3a0003-6c21-4b7d-9d5d-1f4f2b3a9000
```

电脑端写入 RX，ESP32-S3 通过 TX notify 回传确认或状态。

## 开发状态

当前桌面端已经具备状态栏运行、BLE 扫描与自动连接、系统状态与 Codex 用量同步、开机自启和跨平台打包能力。后续新增数据源应放入独立采集模块，再由 `internal/statussync/` 注册同步任务。
