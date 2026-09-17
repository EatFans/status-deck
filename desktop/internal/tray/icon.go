//go:build !windows

package tray

import "status-deck/desktop/internal/appicon"

// statusDeckIconPNG generates the status-bar icon in memory. The package
// workflow reuses the same drawing code for the macOS and Windows app icons.
func statusDeckIconPNG() []byte {
	icon, err := appicon.PNG(18)
	if err != nil {
		return nil
	}
	return icon
}
