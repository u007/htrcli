package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// runFindRefCDP finds an element via CDP, mints a ref for it, and saves it
// to the persistent ref store (~/.htrcli/refs.json).
func runFindRefCDP(selector string, all bool) error {
	if frameFlag != "" {
		return fmt.Errorf("find --ref cannot be combined with --frame (refs are resolved in the top-level document)")
	}
	sel := parseSelector(selector)
	cssSel := ""
	if sel.Ref != "" {
		// Resolve from the ref store directly.
		rs, err := LoadRefStore()
		if err != nil {
			return err
		}
		backendID, ok := rs.Lookup(sel.Ref)
		if !ok {
			return fmt.Errorf("stale ref: %s is not known in the CDP ref store", sel.Ref)
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"ref": sel.Ref, "backendNodeId": backendID})
			return nil
		}
		fmt.Printf("%s => backendNodeId %d (from store)\n", sel.Ref, backendID)
		return nil
	}
	cssSel = sel.Selector
	if cssSel == "" {
		return fmt.Errorf("find --ref on CDP requires a CSS selector or an @eN ref, got %q", selector)
	}

	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	rs, err := LoadRefStore()
	if err != nil {
		return err
	}

	if all {
		ids, err := cdp.ResolveRefTargets(s, cssSel)
		if err != nil {
			return fmt.Errorf("resolving ref targets: %w", err)
		}
		refs := make([]string, len(ids))
		for i, id := range ids {
			refs[i] = rs.Alloc(id)
		}
		if err := rs.Save(); err != nil {
			return fmt.Errorf("saving ref store: %w", err)
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"refs": refs, "backendNodeIds": ids})
			return nil
		}
		for _, r := range refs {
			fmt.Println(r)
		}
	} else {
		backendID, err := cdp.ResolveBackendNodeID(s, cssSel)
		if err != nil {
			return fmt.Errorf("resolving backend node: %w", err)
		}
		ref := rs.Alloc(backendID)
		if err := rs.Save(); err != nil {
			return fmt.Errorf("saving ref store: %w", err)
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"ref": ref, "backendNodeId": backendID})
			return nil
		}
		fmt.Println(ref)
	}
	return nil
}

// errUnsupportedCDP guards verbs not yet ported to the CDP transport, so a
// sticky `transport=cdp` config can never silently misroute them to the
// extension daemon (pressing keys or scrolling in the wrong browser).
func errUnsupportedCDP(name string) error {
	return fmt.Errorf("%s is not supported over --cdp yet — run it with --transport ext", name)
}

// errCDPOnly guards CDP-only verbs on the extension transport, so they fail
// loudly instead of being misrouted to the extension daemon.
func errCDPOnly(name string) error {
	return fmt.Errorf("%s is only supported over --cdp", name)
}

// checkFrameTarget rejects selector forms that cannot be scoped to --frame:
// @eN refs live in the top document's ref store, and xy= coordinates are
// top-viewport pixels, so neither can be meaningfully resolved inside a frame.
func checkFrameTarget(sel *api.TargetSelector) error {
	if frameFlag == "" || sel == nil {
		return nil
	}
	if sel.Ref != "" {
		return fmt.Errorf("@eN refs cannot be combined with --frame; use a CSS (or name=/label=/text=…) selector")
	}
	if sel.X != nil || sel.Y != nil {
		return fmt.Errorf("xy= coordinates cannot be combined with --frame; use an element selector")
	}
	return nil
}

// cdpSession opens the target page session for the current --tab/--cdp-port.
func cdpSession() (*cdp.Session, string, error) {
	targetID := GetTabTarget()
	if err := ensureContextResolved(); err != nil {
		return nil, "", err
	}
	port := GetCDPPort()
	s, err := cdp.PageSession(port, targetID)
	if err != nil {
		return nil, "", err
	}
	s.SetFrame(frameFlag)
	if targetID == "" {
		targets, err := cdp.ListTargets(port)
		if err == nil && len(targets) > 0 {
			targetID = targets[0].ID
		}
	}
	return s, targetID, nil
}

// runInteractCDP mirrors runInteract over the CDP transport.
func runInteractCDP(action, selector, value string) error {
	if err := checkFrameTarget(parseSelector(selector)); err != nil {
		return err
	}
	s, targetID, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	switch action {
	case "click", "dblclick", "rightclick":
		if err := cdp.Click(s, targetID, parseSelector(selector), action); err != nil {
			return err
		}
	case "pressKey":
		if err := cdp.Press(s, targetID, value); err != nil {
			return err
		}
	case "keyDown":
		if err := cdp.KeyDown(s, targetID, value); err != nil {
			return err
		}
	case "keyUp":
		if err := cdp.KeyUp(s, targetID, value); err != nil {
			return err
		}
	case "mouseDown":
		if err := cdp.MouseDown(s, targetID, parseSelector(selector)); err != nil {
			return err
		}
	case "mouseUp":
		if err := cdp.MouseUp(s, targetID, parseSelector(selector)); err != nil {
			return err
		}
	case "mouseMove":
		if err := cdp.MouseMove(s, targetID, parseSelector(selector)); err != nil {
			return err
		}
	default: // fill, select, check, uncheck, clear, type — DOM verbs
		result, err := cdp.ExecDOM(s, api.Command{
			ID: "1", Action: action, Target: parseSelector(selector), Value: value,
		})
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("%s failed: %s", action, result.Error)
		}
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "action": action})
		return nil
	}
	fmt.Printf("%s %s (cdp)\n", strings.Title(action), selector)
	return nil
}

func runDragCDP(s *cdp.Session, targetID, src, dst string, steps, delay int) error {
	srcSel := parseSelector(src)
	dstSel := parseSelector(dst)
	var refs *RefStore
	if srcSel.Ref != "" || dstSel.Ref != "" {
		var err error
		refs, err = LoadRefStore()
		if err != nil {
			return err
		}
	}

	srcEnd, err := resolveDragTargetCDP(refs, srcSel)
	if err != nil {
		return fmt.Errorf("resolve drag source: %w", err)
	}
	dstEnd, err := resolveDragTargetCDP(refs, dstSel)
	if err != nil {
		return fmt.Errorf("resolve drag destination: %w", err)
	}
	return cdp.Drag(s, targetID, srcEnd, dstEnd, steps, delay)
}

// resolveDragTargetCDP translates a persistent @eN ref in the command layer
// into the backendNodeId the cdp package drags by. RefStore belongs to
// commands; the cdp package only owns the protocol work. The ref is NOT
// turned into pixels here: the drag resolver scrolls both endpoints first and
// reads their positions afterwards, so a coordinate taken now would be stale
// by the time the other endpoint has been scrolled into view.
func resolveDragTargetCDP(refs *RefStore, sel *api.TargetSelector) (cdp.DragEndpoint, error) {
	if sel == nil || sel.Ref == "" {
		return cdp.DragEndpoint{Sel: sel}, nil
	}
	if refs == nil {
		return cdp.DragEndpoint{}, fmt.Errorf("stale ref %q: CDP ref store was not loaded", sel.Ref)
	}
	backendNodeID, ok := refs.Lookup(sel.Ref)
	if !ok {
		return cdp.DragEndpoint{}, fmt.Errorf("stale ref %q: not found in the CDP ref store", sel.Ref)
	}
	return cdp.DragEndpoint{BackendNodeID: backendNodeID}, nil
}

// runInspectCDP routes inspect verbs (find, getValue, getText, getHTML,
// getAttribute, getPageInfo, ...) through the embedded DOM bundle.
func runInspectCDP(action, selector, attr string) error {
	if err := checkFrameTarget(parseSelector(selector)); err != nil {
		return err
	}
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	cmd := api.Command{ID: "1", Action: action, Target: parseSelector(selector)}
	if action == "getAttribute" && attr != "" {
		cmd.Options = map[string]any{"attribute": attr}
	}
	result, err := cdp.ExecDOM(s, cmd)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s failed: %s", action, result.Error)
	}
	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}
	fmt.Printf("%v\n", result.Data)
	return nil
}

// runSnapshotCDP captures an accessibility snapshot through the embedded DOM
// bundle and renders it with the same pretty printer used by the extension
// transport.
func runSnapshotCDP() error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	result, err := cdp.ExecDOM(s, api.Command{ID: "1", Action: "snapshot"})
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("snapshot failed: %s", result.Error)
	}
	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}
	return printSnapshotResult(result)
}

// runEvalCDP runs a user script via CDP Runtime.evaluate.
func runEvalCDP(expression string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	raw, err := cdp.Evaluate(s, expression)
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(json.RawMessage(raw))
		return nil
	}
	fmt.Printf("%s\n", raw)
	return nil
}

// runScreenshotCDP captures the page via CDP.
func runScreenshotCDP(path string, fullPage bool, annotate []api.TargetSelector) error {
	if len(annotate) > 0 {
		return errUnsupportedCDP("screenshot --annotate")
	}

	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	var png []byte
	if fullPage {
		png, err = cdp.ScreenshotFullPage(s)
	} else {
		png, err = cdp.Screenshot(s)
	}
	if err != nil {
		return err
	}

	out := path
	if out == "" {
		out = filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixMilli()))
	}
	if err := os.WriteFile(out, png, 0644); err != nil {
		return fmt.Errorf("failed to write screenshot: %w", err)
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]string{"screenshot": out})
		return nil
	}
	fmt.Printf("Screenshot saved to %s\n", out)
	return nil
}

// runOpenCDP navigates the page via CDP.
func runOpenCDP(url string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := cdp.Navigate(s, url, timeout); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "url": url})
		return nil
	}
	fmt.Printf("Navigated to %s (cdp)\n", url)
	return nil
}

// runReloadCDP mirrors runReload over the CDP transport.
func runReloadCDP() error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := cdp.Reload(s, timeout); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true})
		return nil
	}
	fmt.Println("Reloaded (cdp)")
	return nil
}

// runTabsListCDP lists CDP page targets (no Active column — CDP has no
// reliable notion of "active").
func runTabsListCDP() error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	targets, err := cdp.ListTargets(GetCDPPort())
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(targets)
		return nil
	}
	if len(targets) == 0 {
		fmt.Println("No tabs connected")
		return nil
	}
	table := output.NewTable("ID", "Title", "URL")
	for _, t := range targets {
		title := t.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		url := t.URL
		if len(url) > 40 {
			url = url[:37] + "..."
		}
		table.AddRow(t.ID, title, url)
	}
	fmt.Print(table)
	return nil
}
