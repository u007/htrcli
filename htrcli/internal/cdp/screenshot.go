package cdp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/u007/htrcli/internal/api"
)

// AnnotationMarkerAttr marks every overlay element htrcli draws, so cleanup
// removes exactly those and nothing else the page contains.
const AnnotationMarkerAttr = "data-htrcli-annotate"

// AnnotationBox is one numbered overlay: the element's viewport rectangle and
// the number drawn on it.
type AnnotationBox struct {
	Index  int     `json:"index"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// AnnotationBoxes resolves each selector through the DOM bundle, the same
// resolver Click uses, and returns its viewport box. Numbering follows argument
// order. A selector that matches nothing is an error that names it.
func AnnotationBoxes(s *Session, targets []api.TargetSelector) ([]AnnotationBox, error) {
	boxes := make([]AnnotationBox, 0, len(targets))
	for i := range targets {
		sel := &targets[i]
		label, err := json.Marshal(sel)
		if err != nil {
			return nil, fmt.Errorf("encoding annotate selector: %w", err)
		}
		res, err := ExecDOM(s, api.Command{ID: "annotate", Action: "getBoundingBox", Target: sel})
		if err != nil {
			return nil, fmt.Errorf("annotate %s: %w", label, err)
		}
		if !res.Success {
			return nil, fmt.Errorf("annotate %s: %s", label, res.Error)
		}
		if res.Data == nil {
			return nil, fmt.Errorf("annotate %s: no element matches it", label)
		}
		data, err := json.Marshal(res.Data)
		if err != nil {
			return nil, fmt.Errorf("re-encoding bounding box for annotate %s: %w", label, err)
		}
		var rect struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		}
		if err := json.Unmarshal(data, &rect); err != nil {
			return nil, fmt.Errorf("decoding bounding box for annotate %s: %w", label, err)
		}
		boxes = append(boxes, AnnotationBox{Index: i + 1, X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height})
	}
	return boxes, nil
}

// annotateScriptTmpl draws one fixed-position box and numbered badge per entry.
// %[1]s is the JSON array of boxes, %[2]s the marker attribute name.
const annotateScriptTmpl = `(() => {
	const boxes = %[1]s;
	for (const b of boxes) {
		const box = document.createElement("div");
		box.setAttribute("%[2]s", String(b.index));
		Object.assign(box.style, {
			position: "absolute", left: (b.x + window.scrollX) + "px", top: (b.y + window.scrollY) + "px",
			width: b.width + "px", height: b.height + "px",
			border: "2px solid #ef4444", boxSizing: "border-box",
			pointerEvents: "none", zIndex: "2147483647"
		});
		const badge = document.createElement("span");
		badge.textContent = String(b.index);
		Object.assign(badge.style, {
			position: "absolute", left: "-2px", top: "-2px",
			background: "#ef4444", color: "#fff",
			font: "600 12px/16px sans-serif", padding: "0 4px"
		});
		box.appendChild(badge);
		document.documentElement.appendChild(box);
	}
	return boxes.length;
})()`

// AnnotateScript builds the page expression that draws the overlay boxes. Boxes
// are absolute in the document (viewport rect plus scroll offset), so a
// --full-page capture of a scrolled page places them on the right elements.
func AnnotateScript(boxes []AnnotationBox) (string, error) {
	raw, err := json.Marshal(boxes)
	if err != nil {
		return "", fmt.Errorf("encoding annotation boxes: %w", err)
	}
	return fmt.Sprintf(annotateScriptTmpl, raw, AnnotationMarkerAttr), nil
}

// annotateClearScript builds the page expression that removes every element
// carrying AnnotationMarkerAttr.
func annotateClearScript() string {
	return fmt.Sprintf(`(() => {
	document.querySelectorAll("[%s]").forEach((el) => el.remove());
	return true;
})()`, AnnotationMarkerAttr)
}

// ClearAnnotations removes every overlay drawn by AnnotateScript.
func ClearAnnotations(s *Session) error {
	if _, err := Evaluate(s, annotateClearScript()); err != nil {
		return fmt.Errorf("removing annotation overlay: %w", err)
	}
	return nil
}

// PrintPDF renders the page to PDF with backgrounds (Page.printToPDF).
func PrintPDF(s *Session) ([]byte, error) {
	var res struct {
		Data string `json:"data"`
	}
	if err := s.Call("Page.printToPDF", map[string]any{"printBackground": true}, &res); err != nil {
		return nil, err
	}
	pdf, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding PDF: %w", err)
	}
	return pdf, nil
}

// layoutMetrics is the subset of Page.getLayoutMetrics we consume. cssContentSize
// is the full scrollable document size in CSS pixels (modern Chrome); it is what
// the full-page clip must match.
type layoutMetrics struct {
	CSSContentSize struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"cssContentSize"`
}

// ScreenshotFullPage captures the entire page, including content below the fold,
// via Page.captureScreenshot{captureBeyondViewport:true} clipped to the document's
// CSS content size (from Page.getLayoutMetrics). Sibling to Screenshot (viewport-
// only) in nav.go; the plain viewport path is intentionally left unchanged.
func ScreenshotFullPage(s *Session) ([]byte, error) {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return nil, fmt.Errorf("Page.enable: %w", err)
	}
	var m layoutMetrics
	if err := s.Call("Page.getLayoutMetrics", nil, &m); err != nil {
		return nil, fmt.Errorf("Page.getLayoutMetrics: %w", err)
	}
	w, h := m.CSSContentSize.Width, m.CSSContentSize.Height
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("could not determine page content size (got %gx%g); requires Chrome with cssContentSize support", w, h)
	}

	var res struct {
		Data string `json:"data"`
	}
	params := map[string]any{
		"format":                "png",
		"captureBeyondViewport": true,
		"clip": map[string]any{
			"x":      0.0,
			"y":      0.0,
			"width":  w,
			"height": h,
			"scale":  1.0,
		},
	}
	if err := s.Call("Page.captureScreenshot", params, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}
