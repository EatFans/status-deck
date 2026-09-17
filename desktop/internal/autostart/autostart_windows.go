//go:build windows

package autostart

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	windowsRunKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	windowsValueName = "Status Deck"
)

func enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open Windows startup registry key: %w", err)
	}
	defer key.Close()

	_, _, err = key.GetStringValue(windowsValueName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read Windows startup registry value: %w", err)
	}
	return true, nil
}

func enable() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, windowsRunKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create Windows startup registry key: %w", err)
	}
	defer key.Close()

	// The Run value is a command line, so quote the path for installations such
	// as C:\\Program Files\\Status Deck\\Status Deck.exe.
	if err := key.SetStringValue(windowsValueName, `"`+executable+`"`); err != nil {
		return fmt.Errorf("write Windows startup registry value: %w", err)
	}
	return nil
}

func disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.SET_VALUE)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Windows startup registry key: %w", err)
	}
	defer key.Close()

	err = key.DeleteValue(windowsValueName)
	if err == nil || err == registry.ErrNotExist {
		return nil
	}
	return fmt.Errorf("remove Windows startup registry value: %w", err)
}
