// Package autostart manages whether the desktop application starts at login.
package autostart

// Enabled reports whether automatic startup is enabled for the current user.
func Enabled() (bool, error) { return enabled() }

// Enable configures the current executable to start when the user logs in.
func Enable() error { return enable() }

// Disable removes the current user's automatic startup configuration.
func Disable() error { return disable() }
