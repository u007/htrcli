package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// refStorePathOverride, when non-empty, redirects LoadRefStore's file path from
// the default ~/.htrcli/refs.json so tests can use a temp directory.
var refStorePathOverride string

// refStorePath returns the path to the ref store JSON file.
func refStorePath() string {
	if refStorePathOverride != "" {
		return refStorePathOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".htrcli", "refs.json")
}

// RefStore persists @eN → backendNodeId mappings across CDP CLI invocations.
// The JSON file is stored at ~/.htrcli/refs.json.
//
// LoaderIDs records the document identity (main-frame loaderId) each ref was
// minted in, which lets `wait --gone` tell "this node was pruned in the same
// document" apart from "the document that held it navigated away". TargetIDs
// records the CDP page target (tab) each ref was minted in, so a wait aimed at
// a different tab is a hard error instead of a false "gone": a backendNodeId
// means nothing outside the target whose DOM tree issued it.
//
// Both maps are additive and optional: a file written by an older build has
// neither key, so a missing entry means the identity is unknown, never that it
// mismatched. That keeps the existing refs.json readable by both directions, so
// the file format needs no version bump; an older build ignores the keys on
// read and drops them on its next write.
type RefStore struct {
	NextRef   int               `json:"nextRef"`
	Refs      map[string]int64  `json:"refs"`
	LoaderIDs map[string]string `json:"loaderIds,omitempty"`
	TargetIDs map[string]string `json:"targetIds,omitempty"`
	path      string            // populated on load, not serialized
}

// LoadRefStore reads the ref store from disk, or returns an empty store if the
// file does not exist (first run).
func LoadRefStore() (*RefStore, error) {
	path := refStorePath()
	if path == "" {
		return nil, fmt.Errorf("cannot determine home directory for ref store")
	}
	rs := &RefStore{NextRef: 0, Refs: map[string]int64{}, LoaderIDs: map[string]string{}, TargetIDs: map[string]string{}, path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return rs, nil // fresh store
		}
		return nil, fmt.Errorf("reading ref store %s: %w", path, err)
	}
	if err := json.Unmarshal(data, rs); err != nil {
		return nil, fmt.Errorf("parsing ref store %s: %w", path, err)
	}
	if rs.Refs == nil {
		rs.Refs = map[string]int64{}
	}
	if rs.LoaderIDs == nil {
		rs.LoaderIDs = map[string]string{}
	}
	if rs.TargetIDs == nil {
		rs.TargetIDs = map[string]string{}
	}
	rs.path = path
	return rs, nil
}

// Alloc mints the next @eN id for a backendNodeId.
func (rs *RefStore) Alloc(backendNodeID int64) string {
	rs.NextRef++
	refID := fmt.Sprintf("@e%d", rs.NextRef)
	rs.Refs[refID] = backendNodeID
	return refID
}

// AllocInDocument mints the next @eN id for a backendNodeId and records where
// it was minted: the main frame's loaderId and the CDP page target that served
// the DOM query. An empty value leaves that identity unknown, so the ref
// behaves like one written by an older build.
func (rs *RefStore) AllocInDocument(backendNodeID int64, loaderID, targetID string) string {
	refID := rs.Alloc(backendNodeID)
	if loaderID != "" {
		rs.LoaderIDs[refID] = loaderID
	}
	if targetID != "" {
		rs.TargetIDs[refID] = targetID
	}
	return refID
}

// Lookup resolves a ref id to its backendNodeId.
func (rs *RefStore) Lookup(refID string) (int64, bool) {
	id, ok := rs.Refs[refID]
	return id, ok
}

// LookupDocument returns the loaderId of the document the ref was minted in.
// ok is false when the identity is unknown (a ref written before loaderIDs were
// recorded), which callers must not treat as a mismatch.
func (rs *RefStore) LookupDocument(refID string) (string, bool) {
	id, ok := rs.LoaderIDs[refID]
	return id, ok && id != ""
}

// LookupTarget returns the CDP page target the ref was minted in. ok is false
// when the identity is unknown (a ref written before targetIDs were recorded),
// which callers must not treat as a mismatch.
func (rs *RefStore) LookupTarget(refID string) (string, bool) {
	id, ok := rs.TargetIDs[refID]
	return id, ok && id != ""
}

// Save writes the store back to disk, creating ~/.htrcli if needed.
func (rs *RefStore) Save() error {
	if err := os.MkdirAll(filepath.Dir(rs.path), 0o755); err != nil {
		return fmt.Errorf("creating ref store dir: %w", err)
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(rs.path, data, 0o600); err != nil {
		return fmt.Errorf("writing ref store %s: %w", rs.path, err)
	}
	return nil
}
