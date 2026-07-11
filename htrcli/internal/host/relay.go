package host

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
)

const DefaultSocketPath = "/.htrcli/daemon.sock"

// RunRelay is the entry point when Chrome spawns htrcli as a native host.
// It connects to the daemon Unix socket and bridges stdin/stdout to it.
func RunRelay() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	return RunRelayWithIO(os.Stdin, os.Stdout, home+DefaultSocketPath)
}

// RunRelayWithIO is the testable core of RunRelay.
func RunRelayWithIO(stdin io.Reader, stdout io.Writer, socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		writeErrorToChrome(stdout, "daemon not running: "+err.Error())
		return fmt.Errorf("dial daemon: %w", err)
	}
	defer conn.Close()

	errc := make(chan error, 2)

	// stdin → socket
	go func() {
		for {
			msg, err := ReadMessage(stdin)
			if err != nil {
				errc <- err
				return
			}
			if err := WriteMessage(conn, msg); err != nil {
				errc <- err
				return
			}
		}
	}()

	// socket → stdout
	go func() {
		for {
			msg, err := ReadMessage(conn)
			if err != nil {
				errc <- err
				return
			}
			if err := WriteMessage(stdout, msg); err != nil {
				errc <- err
				return
			}
		}
	}()

	<-errc
	return nil
}

func writeErrorToChrome(w io.Writer, msg string) {
	data, _ := json.Marshal(map[string]string{"type": "error", "error": msg})
	WriteMessage(w, data) //nolint:errcheck
}
