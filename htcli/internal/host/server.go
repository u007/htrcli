package host

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// NewHTTPServer builds the HTTP server with all API routes.
// bearerToken: if non-empty, all requests must supply "Authorization: Bearer <token>".
// allowedIPs: if non-nil and non-empty, requests from other IPs are rejected.
func NewHTTPServer(d *Daemon, port int, bearerToken string, allowedIPs []string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/api/", authMiddleware(bearerToken, allowedIPs, apiHandler(d, port, bearerToken)))

	return &http.Server{
		Addr:         addrFromPort(port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
}

func addrFromPort(port int) string {
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func authMiddleware(token string, allowedIPs []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != token {
				apiError(w, 401, "unauthorized")
				return
			}
		}
		if len(allowedIPs) > 0 {
			remote := r.RemoteAddr
			if idx := strings.LastIndex(remote, ":"); idx >= 0 {
				remote = remote[:idx]
			}
			remote = strings.Trim(remote, "[]")
			allowed := false
			for _, ip := range allowedIPs {
				if ip == remote {
					allowed = true
					break
				}
			}
			if !allowed {
				apiError(w, 403, "forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

var tabCmdRe = regexp.MustCompile(`^/api/tabs/(\d+)/command$`)
var tabGetRe = regexp.MustCompile(`^/api/tabs/(\d+)$`)

func apiHandler(d *Daemon, port int, bearerToken string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case path == "/api/health" && r.Method == "GET":
			apiOK(w, map[string]any{
				"status":        "running",
				"connectedTabs": len(d.Tabs()),
				"uptime":        0,
			})

		case path == "/api/screenshot" && r.Method == "GET":
			handleScreenshotGet(w, r, d, port, bearerToken)

		case path == "/api/screenshot" && r.Method == "POST":
			handleScreenshotPost(w, r, d)

		case path == "/api/tabs" && r.Method == "GET":
			apiOK(w, d.Tabs())

		case tabGetRe.MatchString(path) && r.Method == "GET":
			m := tabGetRe.FindStringSubmatch(path)
			id := parseTabID(m[1])
			tabs := d.Tabs()
			for _, t := range tabs {
				if t.ID == id {
					apiOK(w, t)
					return
				}
			}
			apiError(w, 404, "tab not found")

		case path == "/api/command" && r.Method == "POST":
			handleCommand(w, r, d, 0)

		case tabCmdRe.MatchString(path) && r.Method == "POST":
			m := tabCmdRe.FindStringSubmatch(path)
			handleCommand(w, r, d, parseTabID(m[1]))

		default:
			apiError(w, 404, "not found")
		}
	})
}

type commandRequest struct {
	Command Command `json:"command"`
	Timeout int     `json:"timeout"`
}

func handleCommand(w http.ResponseWriter, r *http.Request, d *Daemon, tabID int) {
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command.Action == "" {
		apiError(w, 400, "invalid request body")
		return
	}
	if req.Command.ID == "" {
		req.Command.ID = generateID()
	}
	timeoutMs := req.Timeout
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	if tabID == 0 {
		id, ok := d.FirstTabID()
		if !ok {
			apiError(w, 404, "no tabs connected")
			return
		}
		tabID = id
	}

	result, err := sendCommand(d, tabID, req.Command, timeoutMs)
	if err != nil {
		apiError(w, 404, err.Error())
		return
	}
	apiOK(w, result)
}

// screenshotTimeout bounds how long GET /api/screenshot waits for the
// extension to capture and upload the PNG over HTTP. Kept below the htcli
// client's 30s HTTP timeout so the caller receives the daemon's explicit
// "screenshot timed out" error rather than an opaque client-side deadline.
const screenshotTimeout = 25 * time.Second

// handleScreenshotGet triggers a capture in the extension and blocks until the
// extension POSTs the PNG back (or the wait times out). The screenshot travels
// over HTTP, not the relay, to avoid the 1 MB native-messaging frame limit.
func handleScreenshotGet(w http.ResponseWriter, r *http.Request, d *Daemon, port int, bearerToken string) {
	tabID, ok := d.FirstTabID()
	if !ok {
		apiError(w, 404, "no tabs connected")
		return
	}

	commandID := generateID()
	uploadURL := fmt.Sprintf("http://127.0.0.1:%d/api/screenshot", port)

	ch, err := d.TriggerScreenshot(tabID, commandID, uploadURL, bearerToken)
	if err != nil {
		apiError(w, 404, err.Error())
		return
	}

	timer := time.NewTimer(screenshotTimeout)
	defer timer.Stop()
	select {
	case res := <-ch:
		if res.err != "" {
			apiError(w, 502, "screenshot capture failed: "+res.err)
			return
		}
		apiOK(w, res.data)
	case <-timer.C:
		d.ResolveScreenshot(commandID, "", "") // drop the pending entry
		apiError(w, 504, "screenshot timed out")
	}
}

type screenshotUpload struct {
	CommandID string `json:"commandId"`
	Data      string `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
}

// handleScreenshotPost receives the PNG the extension captured and correlates it
// to the waiting GET handler by command ID. The base64 may be several MB; that
// is fine over HTTP (no native-messaging frame limit applies here).
func handleScreenshotPost(w http.ResponseWriter, r *http.Request, d *Daemon) {
	var up screenshotUpload
	if err := json.NewDecoder(r.Body).Decode(&up); err != nil || up.CommandID == "" {
		apiError(w, 400, "invalid request body")
		return
	}
	d.ResolveScreenshot(up.CommandID, stripDataURLPrefix(up.Data), up.Error)
	apiOK(w, map[string]any{"received": true})
}

// stripDataURLPrefix removes a leading "data:image/...;base64," so the stored
// value is raw base64, which is what GetScreenshot / htcli screenshot expect.
func stripDataURLPrefix(s string) string {
	if i := strings.Index(s, ";base64,"); i != -1 && strings.HasPrefix(s, "data:") {
		return s[i+len(";base64,"):]
	}
	return s
}

func parseTabID(s string) int {
	var id int
	fmt.Sscanf(s, "%d", &id)
	return id
}
