package host

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
)

const DefaultSocketPath = "/.htrcli/daemon.sock"

// SocketPath resolves the daemon socket path: HTR_SOCKET_PATH env var,
// falling back to home+DefaultSocketPath.
func SocketPath() (string, error) {
	if p := os.Getenv("HTR_SOCKET_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	if runtime.GOOS == "windows" {
		return "127.0.0.1:3847", nil
	}
	return home + DefaultSocketPath, nil
}

// RunRelay is the entry point when Chrome spawns htrcli as a native host.
// It connects to the daemon Unix socket and bridges stdin/stdout to it.
func RunRelay() error {
	socketPath, err := SocketPath()
	if err != nil {
		return err
	}
	return RunRelayWithIO(os.Stdin, os.Stdout, socketPath)
}

// RunRelayWithIO is the testable core of RunRelay.
func RunRelayWithIO(stdin io.Reader, stdout io.Writer, socketPath string) error {
	network := "unix"
	address := socketPath
	if runtime.GOOS == "windows" || strings.HasPrefix(socketPath, "tcp://") {
		network = "tcp"
		address = strings.TrimPrefix(socketPath, "tcp://")
	}
	conn, err := net.Dial(network, address)
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
