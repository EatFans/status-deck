//go:build darwin

package systeminfo

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
)

var pmsetPercentPattern = regexp.MustCompile(`(\d+)%`)

func collectPower(ctx context.Context) Power {
	output, err := exec.CommandContext(ctx, "pmset", "-g", "batt").Output()
	if err != nil {
		return Power{Available: false}
	}

	text := string(output)
	percent := parseBatteryPercent(text)
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
