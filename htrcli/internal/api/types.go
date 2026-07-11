package api

// TargetSelector defines how to find an element on the page.
// Multiple strategies can be combined; they are tried in priority order.
type TargetSelector struct {
	Selector      string `json:"selector,omitempty"`
	XPath         string `json:"xpath,omitempty"`
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Role          string `json:"role,omitempty"`
	Label         string `json:"label,omitempty"`
	Placeholder   string `json:"placeholder,omitempty"`
	Text          string `json:"text,omitempty"`
	TextMatch     string `json:"textMatch,omitempty"`
	CaseSensitive *bool  `json:"caseSensitive,omitempty"`
	Tag           string `json:"tag,omitempty"`
	Type          string `json:"type,omitempty"`
	Index         *int   `json:"index,omitempty"`
	All           *bool  `json:"all,omitempty"`
	Visible       *bool  `json:"visible,omitempty"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

// Command represents a remote control command to execute on a browser tab.
type Command struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Target  *TargetSelector `json:"target,omitempty"`
	Value   string          `json:"value,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

// CommandResult is the response from executing a command.
type CommandResult struct {
	ID         string    `json:"id"`
	Success    bool      `json:"success"`
	Data       any       `json:"data,omitempty"`
	Error      string    `json:"error,omitempty"`
	Screenshot string    `json:"screenshot,omitempty"`
	Duration   int       `json:"duration,omitempty"`
	PageInfo   *PageInfo `json:"pageInfo,omitempty"`
}

// TabInfo contains information about a connected browser tab.
type TabInfo struct {
	ID         int    `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Active     bool   `json:"active"`
	FavIconURL string `json:"favIconUrl,omitempty"`
}

// PageInfo contains information about the current page state.
// Field names mirror the extension's PageInfo; new fields added on the
// extension side must be mirrored here (the daemon's /api/page returns
// the live PageInfo and the client decodes it back into this struct).
type PageInfo struct {
	URL            string  `json:"url"`
	Title          string  `json:"title"`
	Domain         string  `json:"domain"`
	ReadyState     string  `json:"readyState,omitempty"`
	ScrollX        float64 `json:"scrollX"`
	ScrollY        float64 `json:"scrollY"`
	ViewportWidth  int     `json:"viewportWidth"`
	ViewportHeight int     `json:"viewportHeight"`
	DocumentHeight int     `json:"documentHeight"`
	DocumentWidth  int     `json:"documentWidth"`
	HistoryLength  int     `json:"historyLength,omitempty"`
}

// ApiResponse is the standard response envelope from the server.
type ApiResponse struct {
	OK    bool `json:"ok"`
	Data  any  `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// CommandRequest is the request body for POST /api/command.
type CommandRequest struct {
	Command    Command `json:"command"`
	Screenshot bool    `json:"screenshot,omitempty"`
	Timeout    int     `json:"timeout,omitempty"`
}

// HealthResponse is the response from GET /api/health.
type HealthResponse struct {
	Status        string  `json:"status"`
	ConnectedTabs int     `json:"connectedTabs"`
	Uptime        float64 `json:"uptime"`
}
