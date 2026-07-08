package commands

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htcli/internal/host"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the htcli daemon (native messaging host + HTTP :3845)",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		socketPath := home + host.DefaultSocketPath

		bearerToken := os.Getenv("HTR_BEARER_TOKEN")
		if bearerToken == "" {
			if fileToken := readBearerTokenFile(); fileToken != "" {
				bearerToken = fileToken
			} else {
				bearerToken = viper.GetString("token")
			}
		}
		port := 3845
		if p := os.Getenv("HTR_PORT"); p != "" {
			fmt.Sscanf(p, "%d", &port)
		}

		d := host.NewDaemon()

		// Start Unix socket server (for relay connections from Chrome)
		go func() {
			if err := host.StartUnixSocketServer(d, socketPath); err != nil {
				log.Printf("[htcli serve] Unix socket error: %v", err)
			}
		}()

		// Start HTTP server
		srv := host.NewHTTPServer(d, port, bearerToken, defaultAllowedIPs())
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return fmt.Errorf("port %d already in use (Bun server running?): %w", port, err)
		}

		fmt.Printf("[htcli serve] Listening on http://127.0.0.1:%d\n", port)
		fmt.Printf("[htcli serve] Unix socket: %s\n", socketPath)
		if bearerToken == "" {
			fmt.Println("[htcli serve] Warning: no bearer token configured — unauthenticated")
			fmt.Println("  Set HTR_BEARER_TOKEN env var, or run: htcli config set-token <token>")
		} else {
			fmt.Printf("[htcli serve] Using bearer token: %s\n", bearerToken)
		}

		return srv.Serve(ln)
	},
}

func defaultAllowedIPs() []string {
	if v := os.Getenv("HTR_ALLOWED_IPS"); v != "" {
		var ips []string
		for _, ip := range splitComma(v) {
			if ip != "" {
				ips = append(ips, ip)
			}
		}
		return ips
	}
	return []string{"127.0.0.1", "::1", "localhost"}
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// readBearerTokenFile reads a token from a file path. Checks
// HTR_BEARER_TOKEN_FILE first, then $XDG_CONFIG_HOME/htrcontrol/token, then
// ~/.config/htrcontrol/token, then ~/.htrcontrol/token. Mirrors the Bun
// server's readTokenFile so the same per-install token (shown in the
// extension's Options page) is picked up by `make serve` and `htcli serve`.
//
// Returns "" if no readable file is found or all candidates are empty.
func readBearerTokenFile() string {
	var candidates []string
	if v := os.Getenv("HTR_BEARER_TOKEN_FILE"); v != "" {
		candidates = append(candidates, v)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, "htrcontrol", "token"))
		}
		candidates = append(candidates, filepath.Join(home, ".config", "htrcontrol", "token"))
		candidates = append(candidates, filepath.Join(home, ".htrcontrol", "token"))
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if text != "" {
			return text
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
