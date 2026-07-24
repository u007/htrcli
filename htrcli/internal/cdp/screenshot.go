package cdp

import (
	"encoding/base64"
	"fmt"
)

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
