//go:build darwin

package tray

import (
	"fmt"
	"os/exec"
	"strings"
)

func openLogTerminal(logPath string) error {
	escapedPath := strings.ReplaceAll(logPath, `'`, `'\''`)
	command := fmt.Sprintf("clear; echo 'Status Deck 调试日志'; echo '%s'; echo; tail -f '%s'", escapedPath, escapedPath)

	script := fmt.Sprintf(`
tell application "Terminal"
	activate
	do script %q
end tell
`, command)

	return exec.Command("osascript", "-e", script).Run()
}
