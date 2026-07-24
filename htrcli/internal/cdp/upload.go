package cdp

import "fmt"

// SetFileInputFiles sets the files on a file <input> identified by
// backendNodeId, via CDP DOM.setFileInputFiles. The file paths are on the
// same host as the browser (htrcli and Chrome are both local), so no upload
// dialog appears. DOM.enable must already have been called by the resolver.
func SetFileInputFiles(s *Session, backendNodeID int64, files []string) error {
	if err := s.Call("DOM.setFileInputFiles", map[string]any{
		"backendNodeId": backendNodeID,
		"files":         files,
	}, nil); err != nil {
		return fmt.Errorf("DOM.setFileInputFiles: %w", err)
	}
	return nil
}
