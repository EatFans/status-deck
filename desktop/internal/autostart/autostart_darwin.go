//go:build darwin

package autostart

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

const launchAgentLabel = "com.statusdeck.desktop"

func enabled() (bool, error) {
	_, err := os.Stat(launchAgentPath())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("read LaunchAgent: %w", err)
}

func enable() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve executable symlink: %w", err)
	}

	path := launchAgentPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}

	content, err := launchAgentContent(executable)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o644); err != nil {
		return fmt.Errorf("write LaunchAgent: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("install LaunchAgent: %w", err)
	}
	return nil
}

func disable() error {
	err := os.Remove(launchAgentPath())
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove LaunchAgent: %w", err)
}

func launchAgentPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// os.UserHomeDir only fails in unusual, broken login environments. Keeping
		// the path under a relative directory is safer than targeting a broad path.
		return filepath.Join("Library", "LaunchAgents", launchAgentLabel+".plist")
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func launchAgentContent(executable string) ([]byte, error) {
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(executable)); err != nil {
		return nil, fmt.Errorf("escape executable path: %w", err)
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`, launchAgentLabel, escaped.String())), nil
}
