//go:build !traytest

package tray

import "time"

// refreshInterval is the production refresh cadence for the status line.
func refreshInterval() time.Duration {
	return 5 * time.Second
}
