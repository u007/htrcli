package commands

import (
	"fmt"
	"log"
	"net"
	"os"

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
			bearerToken = viper.GetString("token")
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

func init() {
	rootCmd.AddCommand(serveCmd)
}
