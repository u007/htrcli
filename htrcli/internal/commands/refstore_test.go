package commands

import (
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
