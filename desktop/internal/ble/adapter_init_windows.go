//go:build windows

package ble

import (
	"errors"

	"github.com/go-ole/go-ole"
	"tinygo.org/x/bluetooth"
)

const winRTAlreadyInitialized = uintptr(1) // S_FALSE

func enableAdapter(adapter *bluetooth.Adapter) error {
	err := adapter.Enable()
	if err == nil {
		return nil
	}

	// tinygo.org/x/bluetooth v0.16.0 forwards WinRT RoInitialize directly.
	// RoInitialize returns S_FALSE when WinRT was already initialized on this
	// thread. That is a successful, usable state, not an adapter failure.
	var oleError *ole.OleError
	if errors.As(err, &oleError) && oleError.Code() == winRTAlreadyInitialized {
		return nil
	}
	return err
}
