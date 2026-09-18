package tray

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/getlantern/systray"

	desktopapp "status-deck/desktop/internal/app"
	"status-deck/desktop/internal/applog"
	"status-deck/desktop/internal/autostart"
	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/config"
)

type App struct {
	agent *desktopapp.Agent

	// lifecycleCtx 在托盘退出时取消，自动重连与心跳 goroutine 随之停止。
	lifecycleCtx    context.Context
	cancelLifecycle context.CancelFunc

	// 下方菜单项需要在扫描/连接状态变化时更新标题，因此保存在 App 里。
	statusItem   *systray.MenuItem
	lastSyncItem *systray.MenuItem
	scanItem     *systray.MenuItem
	currentPage  *systray.MenuItem
	previousPage *systray.MenuItem
	nextPage     *systray.MenuItem
	autoStart    *systray.MenuItem
	debugLogs    *systray.MenuItem
	quitItem     *systray.MenuItem
}

func NewApp(config config.Config) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		agent:           desktopapp.New(config),
		lifecycleCtx:    ctx,
		cancelLifecycle: cancel,
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
	a.currentPage = deviceManager.AddSubMenuItem("当前页面：等待设备响应", "设备当前显示的页面")
	a.currentPage.Disable()
	a.previousPage = deviceManager.AddSubMenuItem("上一页", "切换设备到上一页")
	a.previousPage.Disable()
	a.nextPage = deviceManager.AddSubMenuItem("下一页", "切换设备到下一页")
	a.nextPage.Disable()

	// 设置菜单先放轻量选项。
	settings := systray.AddMenuItem("设置", "打开 Status Deck 设置")
	a.autoStart = settings.AddSubMenuItemCheckbox("开机自启", "电脑启动时自动启动 Status Deck", false)
	a.debugLogs = settings.AddSubMenuItemCheckbox("调试日志", "打开系统终端查看实时日志", false)
	if enabled, err := autostart.Enabled(); err != nil {
		applog.Printf("读取开机自启状态失败：%v", err)
	} else if enabled {
		a.autoStart.Check()
	}

	systray.AddSeparator()

	a.quitItem = systray.AddMenuItem("退出", "退出 Status Deck")

	// 应用一启动就开始主动寻找 Status Deck。ESP32 只负责广播，连接和断线重连
	// 必须由电脑端这个 Central 发起；这里不需要用户手动点“扫描设备”。
	a.agent.Start(a.lifecycleCtx)

	// systray 菜单点击通过 channel 通知。
	// 每类交互放一个 goroutine，避免阻塞状态栏主循环。
	go a.handleScan(deviceStatus)
	go a.handleAutoStart()
	go a.handleDebugLogs()
	go a.handleQuit()
	go a.handlePageNavigation()
	go a.handleBLEEvents(deviceStatus)
}

func (a *App) onExit() {
	// 先取消后台维护循环，再主动断开 BLE，避免应用退出后继续扫描或写心跳。
	a.cancelLifecycle()
	_ = a.agent.Close()
	applog.Println("状态栏应用退出")
	_ = applog.Close()
}

// handleBLEEvents 把 BLE 后台连接状态映射为状态栏文字，同时将设备的 notify
// 写进应用日志。后续显示屏按键、ACK 和设备错误也都会从这里进入桌面端。
func (a *App) handleBLEEvents(deviceStatus *systray.MenuItem) {
	for event := range a.agent.Events() {
		switch event.Type {
		case ble.EventConnecting:
			a.statusItem.SetTitle("正在连接...")
		case ble.EventConnected:
			name := event.Device.Name
			if name == "" {
				name = event.Device.ID
			}
			a.statusItem.SetTitle("已连接：" + name)
			deviceStatus.SetTitle(fmt.Sprintf("%s RSSI=%d", name, event.Device.RSSI))
			a.lastSyncItem.SetTitle("已建立实时连接")
			applog.Printf("BLE 已连接：name=%s id=%s", name, event.Device.ID)
			a.currentPage.SetTitle("当前页面：正在读取")
		case ble.EventDisconnected:
			a.statusItem.SetTitle("连接已断开，正在重连...")
			a.currentPage.SetTitle("当前页面：--")
			a.previousPage.Disable()
			a.nextPage.Disable()
			applog.Printf("BLE 已断开：id=%s", event.Device.ID)
		case ble.EventConnectFailed:
			// 没开机或暂时不在附近是常态，因此 UI 保持安静，日志保留具体原因。
			a.statusItem.SetTitle("未连接，正在寻找设备...")
			applog.Printf("BLE 连接尝试失败：%v", event.Err)
		case ble.EventNotification:
			a.lastSyncItem.SetTitle("设备响应：" + time.Now().Format("15:04:05"))
			// 正常协议消息是 UTF-8 JSON。使用 %q 可把不可见字符明确转义；
			// 若不是合法 UTF-8，同时记录 hex，便于排查 GATT 特征值或串口固件问题。
			if utf8.Valid(event.Message) {
				applog.Printf("BLE TX notify：bytes=%d text=%q", len(event.Message), string(event.Message))
			} else {
				applog.Printf("BLE TX notify 异常字节：bytes=%d hex=%x", len(event.Message), event.Message)
			}
		case ble.EventPageChanged:
			a.currentPage.SetTitle(fmt.Sprintf("当前页面：%d %s", event.Page.Index, event.Page.Label()))
			a.previousPage.Enable()
			a.nextPage.Enable()
			a.lastSyncItem.SetTitle("页面已切换：" + time.Now().Format("15:04:05"))
			applog.Printf("设备页面：index=%d id=%s count=%d", event.Page.Index, event.Page.ID, event.Page.Count)
			// 设备确认页面后再同步。同步器会只采集并发送此页所属的数据域。
			a.agent.SyncNow(a.lifecycleCtx)
		}
	}
}

func (a *App) handlePageNavigation() {
	for {
		select {
		case <-a.previousPage.ClickedCh:
			a.requestPageChange("上一页", a.agent.PreviousPage)
		case <-a.nextPage.ClickedCh:
			a.requestPageChange("下一页", a.agent.NextPage)
		}
	}
}

func (a *App) requestPageChange(label string, command func(context.Context) error) {
	a.previousPage.Disable()
	a.nextPage.Disable()
	ctx, cancel := context.WithTimeout(a.lifecycleCtx, 5*time.Second)
	err := command(ctx)
	cancel()
	if err != nil {
		applog.Printf("请求%s失败：%v", label, err)
		if a.agent.IsConnected() {
			a.previousPage.Enable()
			a.nextPage.Enable()
		}
		return
	}
	applog.Printf("已请求设备切换%s，等待 page.status 确认", label)
}

func (a *App) handleScan(deviceStatus *systray.MenuItem) {
	for range a.scanItem.ClickedCh {
		// 当前 MVP 只维护一台设备。ESP32 被连接后会停止广播，继续扫描既找不到
		// 当前设备，也会让用户误以为连接失败，因此直接提示当前状态。
		if a.agent.IsConnected() {
			a.statusItem.SetTitle("设备已连接")
			deviceStatus.SetTitle("当前设备已连接")
			applog.Println("跳过扫描：Status Deck 当前已连接")
			continue
		}

		applog.Println("开始扫描 Status Deck 设备")

		// 点击“扫描设备”后，临时禁用菜单项，避免重复扫描并发执行。
		a.statusItem.SetTitle("正在扫描...")
		a.scanItem.Disable()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		devices, err := a.agent.ScanWithOptions(ctx, ble.ScanOptions{
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
		if a.autoStart.Checked() {
			if err := autostart.Disable(); err != nil {
				applog.Printf("关闭开机自启失败：%v", err)
				continue
			}
			a.autoStart.Uncheck()
			applog.Println("开机自启已关闭")
			continue
		}

		if err := autostart.Enable(); err != nil {
			applog.Printf("开启开机自启失败：%v", err)
			continue
		}
		a.autoStart.Check()
		applog.Println("开机自启已开启，将在下次登录时启动")
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
