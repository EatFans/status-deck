//go:build darwin

package systeminfo

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
)

// pmset 输出示例中通常包含 "86%" 这样的电量百分比。
var pmsetPercentPattern = regexp.MustCompile(`(\d+)%`)

// collectPower 在 macOS 上通过 pmset 读取电池/电源状态。
//
// 选择 pmset 的原因是它是系统自带命令，不需要额外权限；
// 对状态栏应用来说也足够轻量。
func collectPower(ctx context.Context) Power {
	output, err := exec.CommandContext(ctx, "pmset", "-g", "batt").Output()
	if err != nil {
		return Power{Available: false}
	}

	text := string(output)
	percent := parseBatteryPercent(text)

	// pmset 的输出会根据系统版本、是否满电、是否接电源而变化。
	// 这里用字符串包含关系做宽松解析，优先保证“能展示大概状态”。
	charging := strings.Contains(text, "; charging;") ||
		strings.Contains(text, "; charged;") ||
		strings.Contains(text, "AC Power")
	onBattery := strings.Contains(text, "Battery Power") ||
		strings.Contains(text, "; discharging;")

	state := "unknown"
	switch {
	case strings.Contains(text, "; charged;"):
		state = "charged"
	case strings.Contains(text, "; charging;"):
		state = "charging"
	case strings.Contains(text, "; discharging;"):
		state = "discharging"
	case strings.Contains(text, "AC Power"):
		state = "ac_power"
	case strings.Contains(text, "Battery Power"):
		state = "battery_power"
	}

	source := "unknown"
	if strings.Contains(text, "AC Power") {
		source = "ac"
	}
	if strings.Contains(text, "Battery Power") {
		source = "battery"
	}

	return Power{
		Available: true,
		Percent:   percent,
		Charging:  &charging,
		OnBattery: &onBattery,
		State:     state,
		Source:    source,
	}
}

// parseBatteryPercent 从 pmset 输出里提取第一个百分比。
func parseBatteryPercent(text string) *int {
	matches := pmsetPercentPattern.FindStringSubmatch(text)
	if len(matches) != 2 {
		return nil
	}

	value := 0
	for _, digit := range matches[1] {
		value = value*10 + int(digit-'0')
	}

	return &value
}
