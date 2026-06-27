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
	mux.Handle("/api/", authMiddleware(bearerToken, allowedIPs, apiHandler(d)))

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

func apiHandler(d *Daemon) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case path == "/api/health" && r.Method == "GET":
			apiOK(w, map[string]any{
				"status":        "running",
				"connectedTabs": len(d.Tabs()),
				"uptime":        0,
			})

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

func parseTabID(s string) int {
	var id int
	fmt.Sscanf(s, "%d", &id)
	return id
}
