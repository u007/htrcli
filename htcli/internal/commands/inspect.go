package commands

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htcli/internal/api"
	"github.com/u007/htcli/internal/output"
)

var findCmd = &cobra.Command{
	Use:   "find <selector>",
	Short: "Find element and return info",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "find",
			Target: parseSelector(args[0]),
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		if result.Data != nil {
			output.PrintJSON(result.Data)
		} else {
			fmt.Printf("Element not found: %s\n", args[0])
		}
		return nil
	},
}

var getTextCmd = &cobra.Command{
	Use:   "text <selector>",
	Short: "Get text content of element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInspect("getText", args[0])
	},
}

var getValueCmd = &cobra.Command{
	Use:   "value <selector>",
	Short: "Get input value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInspect("getValue", args[0])
	},
}

var getAttrCmd = &cobra.Command{
	Use:   "attr <selector> <attribute>",
	Short: "Get attribute value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "getAttribute",
			Target: parseSelector(args[0]),
			Options: map[string]any{
				"attribute": args[1],
			},
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("%v\n", result.Data)
		return nil
	},
}

var getHTMLCmd = &cobra.Command{
	Use:   "html <selector>",
	Short: "Get innerHTML of element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInspect("getHTML", args[0])
	},
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Get page accessibility tree with refs",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "getPageInfo",
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		// For now, output page info as a placeholder for the full snapshot.
		// Full snapshot implementation requires extension changes (axe-core).
		fmt.Println("Snapshot requires extension update (Phase 4)")
		fmt.Println("Use 'page' command for current page info.")
		return nil
	},
}

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [path]",
	Short: "Take screenshot (saves PNG)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()

		// Get screenshot data.
		data, err := c.GetScreenshot()
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(map[string]string{"screenshot": data})
			return nil
		}

		// Decode base64.
		imgData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return fmt.Errorf("failed to decode screenshot: %w", err)
		}

		// Determine output path.
		path := ""
		if len(args) > 0 {
			path = args[0]
		} else {
			// Use temp file.
			tmpDir := os.TempDir()
			filename := fmt.Sprintf("screenshot-%d.png", time.Now().UnixMilli())
			path = filepath.Join(tmpDir, filename)
		}

		// Write file.
		if err := os.WriteFile(path, imgData, 0644); err != nil {
			return fmt.Errorf("failed to write screenshot: %w", err)
		}

		fmt.Printf("Screenshot saved to %s\n", path)
		return nil
	},
}

var pageInfoCmd = &cobra.Command{
	Use:   "page",
	Short: "Get page info (URL, title, dimensions)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		page, err := c.GetPageInfo()
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(page)
			return nil
		}

		fmt.Printf("URL:      %s\n", page.URL)
		fmt.Printf("Title:    %s\n", page.Title)
		fmt.Printf("Domain:   %s\n", page.Domain)
		fmt.Printf("Viewport: %dx%d\n", page.ViewportWidth, page.ViewportHeight)
		fmt.Printf("Document: %dx%d\n", page.DocumentWidth, page.DocumentHeight)
		fmt.Printf("Scroll:   %.0f, %.0f\n", page.ScrollX, page.ScrollY)
		return nil
	},
}

var evalCmd = &cobra.Command{
	Use:   "eval <javascript>",
	Short: "Execute JavaScript",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "evaluate",
			Value:  args[0],
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("%v\n", result.Data)
		return nil
	},
}

var commandCmd = &cobra.Command{
	Use:   "command <json>",
	Short: "Send raw JSON command",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse the raw JSON as a Command.
		var rawCmd api.Command
		if err := json.Unmarshal([]byte(args[0]), &rawCmd); err != nil {
			return fmt.Errorf("invalid command JSON: %w", err)
		}

		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), rawCmd)
		if err != nil {
			return err
		}

		output.PrintJSON(result)
		return nil
	},
}

func runInspect(action, selector string) error {
	c := GetClient()
	result, err := c.ExecuteCommand(GetTabID(), api.Command{
		ID:     "1",
		Action: action,
		Target: parseSelector(selector),
	})
	if err != nil {
		return err
	}

	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}

	fmt.Printf("%v\n", result.Data)
	return nil
}

func init() {
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(getTextCmd)
	rootCmd.AddCommand(getValueCmd)
	rootCmd.AddCommand(getAttrCmd)
	rootCmd.AddCommand(getHTMLCmd)
	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(screenshotCmd)
	rootCmd.AddCommand(pageInfoCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(commandCmd)
}
