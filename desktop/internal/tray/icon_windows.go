//go:build windows

package tray

import "status-deck/desktop/internal/appicon"

// statusDeckIconPNG keeps the platform-neutral call site while returning ICO
// bytes on Windows, as systray loads Windows tray icons through LoadImage.
func statusDeckIconPNG() []byte {
	icon, err := appicon.ICO()
	if err != nil {
		return nil
	}
	return icon
}
