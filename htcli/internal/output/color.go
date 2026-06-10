package output

import (
	"os"

	"github.com/fatih/color"
)

var (
	// Success prints green text.
	Success = color.New(color.FgGreen).SprintFunc()
	// Error prints red text.
	Error = color.New(color.FgRed).SprintFunc()
	// Warning prints yellow text.
	Warning = color.New(color.FgYellow).SprintFunc()
	// Info prints cyan text.
	Info = color.New(color.FgCyan).SprintFunc()
	// Bold prints bold text.
	Bold = color.New(color.Bold).SprintFunc()
	// Dim prints dimmed text.
	Dim = color.New(color.Faint).SprintFunc()
)

func init() {
	// Respect NO_COLOR environment variable.
	if os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}
}
