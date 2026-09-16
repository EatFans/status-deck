//go:build !darwin

package tray

import "fmt"

func openLogTerminal(logPath string) error {
	return fmt.Errorf("当前平台暂未实现自动打开日志终端，日志文件：%s", logPath)
}

func closeLogTerminal() error {
	return fmt.Errorf("当前平台暂未实现自动关闭日志终端")
}
