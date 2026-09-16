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
	systray.SetTitle("Status Deck")
	systray.SetTooltip("Status Deck")

	a.statusItem = systray.AddMenuItem("Disconnected", "Current device connection status")
	a.statusItem.Disable()

	a.lastSyncItem = systray.AddMenuItem("Last Sync: Never", "Last successful sync time")
	a.lastSyncItem.Disable()

	systray.AddSeparator()

	deviceManager := systray.AddMenuItem("Device Manager", "Scan and manage Status Deck devices")
	a.scanItem = deviceManager.AddSubMenuItem("Scan Devices", "Scan nearby Status Deck devices")
	deviceStatus := deviceManager.AddSubMenuItem("No device selected", "Current selected device")
	deviceStatus.Disable()

	settings := systray.AddMenuItem("Settings", "Open Status Deck settings")
	a.autoStart = settings.AddSubMenuItemCheckbox("Launch at Login", "Start Status Deck when the computer starts", false)
	debugLogs := settings.AddSubMenuItemCheckbox("Debug Logs", "Print verbose logs while developing", false)
	debugLogs.Disable()

	systray.AddSeparator()

	a.quitItem = systray.AddMenuItem("Quit", "Quit Status Deck")

	go a.handleScan(deviceStatus)
	go a.handleAutoStart()
	go a.handleQuit()
}

func (a *App) onExit() {
	_ = a.client.Close()
}

func (a *App) handleScan(deviceStatus *systray.MenuItem) {
	for range a.scanItem.ClickedCh {
		a.statusItem.SetTitle("Scanning...")
		a.scanItem.Disable()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		devices, err := a.client.ScanWithOptions(ctx, ble.ScanOptions{
			Timeout: 10 * time.Second,
		})
		cancel()

		a.scanItem.Enable()

		if err != nil {
			a.statusItem.SetTitle("Scan failed")
			deviceStatus.SetTitle(fmt.Sprintf("Error: %v", err))
			continue
		}

		if len(devices) == 0 {
			a.statusItem.SetTitle("Disconnected")
			deviceStatus.SetTitle("No Status Deck device found")
			continue
		}

		device := devices[0]
		name := device.Name
		if name == "" {
			name = device.ID
		}

		a.statusItem.SetTitle("Found: " + name)
		deviceStatus.SetTitle(fmt.Sprintf("%s RSSI=%d", name, device.RSSI))
		a.lastSyncItem.SetTitle("Last Scan: " + time.Now().Format("15:04:05"))
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
