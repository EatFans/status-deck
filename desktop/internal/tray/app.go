package tray

import (
	"context"
	"fmt"
	"time"

	"github.com/getlantern/systray"

	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/config"
)

type App struct {
	config config.Config
	client *ble.Client

	statusItem   *systray.MenuItem
	lastSyncItem *systray.MenuItem
	scanItem     *systray.MenuItem
	autoStart    *systray.MenuItem
	quitItem     *systray.MenuItem
}

func NewApp(config config.Config) *App {
	return &App{
		config: config,
		client: ble.NewClient(config.BLE),
	}
}

func (a *App) Run() error {
	systray.Run(a.onReady, a.onExit)
	return nil
}

func (a *App) onReady() {
	systray.SetIcon(statusDeckIconPNG())
	systray.SetTitle("")
	systray.SetTooltip("Status Deck 状态卡")

	a.statusItem = systray.AddMenuItem("未连接", "当前设备连接状态")
	a.statusItem.Disable()

	a.lastSyncItem = systray.AddMenuItem("上次同步：从未", "最近一次成功同步时间")
	a.lastSyncItem.Disable()

	systray.AddSeparator()

	deviceManager := systray.AddMenuItem("设备管理", "扫描和管理 Status Deck 设备")
	a.scanItem = deviceManager.AddSubMenuItem("扫描设备", "扫描附近的 Status Deck 设备")
	deviceStatus := deviceManager.AddSubMenuItem("未选择设备", "当前选择的设备")
	deviceStatus.Disable()

	settings := systray.AddMenuItem("设置", "打开 Status Deck 设置")
	a.autoStart = settings.AddSubMenuItemCheckbox("开机自启", "电脑启动时自动启动 Status Deck", false)
	debugLogs := settings.AddSubMenuItemCheckbox("调试日志", "开发时输出更详细的日志", false)
	debugLogs.Disable()

	systray.AddSeparator()

	a.quitItem = systray.AddMenuItem("退出", "退出 Status Deck")

	go a.handleScan(deviceStatus)
	go a.handleAutoStart()
	go a.handleQuit()
}

func (a *App) onExit() {
	_ = a.client.Close()
}

func (a *App) handleScan(deviceStatus *systray.MenuItem) {
	for range a.scanItem.ClickedCh {
		a.statusItem.SetTitle("正在扫描...")
		a.scanItem.Disable()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		devices, err := a.client.ScanWithOptions(ctx, ble.ScanOptions{
			Timeout: 10 * time.Second,
		})
		cancel()

		a.scanItem.Enable()

		if err != nil {
			a.statusItem.SetTitle("扫描失败")
			deviceStatus.SetTitle(fmt.Sprintf("错误：%v", err))
			continue
		}

		if len(devices) == 0 {
			a.statusItem.SetTitle("未连接")
			deviceStatus.SetTitle("未发现 Status Deck 设备")
			continue
		}

		device := devices[0]
		name := device.Name
		if name == "" {
			name = device.ID
		}

		a.statusItem.SetTitle("已发现：" + name)
		deviceStatus.SetTitle(fmt.Sprintf("%s RSSI=%d", name, device.RSSI))
		a.lastSyncItem.SetTitle("上次扫描：" + time.Now().Format("15:04:05"))
	}
}

func (a *App) handleAutoStart() {
	for range a.autoStart.ClickedCh {
		if a.autoStart.Checked() {
			a.autoStart.Uncheck()
			continue
		}

		a.autoStart.Check()
	}
}

func (a *App) handleQuit() {
	<-a.quitItem.ClickedCh
	systray.Quit()
}
