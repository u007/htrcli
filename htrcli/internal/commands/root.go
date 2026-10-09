package commands

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var (
	cfgFile       string
	serverURL     string
	token         string
	jsonOutput    bool
	tabTarget     string
	transportFlag string
	// browserFlag is the advisory --browser hint. Empty means no preference, so
	// the daemon falls back to the earliest-connected relay.
	browserFlag string
	cdpFlag     bool
	contextName string
	// frameFlag is the global --frame: CSS selector of a same-origin <iframe>
	// that frame-aware CDP verbs run inside. Empty = top-level document.
	frameFlag        string
	timeout          int
	client           *api.Client
	resolveContextFn = resolveContext
)

var rootCmd = &cobra.Command{
	Use:   "htrcli",
	Short: "HTR NControl CLI - control browser tabs remotely",
	Long: `htrcli is a CLI for controlling browser tabs via the HTR NControl
remote control API. Requires the HTR NControl Chrome extension and
server running.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := validateBrowserHint(browserFlag); err != nil {
			return err
		}
		if err := validateFrameFlag(frameFlag, cmd, UseCDP()); err != nil {
			return err
		}
		output.JSONOutput = jsonOutput
		initClient()
		return nil
	},
}

func init() {
	rootRef = rootCmd
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.htrcli/config.json)")
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "server URL (overrides config)")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "bearer token (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output raw JSON")
	rootCmd.PersistentFlags().StringVar(&tabTarget, "tab", "", "target tab: numeric ID (extension) or CDP target ID (--cdp)")
	rootCmd.PersistentFlags().StringVar(&transportFlag, "transport", "", "transport: ext (extension, default) or cdp")
	rootCmd.PersistentFlags().BoolVar(&cdpFlag, "cdp", false, "shorthand for --transport cdp")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "named browser context (isolated profile)")
	rootCmd.PersistentFlags().StringVar(&frameFlag, "frame", "",
		"CSS selector of a same-origin <iframe> to run the command inside (--cdp only; selector verbs and fill-form)")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 30000, "command timeout in ms")
	rootCmd.PersistentFlags().StringVar(&browserFlag, "browser", "",
		"advisory hint for tab-less commands: prefer this browser profile (chrome|firefox); "+
			"falls back to the earliest connected browser if it is not connected")
}

// validateBrowserHint rejects an unrecognised --browser value. An empty value is
// valid and means "no preference".
//
// The check is deliberately LOUD. The hint is advisory — the daemon falls back
// to the earliest-connected relay when the named profile is absent — so silently
// accepting a typo would make `--browser safri` behave exactly like a first-wins
// call, and the user would have no way to tell their hint was ignored.
func validateBrowserHint(v string) error {
	switch v {
	case "", "chrome", "firefox":
		return nil
	default:
		return fmt.Errorf("invalid --browser %q: want chrome, firefox, or omitted for no preference", v)
	}
}

// AnnotationFrameAware marks a cobra command as honouring the global --frame:
// every CDP verb that takes an element selector and runs through the DOM
// bundle or trusted input. The annotation lives ON the command (see the
// frame-aware verbs in interact.go, inspect.go, pick.go, fillform.go) instead
// of in a name-keyed allowlist, so a command cannot be accepted because its
// leaf name collides with another verb's, and a new frame-aware command can
// never be silently rejected. Anything not annotated (eval, screenshot, open,
// upload, drag, findAll, tabs, …) would silently ignore the flag and act on
// the top-level document, so it is rejected instead.
const AnnotationFrameAware = "htrcli.frame-aware"

// FrameAwareAnnotations returns the annotation value to set on a command that
// honours --frame.
func FrameAwareAnnotations() map[string]string {
	return map[string]string{AnnotationFrameAware: "true"}
}

// frameAware reports whether cmd, or one of its parents, honours --frame.
func frameAware(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[AnnotationFrameAware] == "true" {
			return true
		}
	}
	return false
}

// rootRef points at rootCmd once init() has run. frameAwareVerbs goes through
// it rather than rootCmd directly: rootCmd's PersistentPreRunE calls
// validateFrameFlag, so a direct reference would be an initialization cycle.
var rootRef *cobra.Command

// frameAwareVerbs lists the annotated commands (sorted) so the rejection
// message can never drift from the annotations themselves.
func frameAwareVerbs() []string {
	var verbs []string
	if rootRef == nil {
		return nil
	}
	for _, c := range rootRef.Commands() {
		if c.Annotations[AnnotationFrameAware] == "true" {
			verbs = append(verbs, c.Name())
		}
	}
	sort.Strings(verbs)
	return verbs
}

// validateFrameFlag rejects --frame wherever it cannot be honoured: on the
// extension transport, and on verbs that do not run inside a frame.
func validateFrameFlag(frame string, cmd *cobra.Command, cdp bool) error {
	if frame == "" {
		return nil
	}
	if !cdp {
		return fmt.Errorf("--frame is only supported over --cdp")
	}
	if !frameAware(cmd) {
		return fmt.Errorf("--frame is not supported by %q (frame-aware verbs: %s)",
			cmd.Name(), strings.Join(frameAwareVerbs(), ", "))
	}
	return nil
}

// BrowserHint returns the advisory --browser value, or "" when unset.
func BrowserHint() string { return browserFlag }

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home dir: %v\n", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home + "/.htrcli")
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("HTRCLI")
	// Dashed config keys (cdp-port, chrome-path) map to underscored env vars
	// (HTRCLI_CDP_PORT, HTRCLI_CHROME_PATH) — dashes can't appear in shell
	// variable names.
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	_ = viper.ReadInConfig()
}

func initClient() {
	// Priority: flags > env > config file > defaults
	srv := serverURL
	if srv == "" {
		srv = viper.GetString("server")
	}
	if srv == "" {
		srv = "http://127.0.0.1:3845"
	}

	tkn := token
	if tkn == "" {
		tkn = viper.GetString("token")
	}

	client = api.NewClient(srv, tkn)
}

// GetClient returns the initialized API client.
func GetClient() *api.Client {
	if client == nil {
		initClient()
	}
	return client
}

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

// contextCDPPort caches the resolved context port so GetCDPPort returns the
// same port for the duration of a single command invocation.
var contextCDPPort int

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

// GetCDPPort returns the CDP debugging port.
//   - When --context is set, returns the resolved context port (cached after resolveContext).
//   - When cdp-port is configured, returns that value.
//   - Default: 9222.
func GetCDPPort() int {
	if contextCDPPort > 0 {
		return contextCDPPort
	}
	if p := viper.GetInt("cdp-port"); p > 0 {
		return p
	}
	return 9222
}

// ensureContextResolved resolves a named context lazily the first time a CDP
// command actually needs the port. Non-CDP commands never call this helper, so
// `--context` stays inert for extension/HTTP-only verbs.
func ensureContextResolved() error {
	if contextName == "" || contextCDPPort > 0 {
		return nil
	}
	return resolveContextFn()
}

// GetChromePath returns the configured Chrome binary path ("" = autodetect).
func GetChromePath() string {
	return viper.GetString("chrome-path")
}

// resolveContext ensures the named context's Chrome process is running and
// caches its debugging port. Called from PersistentPreRunE when --context is set.
func resolveContext() error {
	chrome, err := cdp.FindChrome(GetChromePath())
	if err != nil {
		return fmt.Errorf("--context %s: %w", contextName, err)
	}
	port, err := cdp.EnsureContext(contextName, chrome, false)
	if err != nil {
		return fmt.Errorf("--context %s: %w", contextName, err)
	}
	contextCDPPort = port
	return nil
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errorObjectNeeded(err) {
			output.PrintJSON(map[string]any{
				"success": false,
				"error":   err.Error(),
			})
		}
		os.Exit(1)
	}
}

// reportedJSONError marks a failure whose full report a command has already
// printed as its single --json document (with "success": false). Execute
// then exits non-zero without appending a second JSON object, so stdout stays
// one parseable value.
type reportedJSONError struct{ msg string }

func (e *reportedJSONError) Error() string { return e.msg }

// errorObjectNeeded reports whether Execute must print the standard
// {"success": false, "error"} object for err.
func errorObjectNeeded(err error) bool {
	var reported *reportedJSONError
	return !errors.As(err, &reported)
}
