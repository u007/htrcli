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

var fetchCmd = &cobra.Command{
	Use:   "fetch <url>",
	Short: "Make an HTTP request from the browser (bypasses page CSP)",
	Long:  "Makes a fetch request via the extension background script, including session cookies. Use --json for full response.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		method, _ := cmd.Flags().GetString("method")
		body, _ := cmd.Flags().GetString("body")

		opts := map[string]interface{}{
			"method": method,
		}
		if body != "" {
			var jsonBody interface{}
			if err := json.Unmarshal([]byte(body), &jsonBody); err != nil {
				return fmt.Errorf("invalid JSON body: %w", err)
			}
			opts["body"] = jsonBody
		}

		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:      "1",
			Action:  "fetch",
			Value:   args[0],
			Options: opts,
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		if !result.Success {
			return fmt.Errorf("fetch failed: %s", result.Error)
		}

		resultJSON, err := json.MarshalIndent(result.Data, "", "  ")
		if err != nil {
			fmt.Printf("%v\n", result.Data)
			return nil
		}
		fmt.Println(string(resultJSON))
		return nil
	},
}

var printPDFCmd = &cobra.Command{
	Use:   "printpdf <output-path>",
	Short: "Print current tab to PDF via CDP (no save-as prompt)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputPath := args[0]

		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "printToPDF",
		})
		if err != nil {
			return err
		}

		if !result.Success {
			return fmt.Errorf("printToPDF failed: %s", result.Error)
		}

		b64, ok := result.Data.(string)
		if !ok {
			return fmt.Errorf("unexpected result type: %T", result.Data)
		}

		pdfBytes, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return fmt.Errorf("failed to decode PDF: %w", err)
		}

		if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
			return fmt.Errorf("failed to write PDF: %w", err)
		}

		fmt.Printf("PDF saved to %s (%d bytes)\n", outputPath, len(pdfBytes))
		return nil
	},
}

func intPtr(v int) *int { return &v }

// Receipt descriptor for AIA e-receipt download
type aiaReceipt struct {
	idx        int
	policy     string
	rcpt       string
	date       string
	previewURL string
}

var aiaReceipts = []aiaReceipt{
	{0, "PB07156305", "A7976522", "29-DEC-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{1, "4473783A02", "SQ158521", "8-DEC-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{2, "PB07156305", "A7827931", "28-NOV-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{3, "PB07156305", "A7753999", "17-NOV-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{4, "4473783A02", "SO879047", "6-NOV-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{5, "4473783A02", "JE735664", "8-OCT-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{6, "PB07156305", "A7498721", "29-SEPT-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{7, "4473783A02", "SM378133", "9-SEPT-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{8, "PB07156305", "A7338934", "28-AUG-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{9, "4473783A02", "SL071973", "6-AUG-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{10, "PB07156305", "A7179905", "29-JUL-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{11, "4473783A02", "SJ833747", "8-JUL-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{12, "PB07156305", "A7014946", "30-JUN-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{13, "4473783A02", "SI553030", "6-JUN-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{14, "PB07156305", "A6851964", "28-MAY-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{15, "PB07156305", "A6776786", "15-MAY-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{16, "4473783A02", "SH204885", "6-MAY-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{17, "4473783A02", "SF996314", "8-APR-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{18, "PB07156305", "A6530562", "29-MAR-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{19, "4473783A02", "SE677198", "6-MAR-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{20, "PB07156305", "A6369210", "28-FEB-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{21, "4473783A02", "SD410364", "6-FEB-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
	{22, "PB07156305", "A6215219", "28-JAN-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aiagb.html"},
	{23, "4473783A02", "SC105873", "7-JAN-2025", "https://www.aia.com.my/content/my/en/my-aia/dashboard-and-statements/view-statement-ereceipt/e-receipt-aptb.html"},
}

var downloadReceiptsCmd = &cobra.Command{
	Use:   "downloadreceipts",
	Short: "Download all AIA e-receipts for Tan Yee Wen 2025",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		outDir, _ := cmd.Flags().GetString("out")
		aiaTabID := 98184784 // AIA ereceipt page pseudo-ID
		erReceiptURL := "https://www.aia.com.my/en/my-aia/dashboard-and-statements/view-statement-ereceipt.html"

		// Step 1: Find a helper tab and ensure AIA tab is connected and active
		fmt.Println("Step 1: Checking connected tabs...")
		tabs, err := c.ListTabs()
		if err != nil {
			return fmt.Errorf("list tabs: %w", err)
		}
		var helperTabID int
		aiaConnected := false
		for _, t := range tabs {
			if t.ID == aiaTabID {
				aiaConnected = true
			} else if helperTabID == 0 {
				helperTabID = t.ID
			}
		}
		if helperTabID == 0 {
			return fmt.Errorf("no connected helper tab found; open any browser tab with the extension loaded")
		}

		var aiaRealTabID int
		if !aiaConnected {
			fmt.Printf("AIA tab not connected. Opening fresh AIA tab from helper %d...\n", helperTabID)
			openResult, openErr := c.ExecuteCommand(intPtr(helperTabID), api.Command{
				ID:     "openaia",
				Action: "openTab",
				Options: map[string]any{"url": erReceiptURL},
			})
			if openErr != nil || !openResult.Success {
				errMsg := ""
				if openErr != nil {
					errMsg = openErr.Error()
				} else {
					errMsg = openResult.Error
				}
				return fmt.Errorf("failed to open AIA tab: %s", errMsg)
			}
			realIDFloat, _ := openResult.Data.(map[string]any)["tabId"].(float64)
			aiaRealTabID = int(realIDFloat)
			fmt.Printf("AIA tab opened (real ID: %d). Waiting 8s for connection...\n", aiaRealTabID)
			time.Sleep(8 * time.Second)

			tabs, err = c.ListTabs()
			if err != nil {
				return fmt.Errorf("list tabs after open: %w", err)
			}
			aiaConnected = false
			for _, t := range tabs {
				if t.ID == aiaTabID {
					aiaConnected = true
					break
				}
			}
			if !aiaConnected {
				return fmt.Errorf("AIA tab did not connect after opening; real tab ID was %d", aiaRealTabID)
			}
		}

		// Step 2: Make AIA tab active so receipt list loads (only needed for background tab we just opened)
		if aiaRealTabID != 0 {
			fmt.Printf("AIA tab connected. Activating it (real ID %d)...\n", aiaRealTabID)
			_, _ = c.ExecuteCommand(intPtr(helperTabID), api.Command{
				ID:     "switchtab",
				Action: "switchTab",
				Value:  fmt.Sprintf("%d", aiaRealTabID),
			})
			fmt.Println("Waiting 5s for receipt list to load...")
			time.Sleep(5 * time.Second)
		} else {
			fmt.Println("AIA tab already connected and active.")
		}

		// Step 3: Download each receipt
		for _, rcpt := range aiaReceipts {
			outPath := filepath.Join(outDir, rcpt.rcpt+"-"+rcpt.date+".pdf")
			if _, statErr := os.Stat(outPath); statErr == nil {
				fmt.Printf("[%d/24] SKIP (exists): %s\n", rcpt.idx+1, outPath)
				continue
			}

			fmt.Printf("[%d/24] Downloading %s (%s)...\n", rcpt.idx+1, rcpt.rcpt, rcpt.date)

			// Click Preview button at index
			clickResult, err := c.ExecuteCommand(intPtr(aiaTabID), api.Command{
				ID:     "click" + rcpt.rcpt,
				Action: "click",
				Target: &api.TargetSelector{
					Selector: ".ereceipt-list [alt=PreviewPDF]",
					Index:    &rcpt.idx,
				},
			})
			if err != nil || !clickResult.Success {
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				} else {
					errMsg = clickResult.Error
				}
				fmt.Printf("  ERROR clicking preview: %s\n", errMsg)
				continue
			}

			// Poll for sessionStorage.eReceiptData (up to 10s)
			var sessionData string
			for attempt := 0; attempt < 20; attempt++ {
				time.Sleep(500 * time.Millisecond)
				r, pollErr := c.ExecuteCommand(intPtr(aiaTabID), api.Command{
					ID:     "ss" + rcpt.rcpt,
					Action: "getSessionStorage",
					Value:  "eReceiptData",
				})
				if pollErr == nil && r.Success && r.Data != nil {
					if s, ok := r.Data.(string); ok && s != "" && s != "null" {
						sessionData = s
						break
					}
				}
			}
			if sessionData == "" {
				fmt.Printf("  ERROR: sessionStorage not populated after click (API may be unavailable)\n")
				continue
			}
			fmt.Printf("  Session data obtained (%d bytes). Opening preview tab...\n", len(sessionData))

			// Open preview tab with session data injected
			openResult, err := c.ExecuteCommand(intPtr(aiaTabID), api.Command{
				ID:     "open" + rcpt.rcpt,
				Action: "openTab",
				Options: map[string]any{
					"url":         rcpt.previewURL,
					"sessionData": sessionData,
				},
			})
			if err != nil || !openResult.Success {
				errMsg := ""
				if err != nil {
					errMsg = err.Error()
				} else {
					errMsg = openResult.Error
				}
				fmt.Printf("  ERROR opening preview tab: %s\n", errMsg)
				continue
			}

			// Extract real tab ID from openTab result
			realTabIDFloat, _ := openResult.Data.(map[string]any)["tabId"].(float64)
			previewRealTabID := int(realTabIDFloat)
			fmt.Printf("  Preview tab opened (real ID: %d). Waiting for page render...\n", previewRealTabID)
			time.Sleep(5 * time.Second)

			// Print PDF via AIA tab (passes real preview tab ID to background CDP)
			fmt.Printf("  Printing PDF (real tab ID: %d)...\n", previewRealTabID)
			pdfResult, pdfErr := c.ExecuteCommand(intPtr(aiaTabID), api.Command{
				ID:     "print" + rcpt.rcpt,
				Action: "printToPDF",
				Options: map[string]any{
					"tabId": previewRealTabID,
				},
			})
			if pdfErr != nil || !pdfResult.Success {
				// Close tab before skipping
				_, _ = c.ExecuteCommand(intPtr(aiaTabID), api.Command{
					ID: "ct" + rcpt.rcpt, Action: "closeTab",
					Options: map[string]any{"tabId": previewRealTabID},
				})
				if pdfErr != nil {
					fmt.Printf("  ERROR printPDF: %v\n", pdfErr)
				} else {
					fmt.Printf("  ERROR printPDF: %s\n", pdfResult.Error)
				}
				continue
			}

			b64, ok := pdfResult.Data.(string)
			if !ok {
				fmt.Printf("  ERROR: unexpected PDF data type %T\n", pdfResult.Data)
				_, _ = c.ExecuteCommand(intPtr(aiaTabID), api.Command{
					ID: "ct" + rcpt.rcpt, Action: "closeTab",
					Options: map[string]any{"tabId": previewRealTabID},
				})
				continue
			}

			pdfBytes, decErr := base64.StdEncoding.DecodeString(b64)
			if decErr != nil {
				fmt.Printf("  ERROR decoding PDF: %v\n", decErr)
				continue
			}

			if writeErr := os.WriteFile(outPath, pdfBytes, 0644); writeErr != nil {
				fmt.Printf("  ERROR writing file: %v\n", writeErr)
				continue
			}
			fmt.Printf("  Saved: %s (%d bytes)\n", outPath, len(pdfBytes))

			// Close preview tab
			_, _ = c.ExecuteCommand(intPtr(aiaTabID), api.Command{
				ID: "ct" + rcpt.rcpt, Action: "closeTab",
				Options: map[string]any{"tabId": previewRealTabID},
			})
			time.Sleep(1 * time.Second)
		}

		fmt.Println("\nDone.")
		return nil
	},
}

// hashPreviewURL computes the pseudo-tab-ID for a given URL using the same
// algorithm as wsClient.ts hashString(): hash = (hash << 5) - hash + charCode, then abs.
func hashPreviewURL(url string) int {
	hash := int32(0)
	for _, c := range url {
		hash = (hash << 5) - hash + int32(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return int(hash)
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
	fetchCmd.Flags().String("method", "POST", "HTTP method (GET, POST, etc.)")
	fetchCmd.Flags().String("body", "", "JSON body for POST requests")
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(printPDFCmd)
	downloadReceiptsCmd.Flags().String("out", os.ExpandEnv("$HOME/personal/2025tax/receipts/aia"), "Output directory for PDFs")
	rootCmd.AddCommand(downloadReceiptsCmd)
}
