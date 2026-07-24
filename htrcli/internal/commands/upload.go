package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// parseUploadFiles splits the comma-separated file arg and verifies each path
// exists locally (setFileInputFiles fails opaquely on a missing path, so we
// fail early and clearly). Returns absolute-ish paths as given.
func parseUploadFiles(arg string) ([]string, error) {
	parts := strings.Split(arg, ",")
	files := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("file not found: %s", p)
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files given")
	}
	return files, nil
}

var uploadCmd = &cobra.Command{
	Use:   "upload <selector|@ref> <file[,file...]>",
	Short: "Set files on a file input without an OS file-picker",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := parseUploadFiles(args[1])
		if err != nil {
			return err
		}
		if UseCDP() {
			return runUploadCDP(args[0], files)
		}
		return runUploadExt(args[0], files)
	},
}

// runUploadCDP uploads files via CDP DOM.setFileInputFiles, resolving the
// target as either a CSS selector (resolve -> backendNodeId) or an @eN ref
// (backendNodeId from the persistent ref store).
func runUploadCDP(target string, files []string) error {
	sel := parseSelector(target)

	// If it's an @eN ref, look up the backendNodeId from the ref store.
	if sel.Ref != "" {
		rs, err := LoadRefStore()
		if err != nil {
			return err
		}
		backendID, ok := rs.Lookup(sel.Ref)
		if !ok {
			return fmt.Errorf("stale ref: %s is not known in the CDP ref store", sel.Ref)
		}
		s, _, err := cdpSession()
		if err != nil {
			return err
		}
		defer s.Close()
		if err := cdp.SetFileInputFiles(s, backendID, files); err != nil {
			return fmt.Errorf("setting files via ref %s: %w", sel.Ref, err)
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"success": true, "ref": sel.Ref, "files": files})
			return nil
		}
		fmt.Printf("Uploaded %d file(s) to %s (cdp)\n", len(files), sel.Ref)
		return nil
	}

	// Fresh CSS selector resolve.
	cssSel := sel.Selector
	if cssSel == "" {
		return fmt.Errorf("upload requires a CSS selector or @eN ref, got %q", target)
	}

	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	backendID, err := cdp.ResolveBackendNodeID(s, cssSel)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", cssSel, err)
	}
	if err := cdp.SetFileInputFiles(s, backendID, files); err != nil {
		return fmt.Errorf("setting files on %q: %w", cssSel, err)
	}

	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "selector": cssSel, "files": files})
		return nil
	}
	fmt.Printf("Uploaded %d file(s) to %s (cdp)\n", len(files), target)
	return nil
}

// runUploadExt sends an uploadFiles command via the extension daemon, which
// handles the chrome.debugger attach -> DOM.* resolve -> DOM.setFileInputFiles
// flow. @eN refs are not supported on the extension transport (CDP only).
func runUploadExt(target string, files []string) error {
	sel := parseSelector(target)
	if sel.Ref != "" {
		return fmt.Errorf("upload using @eN refs is only supported on the --cdp transport; use a CSS selector or --cdp")
	}
	if sel.Selector == "" {
		return fmt.Errorf("upload on the extension transport supports CSS selectors only (got %q)", target)
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	// Build []any so the JSON body carries a plain string array under files.
	fileList := make([]any, len(files))
	for i, f := range files {
		fileList[i] = f
	}
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  "uploadFiles",
		Target:  sel,
		Options: map[string]any{"files": fileList},
	})
	if err != nil {
		return err
	}
	if err := commandError(result); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}
	fmt.Printf("Uploaded %d file(s) to %s\n", len(files), target)
	return nil
}

func init() {
	rootCmd.AddCommand(uploadCmd)
}
