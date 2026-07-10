# Part 1: Flag & Config Groundwork

Spec: `docs/superpowers/specs/2026-07-10-htcli-cdp-transport-design.md`. All work in `htcli/` (Go 1.22, cobra + viper). Run tests with `cd htcli && go test ./...`.

---

### Task 1: Change global `--tab` flag from int to string

CDP target IDs are 32-char hex strings; the current flag is `IntVar`. Change the flag to string; the extension path parses to int with a clear error.

**Files:**
- Modify: `htcli/internal/commands/root.go` (vars block ~line 18, `init()` ~line 44, `GetTabID()` ~line 96)
- Test: `htcli/internal/commands/root_test.go` (create)

**Interfaces:**
- Consumes: nothing new.
- Produces: `GetTabID() (*int, error)` — parsed extension tab ID or nil; error on non-numeric. `GetTabTarget() string` — raw string for the CDP path (empty = unset). ALL existing callers of `GetTabID()` must be updated to handle the error.

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/commands/root_test.go`:

```go
package commands

import "testing"

func TestGetTabIDNumeric(t *testing.T) {
	tabTarget = "123"
	defer func() { tabTarget = "" }()
	id, err := GetTabID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == nil || *id != 123 {
		t.Fatalf("want 123, got %v", id)
	}
}

func TestGetTabIDEmpty(t *testing.T) {
	tabTarget = ""
	id, err := GetTabID()
	if err != nil || id != nil {
		t.Fatalf("want nil,nil got %v,%v", id, err)
	}
}

func TestGetTabIDNonNumeric(t *testing.T) {
	tabTarget = "8E17C9D24A3B41F09E60C1D2A55F7B31"
	defer func() { tabTarget = "" }()
	if _, err := GetTabID(); err == nil {
		t.Fatal("want error for non-numeric tab on extension transport")
	}
}

func TestGetTabTarget(t *testing.T) {
	tabTarget = "8E17C9D24A3B41F09E60C1D2A55F7B31"
	defer func() { tabTarget = "" }()
	if got := GetTabTarget(); got != "8E17C9D24A3B41F09E60C1D2A55F7B31" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/commands/ -run TestGetTab -v`
Expected: compile error — `tabTarget` undefined, `GetTabID()` returns one value.

- [ ] **Step 3: Implement in root.go**

In the `var` block, replace `tabID int` with `tabTarget string`. In `init()` replace the IntVar line with:

```go
rootCmd.PersistentFlags().StringVar(&tabTarget, "tab", "", "target tab: numeric ID (extension) or CDP target ID (--cdp)")
```

Replace `GetTabID` with:

```go
// GetTabID returns the numeric extension tab ID, nil if unset.
// Errors when --tab is non-numeric (that form is CDP-only).
func GetTabID() (*int, error) {
	if tabTarget == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(tabTarget)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("--tab %q is not a numeric tab ID (CDP target IDs require --cdp)", tabTarget)
	}
	return &id, nil
}

// GetTabTarget returns the raw --tab value for the CDP transport ("" = unset).
func GetTabTarget() string {
	return tabTarget
}
```

Add `"strconv"` to imports.

- [ ] **Step 4: Update all GetTabID callers**

Find them: `grep -rn "GetTabID()" htcli/internal/commands/`. Every call site (in `interact.go`, `inspect.go`, `navigate.go`, `tabs.go`, …) currently does `c.ExecuteCommand(GetTabID(), …)` or similar. Update each to:

```go
tabID, err := GetTabID()
if err != nil {
	return err
}
result, err := c.ExecuteCommand(tabID, api.Command{ /* unchanged */ })
```

Keep each call site's surrounding logic untouched — only the tab-ID retrieval changes.

- [ ] **Step 5: Run full test suite and build**

Run: `cd htcli && go build ./... && go test ./...`
Expected: all PASS, no compile errors.

- [ ] **Step 6: Commit**

```bash
git add htcli/internal/commands/
git commit -m "refactor(htcli): --tab flag accepts string for CDP target IDs

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Config fields + subcommands + transport resolution

**Files:**
- Modify: `htcli/internal/commands/config.go` (`configData` struct, `setConfigValue` switch, `configShowCmd`, `init()`)
- Modify: `htcli/internal/commands/root.go` (add `--transport` / `--cdp` flags, `UseCDP()`, `GetCDPPort()`, `GetChromePath()`)
- Test: `htcli/internal/commands/config_test.go` (create)

**Interfaces:**
- Consumes: Task 1 (`tabTarget` exists; no direct use).
- Produces: `UseCDP() bool`, `GetCDPPort() int` (default 9222), `GetChromePath() string` (may be ""). Config file keys: `transport`, `cdp-port`, `chrome-path`. Subcommands: `config set-transport <ext|cdp>`, `config set-cdp-port <port>`, `config set-chrome-path <path>`.

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/commands/config_test.go`:

```go
package commands

import (
	"testing"

	"github.com/spf13/viper"
)

func resetTransportState() {
	transportFlag = ""
	cdpFlag = false
	viper.Set("transport", "")
	viper.Set("cdp-port", 0)
}

func TestUseCDPDefaultFalse(t *testing.T) {
	resetTransportState()
	if UseCDP() {
		t.Fatal("default transport must be extension")
	}
}

func TestUseCDPFlag(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	defer resetTransportState()
	if !UseCDP() {
		t.Fatal("--cdp must enable CDP transport")
	}
}

func TestUseCDPConfigSticky(t *testing.T) {
	resetTransportState()
	viper.Set("transport", "cdp")
	defer resetTransportState()
	if !UseCDP() {
		t.Fatal("config transport=cdp must enable CDP")
	}
}

func TestFlagOverridesConfigBothDirections(t *testing.T) {
	resetTransportState()
	viper.Set("transport", "cdp")
	transportFlag = "ext"
	defer resetTransportState()
	if UseCDP() {
		t.Fatal("--transport ext must override config cdp")
	}
}

func TestGetCDPPortDefault(t *testing.T) {
	resetTransportState()
	if got := GetCDPPort(); got != 9222 {
		t.Fatalf("want 9222, got %d", got)
	}
}

func TestSetTransportRejectsInvalid(t *testing.T) {
	if err := validateTransport("chrome"); err == nil {
		t.Fatal("want error for invalid transport value")
	}
	if err := validateTransport("cdp"); err != nil {
		t.Fatalf("cdp must be valid: %v", err)
	}
	if err := validateTransport("ext"); err != nil {
		t.Fatalf("ext must be valid: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/commands/ -run 'TestUseCDP|TestGetCDPPort|TestSetTransport|TestFlagOverrides' -v`
Expected: compile error — `transportFlag`, `cdpFlag`, `UseCDP`, `validateTransport` undefined.

- [ ] **Step 3: Implement flags + resolution in root.go**

Add to the `var` block: `transportFlag string` and `cdpFlag bool`. In `init()`:

```go
rootCmd.PersistentFlags().StringVar(&transportFlag, "transport", "", "transport: ext (extension, default) or cdp")
rootCmd.PersistentFlags().BoolVar(&cdpFlag, "cdp", false, "shorthand for --transport cdp")
```

Add:

```go
// UseCDP resolves the transport: flags > config/env > default ext.
func UseCDP() bool {
	if transportFlag != "" {
		return transportFlag == "cdp"
	}
	if cdpFlag {
		return true
	}
	return viper.GetString("transport") == "cdp"
}

// GetCDPPort returns the CDP debugging port (flags none; env/config; default 9222).
func GetCDPPort() int {
	if p := viper.GetInt("cdp-port"); p > 0 {
		return p
	}
	return 9222
}

// GetChromePath returns the configured Chrome binary path ("" = autodetect).
func GetChromePath() string {
	return viper.GetString("chrome-path")
}
```

- [ ] **Step 4: Extend configData and subcommands in config.go**

Extend the struct (order matters for readability, keep existing fields first):

```go
type configData struct {
	Server       string `json:"server"`
	Token        string `json:"token"`
	AMOAPIKey    string `json:"amo-api-key"`
	AMOAPISecret string `json:"amo-api-secret"`
	Transport    string `json:"transport,omitempty"`
	CDPPort      int    `json:"cdp-port,omitempty"`
	ChromePath   string `json:"chrome-path,omitempty"`
}
```

Add cases to the `switch key` in `setConfigValue` — plus a default that errors instead of silently no-oping:

```go
	case "transport":
		cfg.Transport = value
	case "cdp-port":
		p, err := strconv.Atoi(value)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("cdp-port must be a port number, got %q", value)
		}
		cfg.CDPPort = p
	case "chrome-path":
		cfg.ChromePath = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
```

(`cdp-port` needs `viper.Set(key, p)` with the int, not the string — special-case it before the shared `viper.Set(key, value)` line.)

Add validation + subcommands:

```go
func validateTransport(v string) error {
	if v != "ext" && v != "cdp" {
		return fmt.Errorf("transport must be \"ext\" or \"cdp\", got %q", v)
	}
	return nil
}

var configSetTransportCmd = &cobra.Command{
	Use:   "set-transport <ext|cdp>",
	Short: "Set default transport (ext = extension, cdp = direct CDP)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateTransport(args[0]); err != nil {
			return err
		}
		return setConfigValue("transport", args[0])
	},
}

var configSetCDPPortCmd = &cobra.Command{
	Use:   "set-cdp-port <port>",
	Short: "Set CDP debugging port (default 9222)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("cdp-port", args[0])
	},
}

var configSetChromePathCmd = &cobra.Command{
	Use:   "set-chrome-path <path>",
	Short: "Set Chrome binary path for htcli browser start",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("chrome-path", args[0])
	},
}
```

Register all three in `init()`. Extend `configShowCmd` to print the three new values (plain values, no masking — none are secrets):

```go
	fmt.Printf("Transport: %s\n", cmp.Or(cfg.Transport, "ext"))
	fmt.Printf("CDP port: %d\n", cmp.Or(cfg.CDPPort, 9222))
	fmt.Printf("Chrome path: %s\n", cmp.Or(cfg.ChromePath, "(autodetect)"))
```

and populate them in the `cfg := configData{...}` literal from viper (`viper.GetString("transport")`, `viper.GetInt("cdp-port")`, `viper.GetString("chrome-path")`). Import `cmp` and `strconv`.

- [ ] **Step 5: Run tests**

Run: `cd htcli && go build ./... && go test ./internal/commands/ -v`
Expected: all PASS including Task 1 tests.

- [ ] **Step 6: Verify round-trip manually**

```bash
cd htcli && go run ./cmd/htcli --config /tmp/htcli-test.json config set-transport cdp
go run ./cmd/htcli --config /tmp/htcli-test.json config set-server http://127.0.0.1:3845
cat /tmp/htcli-test.json
```
Expected: JSON contains BOTH `"transport": "cdp"` and `"server"` — proving new keys survive writes of old keys. Then `config set-transport bogus` must error. Clean up: `rm /tmp/htcli-test.json`.

- [ ] **Step 7: Commit**

```bash
git add htcli/internal/commands/config.go htcli/internal/commands/root.go htcli/internal/commands/config_test.go
git commit -m "feat(htcli): transport/cdp-port/chrome-path config and --cdp flag resolution

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
