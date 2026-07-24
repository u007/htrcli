package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

const dialogEventKind = "dialog"

type dialogEventData struct {
	DialogType     string `json:"dialogType"`
	Message        string `json:"message"`
	ResolvedAction string `json:"resolvedAction"`
	RespondedText  string `json:"respondedText,omitempty"`
}

var allowedDialogActions = map[string]struct{}{
	"accept":  {},
	"dismiss": {},
	"respond": {},
}

func parseDialogAction(raw string) (string, error) {
	action := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := allowedDialogActions[action]; !ok {
		return "", fmt.Errorf("invalid action %q (expected accept, dismiss, or respond)", raw)
	}
	return action, nil
}

func formatDialogEvent(entry api.EventEntry) string {
	var data dialogEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Sprintf("[seq %d] <unparseable dialog entry>", entry.Seq)
	}
	if data.RespondedText != "" {
		return fmt.Sprintf("[seq %d] %s %q → %s (%q)", entry.Seq, data.DialogType, data.Message, data.ResolvedAction, data.RespondedText)
	}
	return fmt.Sprintf("[seq %d] %s %q → %s", entry.Seq, data.DialogType, data.Message, data.ResolvedAction)
}

func formatDialogEntries(resp *api.EventsResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	if resp.Dropped > 0 {
		fmt.Fprintf(&b, "%s %d events were evicted (buffer cap reached)\n", output.Warning("⚠"), resp.Dropped)
	}
	for _, entry := range resp.Entries {
		b.WriteString(formatDialogEvent(entry))
		b.WriteByte('\n')
	}
	return b.String()
}

var (
	dialogHandleAction string
	dialogHandleText   string
	dialogListSince    int
)

var dialogCmd = &cobra.Command{
	Use:   "dialog",
	Short: "Arm dialog handling and list handled dialogs",
}

var dialogHandleCmd = &cobra.Command{
	Use:   "handle",
	Short: "Arm a policy for the next JavaScript dialog(s)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("dialog handle")
		}
		action, err := parseDialogAction(dialogHandleAction)
		if err != nil {
			return err
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		options := map[string]any{"action": action}
		if action == "respond" {
			options["text"] = dialogHandleText
		}
		result, err := GetClient().ExecuteCommand(tabID, api.Command{
			ID:      "1",
			Action:  "dialogPolicy",
			Options: options,
		})
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("%s", result.Error)
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		fmt.Printf("Dialog policy armed: %s\n", action)
		return nil
	},
}

var dialogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List handled dialogs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("dialog list")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: dialogEventKind}
		resp, err := poller.Read(dialogListSince)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(resp)
			return nil
		}
		fmt.Print(formatDialogEntries(resp))
		return nil
	},
}

func init() {
	dialogHandleCmd.Flags().StringVar(&dialogHandleAction, "action", "accept", "accept, dismiss, or respond")
	dialogHandleCmd.Flags().StringVar(&dialogHandleText, "text", "", "response text (used with --action respond)")
	dialogListCmd.Flags().IntVar(&dialogListSince, "since", 0, "cursor to list after")

	dialogCmd.AddCommand(dialogHandleCmd)
	dialogCmd.AddCommand(dialogListCmd)
	rootCmd.AddCommand(dialogCmd)
}
