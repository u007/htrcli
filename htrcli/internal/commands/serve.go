package commands

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/host"
	"github.com/u007/htrcli/internal/tray"
)

var serveNoTray bool

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the htrcli daemon (native messaging host + HTTP :3845)",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		socketPath := home + host.DefaultSocketPath

		bearerToken := resolveBearerToken()
		port := 3845
		if p := os.Getenv("HTR_PORT"); p != "" {
			fmt.Sscanf(p, "%d", &port)
		}

		d := host.NewDaemon()

		srv := host.NewHTTPServer(d, port, bearerToken, defaultAllowedIPs())
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return fmt.Errorf("port %d already in use (another htrcli serve or the Bun server running?): %w", port, err)
		}

		// Start Unix socket server (for relay connections from Chrome).
		// The listener is owned here so it can be closed during shutdown.
		unixLn, err := host.StartUnixSocketServer(d, socketPath, port, bearerToken)
		if err != nil {
			return fmt.Errorf("unix socket server: %w", err)
		}

		// Reap stale relays: ping each connection periodically and drop any
		// that stay silent. Lifecycle owned by the daemon (see d.Stop).
		go d.StartSweeper(host.PingInterval, host.StaleAfter)

		fmt.Printf("[htrcli serve] Listening on http://127.0.0.1:%d\n", port)
		fmt.Printf("[htrcli serve] Unix socket: %s\n", socketPath)
		if bearerToken == "" {
			fmt.Println("[htrcli serve] Warning: no bearer token configured — unauthenticated")
			fmt.Println("  Set HTR_BEARER_TOKEN env var, or run: htrcli config set-token <token>")
		} else {
			// Never log the full bearer token — only a fingerprint.
			log.Printf("[htrcli serve] Using bearer token: %s", tray.Fingerprint(bearerToken))
		}

		// --- Tray detection -------------------------------------------------
		trayAttached := tray.ShouldStart(serveNoTray)
		if trayAttached {
			log.Printf("[htrcli serve] Tray icon enabled")
		} else {
			log.Printf("[htrcli serve] Tray disabled (no display or HTRCLI_NO_TRAY set)")
		}

		// Resolvers shared by the tray controller (env > file > viper).
		getToken := func() string { return resolveBearerToken() }
		getExtID := func(browser string) string {
			if browser != "" {
				if v := viper.GetString("extension-id." + browser); v != "" {
					return v
				}
			}
			return viper.GetString("extension-id")
		}

		// --- Shutdown sequencing -------------------------------------------
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		done := make(chan struct{})
		var logCloser func() error
		var shutdownOnce sync.Once
		performShutdown := func() {
			shutdownOnce.Do(func() {
				// 1. Quit the tray (idempotent). Routed through the tray
				//    package so serve.go never imports getlantern/systray.
				if trayAttached {
					tray.Quit()
				}
				// 2. Drain HTTP (5s timeout).
				sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(sctx); err != nil {
					log.Printf("[htrcli serve] HTTP shutdown: %v", err)
				}
				// 3. Stop the daemon (sweeper, etc.).
				d.Stop()
				// 4. Close the Unix-socket listener.
				if unixLn != nil {
					_ = unixLn.Close()
				}
				// 5. Flush/close the tray log file if attached.
				if logCloser != nil {
					_ = logCloser()
				}
				close(done)
			})
		}

		// Signal handler goroutine.
		go func() {
			<-sigCh
			performShutdown()
		}()

		// HTTP server goroutine.
		go func() {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Printf("[htrcli serve] HTTP server error: %v", err)
			}
		}()

		if trayAttached {
			// Redirect daemon logs to ~/.htrcli/serve.log (in addition to
			// stderr) so the tray's "Show recent log" menu has something to
			// open. Server runs without a tray are unaffected.
			if closer, lerr := attachServeLog(); lerr != nil {
				log.Printf("[htrcli serve] tray log redirect: %v (continuing without)", lerr)
			} else {
				logCloser = closer
			}

			selfPath, _ := os.Executable()
			ctrl := tray.NewDaemonController(d, port, getToken, getExtID, selfPath, ln, tray.RealCommander{})
			ctrl.SetQuitFn(func() {
				// Reuse the signal-shutdown path.
				sigCh <- syscall.SIGTERM
			})

			// tray.Run blocks the main goroutine (required by systray on
			// macOS/Windows) until tray.Quit() is called — which happens
			// via Quit menu → SetQuitFn → SIGTERM → performShutdown.
			tray.Run(ctrl, trayIcon)
			<-done
			return nil
		}

		// No tray: block until shutdown completes.
		<-done
		return nil
	},
}

// resolveBearerToken resolves the bearer token with priority
// env > token file > viper config.
func resolveBearerToken() string {
	if t := os.Getenv("HTR_BEARER_TOKEN"); t != "" {
		return t
	}
	if t := readBearerTokenFile(); t != "" {
		return t
	}
	return viper.GetString("token")
}

// attachServeLog redirects the standard logger to also write to
// ~/.htrcli/serve.log (keeping stderr for journald/launchd). Returns a closer
// for the log file.
func attachServeLog() (func() error, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".htrcli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	logPath := filepath.Join(dir, "serve.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return f.Close, nil
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
// extension's Options page) is picked up by `make serve` and `htrcli serve`.
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
	serveCmd.Flags().BoolVar(&serveNoTray, "no-tray", false, "Disable the system-tray icon (auto-skipped on headless Linux; set HTRCLI_NO_TRAY=1 to force)")
	rootCmd.AddCommand(serveCmd)
}
