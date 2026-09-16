//go:build darwin

package tray

import (
	"fmt"
	"os/exec"
	"strings"
)

const logTerminalTitle = "Status Deck 调试日志"

func openLogTerminal(logPath string) error {
	escapedPath := strings.ReplaceAll(logPath, `'`, `'\''`)
	command := fmt.Sprintf("printf '\\033]0;%s\\007'; clear; echo '%s'; echo '%s'; echo; tail -f '%s'", logTerminalTitle, logTerminalTitle, escapedPath, escapedPath)

	script := fmt.Sprintf(`
tell application "Terminal"
	activate
	do script %q
end tell
`, command)

	return exec.Command("osascript", "-e", script).Run()
}

func closeLogTerminal() error {
	script := fmt.Sprintf(`
tell application "Terminal"
	repeat with w in windows
		repeat with t in tabs of w
			if custom title of t is %q then
				close w
				return
			end if
		end repeat
	end repeat
end tell
`, logTerminalTitle)

	return exec.Command("osascript", "-e", script).Run()
}
