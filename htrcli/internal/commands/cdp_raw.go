package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/output"
)

// cdpRawCmd is the raw DevTools Protocol passthrough. Each invocation opens its
// own --cdp session and closes it on return, so session-scoped state (any
// Emulation.* override, Network.setUserAgentOverride, Fetch.enable, ...) lasts
// only for this command. Persistent emulation is the emulate verb group.
var cdpRawCmd = &cobra.Command{
	Use:   "cdp <method> [params-json]",
	Short: "Send a raw Chrome DevTools Protocol method (--cdp only)",
	Long: "Sends one DevTools Protocol method, e.g. Page.getLayoutMetrics, to the page " +
		"selected by --tab and prints its result. params-json must be a JSON object. " +
		"Each invocation opens a new session, so session-scoped state (Emulation.*, " +
		"Network.setUserAgentOverride, Fetch.enable, ...) lasts only for this command.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("cdp")
		}
		method, params, err := parseCDPRawArgs(args)
		if err != nil {
			return err
		}
		if note := cdpRawEmulationNote(method); note != "" {
			fmt.Fprintln(os.Stderr, note)
		}

		s, _, err := cdpSession()
		if err != nil {
			return err
		}
		defer s.Close()

		// Pass a literal nil when no params were given, so the request omits
		// the field instead of sending "params": null.
		var callParams any
		if params != nil {
			callParams = params
		}
		var raw json.RawMessage
		if err := s.Call(method, callParams, &raw); err != nil {
			return err
		}
		if len(raw) == 0 {
			return fmt.Errorf("%s: CDP response had no result field", method)
		}

		if output.JSONOutput {
			fmt.Println(string(raw))
			return nil
		}
		output.PrintJSON(raw)
		return nil
	},
}

// parseCDPRawArgs splits the positional arguments into the method and the
// optional params object. params is the original bytes, not a re-marshalled
// map, so numbers and key order reach the browser unchanged. It is nil when no
// params argument was given.
func parseCDPRawArgs(args []string) (string, json.RawMessage, error) {
	method := args[0]
	if method == "" {
		return "", nil, fmt.Errorf("cdp: method must not be empty (expected <Domain.method>)")
	}
	if len(args) == 1 {
		return method, nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args[1]), &obj); err != nil {
		return "", nil, fmt.Errorf("cdp %s: params must be a JSON object: %w", method, err)
	}
	// JSON null unmarshals into a nil map without error, so it is rejected here.
	if obj == nil {
		return "", nil, fmt.Errorf("cdp %s: params must be a JSON object, got %s", method, args[1])
	}
	return method, json.RawMessage(args[1]), nil
}

// cdpRawEmulationNote returns the stderr note for Emulation.* methods, whose
// override ends with this command. Other methods get no note.
func cdpRawEmulationNote(method string) string {
	if !strings.HasPrefix(method, "Emulation.") {
		return ""
	}
	return "[htrcli] " + method + " applies only to this command's CDP session. " +
		"For emulation that persists across commands, use `htrcli --cdp emulate viewport|color-scheme|user-agent`."
}

func init() {
	rootCmd.AddCommand(cdpRawCmd)
}
