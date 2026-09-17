//go:build !darwin && !windows

package autostart

import "fmt"

func enabled() (bool, error) {
	return false, fmt.Errorf("当前平台暂未实现开机自启")
}

func enable() error {
	return fmt.Errorf("当前平台暂未实现开机自启")
}

func disable() error {
	return fmt.Errorf("当前平台暂未实现开机自启")
}
