//go:build !darwin

package systeminfo

import "context"

func collectPower(ctx context.Context) Power {
	return Power{Available: false}
}
