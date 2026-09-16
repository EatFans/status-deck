package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/collectors"
	"status-deck/desktop/internal/config"
	"status-deck/desktop/internal/protocol"
	trayapp "status-deck/desktop/internal/tray"
)

const version = "0.1.0-dev"

func main() {
	// 用户双击应用或直接执行 `status-deck` 时，不进入传统 CLI，
	// 而是直接启动状态栏/托盘应用。这是桌面端的默认产品形态。
	if len(os.Args) < 2 {
		if err := tray(); err != nil {
			fmt.Fprintf(os.Stderr, "status-deck: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 这些子命令主要用于开发、调试和后续自动化。
	// 普通用户正常情况下只需要启动状态栏应用。
	switch os.Args[1] {
	case "run":
		if err := run(); err != nil {
			fmt.Fprintf(os.Stderr, "status-deck: %v\n", err)
			os.Exit(1)
		}
	case "tray":
		if err := tray(); err != nil {
			fmt.Fprintf(os.Stderr, "status-deck: %v\n", err)
			os.Exit(1)
		}
	case "scan":
		if err := scan(); err != nil {
			fmt.Fprintf(os.Stderr, "status-deck: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Status Deck Desktop")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  status-deck          Start the status bar app")
	fmt.Println("  status-deck version  Print version")
	fmt.Println()
	fmt.Println("Development:")
	fmt.Println("  status-deck tray     Start the status bar app")
	fmt.Println("  status-deck run      Start the desktop sync loop")
	fmt.Println("  status-deck scan     Scan for Status Deck devices")
	fmt.Println("  status-deck scan --all --timeout 10s")
}

func run() error {
	// run 是无界面的同步循环，主要用于调试 BLE 连接和数据发送。
	// 后续状态栏应用内部也会复用类似的同步逻辑。
	ctx, stop := signalContext()
	defer stop()

	cfg := config.Default()
	client := ble.NewClient(cfg.BLE)
	collector := collectors.NewSystemCollector()

	fmt.Printf("Starting Status Deck Desktop %s\n", version)
	fmt.Printf("Target BLE service: %s\n", cfg.BLE.ServiceUUID)

	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.Close()

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Shutting down")
			return nil
		case <-ticker.C:
			// 当前还是旧的占位采集器。新的内存/磁盘/电源采集模块在
			// internal/systeminfo 中，暂时没有接入这里。
			snapshot, err := collector.Collect(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "collect status: %v\n", err)
				continue
			}

			payload := protocol.NewStatusPayload(snapshot)
			data, err := protocol.Encode(payload)
			if err != nil {
				fmt.Fprintf(os.Stderr, "encode payload: %v\n", err)
				continue
			}

			if err := client.Write(ctx, data); err != nil {
				fmt.Fprintf(os.Stderr, "write BLE payload: %v\n", err)
				continue
			}
		}
	}
}

func scan() error {
	// scan 用来单独验证电脑端 BLE 扫描能力。
	// 常用排查命令：
	//   status-deck scan --all --timeout 10s
	// 如果能扫到耳机、键盘等设备，说明电脑端蓝牙权限和库基本正常。
	ctx, stop := signalContext()
	defer stop()

	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	includeAll := flags.Bool("all", false, "show all nearby BLE devices instead of only Status Deck")
	includeUnnamed := flags.Bool("unnamed", false, "include unnamed devices when used with --all")
	timeout := flags.Duration("timeout", 5*time.Second, "scan timeout")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}

	cfg := config.Default()
	client := ble.NewClient(cfg.BLE)
	devices, err := client.ScanWithOptions(ctx, ble.ScanOptions{
		Timeout:        *timeout,
		IncludeAll:     *includeAll,
		IncludeUnnamed: *includeUnnamed,
	})
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		if *includeAll {
			fmt.Println("No BLE devices found")
		} else {
			fmt.Println("No Status Deck devices found")
			fmt.Println("Tip: run `status-deck scan --all --timeout 10s` to check whether Bluetooth scanning works.")
		}
		return nil
	}

	for _, device := range devices {
		fmt.Printf("%s %s RSSI=%d\n", device.ID, device.Name, device.RSSI)
	}
	return nil
}

func tray() error {
	// tray 是真正的桌面端入口：创建状态栏图标和菜单。
	app := trayapp.NewApp(config.Default())
	return app.Run()
}

func signalContext() (context.Context, context.CancelFunc) {
	// 统一处理 Ctrl+C 和系统终止信号，保证调试命令退出时能释放资源。
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	return ctx, cancel
}
