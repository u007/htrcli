package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// emulationStorePathOverride, when non-empty, redirects the emulation store
// from ~/.htrcli/emulation.json so tests can use a temp directory.
var emulationStorePathOverride string

// emulationStorePath returns the path to the emulation store JSON file.
func emulationStorePath() (string, error) {
	if emulationStorePathOverride != "" {
		return emulationStorePathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory for emulation store: %w", err)
	}
	return filepath.Join(home, ".htrcli", "emulation.json"), nil
}

// emulationStore is the emulation.json document: a JSON object keyed by CDP
// port (decimal string). Entries stay raw until their own port needs them, so a
// bad entry blocks only its port, and a save writes every other entry back as
// it was read.
type emulationStore map[string]json.RawMessage

// loadEmulationStore reads the store, or returns an empty store if the file
// does not exist. A document that is not a JSON object is an error. Entries are
// decoded and validated per port by portEmulation.
func loadEmulationStore() (emulationStore, error) {
	path, err := emulationStorePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emulationStore{}, nil
		}
		return nil, fmt.Errorf("reading emulation store %s: %w", path, err)
	}
	var store emulationStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parsing emulation store %s: %w", path, err)
	}
	if store == nil {
		return nil, fmt.Errorf("parsing emulation store %s: expected a JSON object keyed by CDP port", path)
	}
	return store, nil
}

// portEmulation decodes and validates the entry for one port. A port with no
// entry has no emulation. A bad entry is an error that names the port and the
// command that clears it.
func portEmulation(store emulationStore, port int) (cdp.Emulation, error) {
	raw, ok := store[strconv.Itoa(port)]
	if !ok {
		return cdp.Emulation{}, nil
	}
	var e cdp.Emulation
	if err := json.Unmarshal(raw, &e); err != nil {
		return cdp.Emulation{}, badEmulationEntry(port, err)
	}
	if err := e.Validate(); err != nil {
		return cdp.Emulation{}, badEmulationEntry(port, err)
	}
	return e, nil
}

// badEmulationEntry is the error for a stored entry that cannot be applied.
func badEmulationEntry(port int, cause error) error {
	return fmt.Errorf("stored emulation for port %d is invalid (%w); clear it with: HTRCLI_CDP_PORT=%d htrcli --cdp emulate reset", port, cause, port)
}

// setPortEmulation stores e as the entry for port. The caller removes the entry
// instead when e is empty.
func setPortEmulation(store emulationStore, port int, e cdp.Emulation) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encoding emulation for port %d: %w", port, err)
	}
	store[strconv.Itoa(port)] = raw
	return nil
}

// saveEmulationStore writes the store back to disk, creating ~/.htrcli if needed.
func saveEmulationStore(store emulationStore) error {
	path, err := emulationStorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating emulation store dir: %w", err)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding emulation store: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing emulation store %s: %w", path, err)
	}
	return nil
}

// parseViewportSize splits a <width>x<height> argument into positive integers.
func parseViewportSize(raw string) (int, int, error) {
	parts := strings.Split(raw, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid viewport %q (expected <width>x<height>, e.g. 390x844)", raw)
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid viewport width in %q: %w", raw, err)
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid viewport height in %q: %w", raw, err)
	}
	if width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("viewport %q must have a positive width and height", raw)
	}
	return width, height, nil
}

// parseColorScheme maps the CLI value to the stored one. "none" is the empty
// string, meaning no override.
func parseColorScheme(raw string) (string, error) {
	switch raw {
	case "light", "dark":
		return raw, nil
	case "none":
		return "", nil
	}
	return "", fmt.Errorf("invalid color scheme %q (expected light, dark, or none)", raw)
}

// emulationInEffect is what the page reports after the emulation is applied.
type emulationInEffect struct {
	InnerWidth         int    `json:"innerWidth"`
	InnerHeight        int    `json:"innerHeight"`
	PrefersColorScheme string `json:"prefersColorScheme"`
	UserAgent          string `json:"userAgent"`
}

const emulationReadbackExpr = `({innerWidth: window.innerWidth, innerHeight: window.innerHeight, ` +
	`prefersColorScheme: matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light', ` +
	`userAgent: navigator.userAgent})`

func readEmulationInEffect(s *cdp.Session) (emulationInEffect, error) {
	raw, err := cdp.Evaluate(s, emulationReadbackExpr)
	if err != nil {
		return emulationInEffect{}, fmt.Errorf("reading back emulation: %w", err)
	}
	var effect emulationInEffect
	if err := json.Unmarshal(raw, &effect); err != nil {
		return emulationInEffect{}, fmt.Errorf("decoding emulation readback %s: %w", raw, err)
	}
	return effect, nil
}

// describeEmulation renders a stored emulation for `emulate show`.
func describeEmulation(e cdp.Emulation) string {
	var parts []string
	if e.Width > 0 {
		parts = append(parts, fmt.Sprintf("viewport %dx%d scale %v", e.Width, e.Height, e.Scale))
	}
	if e.Mobile {
		parts = append(parts, "mobile")
	}
	if e.ColorScheme != "" {
		parts = append(parts, "color-scheme "+e.ColorScheme)
	}
	if e.UserAgent != "" {
		parts = append(parts, fmt.Sprintf("user-agent %q", e.UserAgent))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// updateEmulation applies one change to the emulation stored for the current
// CDP port. The new entry is validated, then the live session is cleared and the
// new entry applied, then the page is read back. The entry is saved only after
// the browser accepts it, so a rejected value is not stored. An entry that
// becomes empty is removed, so an unset port makes no CDP calls.
func updateEmulation(change func(*cdp.Emulation)) error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	port := GetCDPPort()
	store, err := loadEmulationStore()
	if err != nil {
		return err
	}
	next, err := portEmulation(store, port)
	if err != nil {
		return err
	}
	change(&next)
	if err := next.Validate(); err != nil {
		return fmt.Errorf("emulation for port %d: %w", port, err)
	}
	// cdpSession re-applies the stored entry; the clear below removes it before
	// the new entry is applied.
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := cdp.ClearEmulation(s); err != nil {
		return fmt.Errorf("clearing emulation on port %d: %w", port, err)
	}
	if next == (cdp.Emulation{}) {
		delete(store, strconv.Itoa(port))
	} else {
		if err := cdp.ApplyEmulation(s, next); err != nil {
			return fmt.Errorf("applying emulation on port %d: %w", port, err)
		}
		if err := setPortEmulation(store, port, next); err != nil {
			return err
		}
	}
	effect, err := readEmulationInEffect(s)
	if err != nil {
		return err
	}
	if err := saveEmulationStore(store); err != nil {
		return err
	}

	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "port": port, "emulation": next, "inEffect": effect})
		return nil
	}
	fmt.Printf("Emulation in effect on port %d: innerWidth=%d innerHeight=%d prefers-color-scheme=%s userAgent=%q (cdp)\n",
		port, effect.InnerWidth, effect.InnerHeight, effect.PrefersColorScheme, effect.UserAgent)
	return nil
}

// runEmulateShow prints the emulation stored for the current CDP port. A bad
// entry for this port is an error; entries for other ports are not read.
func runEmulateShow() error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	port := GetCDPPort()
	store, err := loadEmulationStore()
	if err != nil {
		return err
	}
	e, err := portEmulation(store, port)
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"port": port, "emulation": e})
		return nil
	}
	fmt.Printf("port %d: %s\n", port, describeEmulation(e))
	return nil
}

// runEmulateReset removes the stored emulation for the current CDP port and
// issues the CDP clear calls once on a live session. The port's entry is removed
// without being decoded, so a bad entry can always be cleared.
func runEmulateReset() error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	port := GetCDPPort()
	store, err := loadEmulationStore()
	if err != nil {
		return err
	}
	delete(store, strconv.Itoa(port))
	if err := saveEmulationStore(store); err != nil {
		return err
	}

	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()
	if err := cdp.ClearEmulation(s); err != nil {
		return fmt.Errorf("clearing emulation for port %d: %w", port, err)
	}

	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "port": port})
		return nil
	}
	fmt.Printf("Emulation cleared for port %d (cdp)\n", port)
	return nil
}

var (
	emulateViewportScale  float64
	emulateViewportMobile bool
)

var emulateCmd = &cobra.Command{
	Use:   "emulate",
	Short: "Persist viewport, color-scheme, and user-agent emulation for --cdp",
	Long: "Emulation is stored per CDP port and re-applied on every --cdp command, " +
		"because CDP overrides reset when each command's session closes.",
}

var emulateViewportCmd = &cobra.Command{
	Use:   "viewport <width>x<height>",
	Short: "Emulate a viewport size and device scale",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("emulate viewport")
		}
		width, height, err := parseViewportSize(args[0])
		if err != nil {
			return err
		}
		return updateEmulation(func(e *cdp.Emulation) {
			e.Width = width
			e.Height = height
			e.Scale = emulateViewportScale
			e.Mobile = emulateViewportMobile
		})
	},
}

var emulateColorSchemeCmd = &cobra.Command{
	Use:   "color-scheme <light|dark|none>",
	Short: "Emulate prefers-color-scheme (none removes the override)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("emulate color-scheme")
		}
		scheme, err := parseColorScheme(args[0])
		if err != nil {
			return err
		}
		return updateEmulation(func(e *cdp.Emulation) {
			e.ColorScheme = scheme
		})
	},
}

var emulateUserAgentCmd = &cobra.Command{
	Use:   "user-agent <string>",
	Short: "Override the user agent (an empty string removes the override)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("emulate user-agent")
		}
		return updateEmulation(func(e *cdp.Emulation) {
			e.UserAgent = args[0]
		})
	},
}

var emulateShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the emulation stored for the current CDP port",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("emulate show")
		}
		return runEmulateShow()
	},
}

var emulateResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove all emulation for the current CDP port",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("emulate reset")
		}
		return runEmulateReset()
	},
}

func init() {
	emulateViewportCmd.Flags().Float64Var(&emulateViewportScale, "scale", 1, "device scale factor (must be positive)")
	emulateViewportCmd.Flags().BoolVar(&emulateViewportMobile, "mobile", false, "emulate a mobile device (mobile meta and touch)")

	emulateCmd.AddCommand(emulateViewportCmd)
	emulateCmd.AddCommand(emulateColorSchemeCmd)
	emulateCmd.AddCommand(emulateUserAgentCmd)
	emulateCmd.AddCommand(emulateShowCmd)
	emulateCmd.AddCommand(emulateResetCmd)
	rootCmd.AddCommand(emulateCmd)
}
