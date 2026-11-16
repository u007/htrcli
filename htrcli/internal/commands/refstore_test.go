package commands

import (
	"os"
	"testing"
)

func TestRefStoreAllocAndLookup(t *testing.T) {
	// Redirect the store to a temp file so the test never touches ~/.htrcli.
	dir := t.TempDir()
	refStorePathOverride = dir + "/refs.json"
	defer func() { refStorePathOverride = "" }()

	rs, err := LoadRefStore()
	if err != nil {
		t.Fatalf("LoadRefStore: %v", err)
	}
	refA := rs.Alloc(9007)
	refB := rs.Alloc(9008)
	if refA != "@e1" || refB != "@e2" {
		t.Fatalf("want @e1,@e2 got %s,%s", refA, refB)
	}
	if err := rs.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A fresh load (new CLI process) still resolves the refs.
	rs2, err := LoadRefStore()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	id, ok := rs2.Lookup("@e1")
	if !ok || id != 9007 {
		t.Fatalf("want 9007,true got %d,%v", id, ok)
	}
	if _, ok := rs2.Lookup("@e999"); ok {
		t.Fatal("unknown ref must not resolve")
	}
	// Alloc continues the sequence across processes (no id reuse).
	if next := rs2.Alloc(9009); next != "@e3" {
		t.Fatalf("want @e3 after reload, got %s", next)
	}
}

// A ref minted in a document records that document's loaderId and the CDP page
// target that served the query, and both survive a reload. Refs minted without
// an identity stay unknown.
func TestRefStoreAllocInDocumentPersistsLoaderID(t *testing.T) {
	dir := t.TempDir()
	refStorePathOverride = dir + "/refs.json"
	defer func() { refStorePathOverride = "" }()

	rs, err := LoadRefStore()
	if err != nil {
		t.Fatalf("LoadRefStore: %v", err)
	}
	known := rs.AllocInDocument(9007, "loader-A", "TAB-A")
	unknown := rs.AllocInDocument(9008, "", "")
	if err := rs.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rs2, err := LoadRefStore()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	loaderID, ok := rs2.LookupDocument(known)
	if !ok || loaderID != "loader-A" {
		t.Fatalf("LookupDocument(%s) = %q,%v, want loader-A,true", known, loaderID, ok)
	}
	if _, ok := rs2.LookupDocument(unknown); ok {
		t.Fatalf("a ref minted without a loaderId must stay unknown, got %v", ok)
	}
	targetID, ok := rs2.LookupTarget(known)
	if !ok || targetID != "TAB-A" {
		t.Fatalf("LookupTarget(%s) = %q,%v, want TAB-A,true", known, targetID, ok)
	}
	if _, ok := rs2.LookupTarget(unknown); ok {
		t.Fatalf("a ref minted without a target must stay unknown, got %v", ok)
	}
	if id, ok := rs2.Lookup(known); !ok || id != 9007 {
		t.Fatalf("backendNodeId for %s = %d,%v, want 9007,true", known, id, ok)
	}
}

// A refs.json written before loaderIDs or targetIDs existed must still load:
// the missing fields mean "identity unknown", never a mismatch, and the next
// save keeps the file readable.
func TestRefStoreLoadsOldFormatWithoutLoaderIDs(t *testing.T) {
	dir := t.TempDir()
	refStorePathOverride = dir + "/refs.json"
	defer func() { refStorePathOverride = "" }()

	oldFormat := `{
  "nextRef": 2,
  "refs": {
    "@e1": 9007,
    "@e2": 9008
  }
}`
	if err := os.WriteFile(refStorePathOverride, []byte(oldFormat), 0o600); err != nil {
		t.Fatalf("write old-format store: %v", err)
	}

	rs, err := LoadRefStore()
	if err != nil {
		t.Fatalf("LoadRefStore old format: %v", err)
	}
	if id, ok := rs.Lookup("@e1"); !ok || id != 9007 {
		t.Fatalf("old-format @e1 = %d,%v, want 9007,true", id, ok)
	}
	if loaderID, ok := rs.LookupDocument("@e1"); ok {
		t.Fatalf("old-format ref must have no document identity, got %q", loaderID)
	}
	if targetID, ok := rs.LookupTarget("@e1"); ok {
		t.Fatalf("old-format ref must have no target identity, got %q", targetID)
	}
	// The sequence still continues, and a new ref can record its identity.
	if next := rs.AllocInDocument(9009, "loader-B", "TAB-B"); next != "@e3" {
		t.Fatalf("want @e3 after an old-format load, got %s", next)
	}
	if err := rs.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rs2, err := LoadRefStore()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaderID, ok := rs2.LookupDocument("@e3"); !ok || loaderID != "loader-B" {
		t.Fatalf("LookupDocument(@e3) = %q,%v, want loader-B,true", loaderID, ok)
	}
	if targetID, ok := rs2.LookupTarget("@e3"); !ok || targetID != "TAB-B" {
		t.Fatalf("LookupTarget(@e3) = %q,%v, want TAB-B,true", targetID, ok)
	}
}
