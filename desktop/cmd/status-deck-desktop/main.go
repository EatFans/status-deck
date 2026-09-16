package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"status-deck/desktop/internal/ble"
	"status-deck/desktop/internal/collectors"
	"status-deck/desktop/internal/config"
	"status-deck/desktop/internal/protocol"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "run":
		if err := run(); err != nil {
			fmt.Fprintf(os.Stderr, "status-deck-desktop: %v\n", err)
			os.Exit(1)
		}
	case "scan":
		if err := scan(); err != nil {
			fmt.Fprintf(os.Stderr, "status-deck-desktop: %v\n", err)
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
	fmt.Println("  status-deck-desktop run      Start the desktop sync loop")
	fmt.Println("  status-deck-desktop scan     Scan for Status Deck devices")
	fmt.Println("  status-deck-desktop version  Print version")
}

func run() error {
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
	ctx, stop := signalContext()
	defer stop()

	cfg := config.Default()
	client := ble.NewClient(cfg.BLE)
	devices, err := client.Scan(ctx, 5*time.Second)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		fmt.Println("No Status Deck devices found")
		return nil
	}

	for _, device := range devices {
		fmt.Printf("%s %s RSSI=%d\n", device.ID, device.Name, device.RSSI)
	}
	return nil
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	return ctx, cancel
}
