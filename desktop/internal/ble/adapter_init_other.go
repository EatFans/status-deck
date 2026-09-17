//go:build !windows

package ble

import "tinygo.org/x/bluetooth"

func enableAdapter(adapter *bluetooth.Adapter) error {
	return adapter.Enable()
}
