package tray

import (
	"context"
	"fmt"
	"time"

	"github.com/getlantern/systray"

	"status-deck/desktop/internal/applog"
	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/config"
)

type App struct {
	config config.Config
	client *ble.Client

	// 下方菜单项需要在扫描/连接状态变化时更新标题，因此保存在 App 里。
	statusItem   *systray.MenuItem
	lastSyncItem *systray.MenuItem
	scanItem     *systray.MenuItem
	autoStart    *systray.MenuItem
	debugLogs    *systray.MenuItem
	quitItem     *systray.MenuItem
}

func NewApp(config config.Config) *App {
	// 状态栏应用内部持有一个 BLE Client。
	// 现在只用于扫描，后续会扩展为自动连接和持续写入状态数据。
	return &App{
		config: config,
		client: ble.NewClient(config.BLE),
	}
}

func (a *App) Run() error {
	if err := applog.Init(); err != nil {
		return err
	}
	applog.Println("状态栏应用启动")

	// systray.Run 会阻塞当前 goroutine，直到用户点击“退出”。
	// onReady 在状态栏运行时回调里创建菜单，onExit 用于释放资源。
	systray.Run(a.onReady, a.onExit)
	return nil
}

func (a *App) onReady() {
	// 使用内置 PNG 图标。SetTitle 置空，尽量让状态栏只显示图标。
	systray.SetIcon(statusDeckIconPNG())
	systray.SetTitle("")
	systray.SetTooltip("Status Deck 状态卡")

	// 连接状态和最近同步时间是只读信息项。
	a.statusItem = systray.AddMenuItem("未连接", "当前设备连接状态")
	a.statusItem.Disable()

	a.lastSyncItem = systray.AddMenuItem("上次同步：从未", "最近一次成功同步时间")
	a.lastSyncItem.Disable()

	systray.AddSeparator()

	// 设备管理菜单先提供扫描入口。
	// 后续会把扫描结果扩展成可点击设备列表，并支持连接/断开/忘记设备。
	deviceManager := systray.AddMenuItem("设备管理", "扫描和管理 Status Deck 设备")
	a.scanItem = deviceManager.AddSubMenuItem("扫描设备", "扫描附近的 Status Deck 设备")
	deviceStatus := deviceManager.AddSubMenuItem("未选择设备", "当前选择的设备")
	deviceStatus.Disable()

	// 设置菜单先放轻量选项。
	// 开机自启当前只是 UI 占位，还没有写入 LaunchAgent/Windows Startup。
	settings := systray.AddMenuItem("设置", "打开 Status Deck 设置")
	a.autoStart = settings.AddSubMenuItemCheckbox("开机自启", "电脑启动时自动启动 Status Deck", false)
	a.debugLogs = settings.AddSubMenuItemCheckbox("调试日志", "打开系统终端查看实时日志", false)

	systray.AddSeparator()

	a.quitItem = systray.AddMenuItem("退出", "退出 Status Deck")

	// systray 菜单点击通过 channel 通知。
	// 每类交互放一个 goroutine，避免阻塞状态栏主循环。
	go a.handleScan(deviceStatus)
	go a.handleAutoStart()
	go a.handleDebugLogs()
	go a.handleQuit()
}

func (a *App) onExit() {
	// 退出时关闭 BLE 客户端。现在 Close 还没有实质逻辑，
	// 等连接功能完成后这里会负责断开设备和停止扫描。
	_ = a.client.Close()
	applog.Println("状态栏应用退出")
	_ = applog.Close()
}

func (a *App) handleScan(deviceStatus *systray.MenuItem) {
	for range a.scanItem.ClickedCh {
		applog.Println("开始扫描 Status Deck 设备")

		// 点击“扫描设备”后，临时禁用菜单项，避免重复扫描并发执行。
		a.statusItem.SetTitle("正在扫描...")
		a.scanItem.Disable()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		devices, err := a.client.ScanWithOptions(ctx, ble.ScanOptions{
			Timeout: 10 * time.Second,
		})
		cancel()

		a.scanItem.Enable()

		if err != nil {
			// 扫描失败通常和系统蓝牙权限、蓝牙关闭、底层库错误有关。
			applog.Printf("扫描失败：%v", err)
			a.statusItem.SetTitle("扫描失败")
			deviceStatus.SetTitle(fmt.Sprintf("错误：%v", err))
			continue
		}

		if len(devices) == 0 {
			// 没发现设备不一定是错误：可能 ESP32 还没烧录、没上电、没广播。
			applog.Println("扫描完成：未发现 Status Deck 设备")
			a.statusItem.SetTitle("未连接")
			deviceStatus.SetTitle("未发现 Status Deck 设备")
			continue
		}

		device := devices[0]
		name := device.Name
		if name == "" {
			// 有些平台扫到的设备没有广播名，用地址兜底展示。
			name = device.ID
		}

		a.statusItem.SetTitle("已发现：" + name)
		deviceStatus.SetTitle(fmt.Sprintf("%s RSSI=%d", name, device.RSSI))
		a.lastSyncItem.SetTitle("上次扫描：" + time.Now().Format("15:04:05"))
		applog.Printf("扫描完成：发现设备 name=%s id=%s rssi=%d", name, device.ID, device.RSSI)
	}
}

func (a *App) handleAutoStart() {
	for range a.autoStart.ClickedCh {
		// 当前只是菜单 UI 行为：点击后切换勾选状态。
		// 后续要在这里接 macOS LaunchAgent / Windows Startup。
		if a.autoStart.Checked() {
			a.autoStart.Uncheck()
			applog.Println("开机自启：关闭")
			continue
		}

		a.autoStart.Check()
		applog.Println("开机自启：开启（当前仅 UI 占位）")
	}
}

func (a *App) handleDebugLogs() {
	for range a.debugLogs.ClickedCh {
		if a.debugLogs.Checked() {
			a.debugLogs.Uncheck()
			applog.Println("关闭调试日志终端")

			if err := closeLogTerminal(); err != nil {
				applog.Printf("关闭调试日志终端失败：%v", err)
			}
			continue
		}

		a.debugLogs.Check()
		path := applog.Path()
		applog.Printf("打开调试日志终端：%s", path)

		if err := openLogTerminal(path); err != nil {
			applog.Printf("打开调试日志终端失败：%v", err)
			a.debugLogs.Uncheck()
		}
	}
}

func (a *App) handleQuit() {
	// 退出入口必须简单可靠：收到点击后让 systray 退出主循环。
	<-a.quitItem.ClickedCh
	applog.Println("用户点击退出")
	systray.Quit()
}
