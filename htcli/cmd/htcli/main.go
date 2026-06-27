package main

import (
	"os"
	"strings"

	"github.com/u007/htcli/internal/commands"
	"github.com/u007/htcli/internal/host"
)

func main() {
	// Chrome passes the calling extension origin as the first argument
	// when spawning a native messaging host.
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "chrome-extension://") {
		if err := host.RunRelay(); err != nil {
			os.Exit(1)
		}
		return
	}
	commands.Execute()
}
