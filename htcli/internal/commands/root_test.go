package commands

import "testing"

func TestGetTabIDNumeric(t *testing.T) {
	tabTarget = "123"
	defer func() { tabTarget = "" }()
	id, err := GetTabID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == nil || *id != 123 {
		t.Fatalf("want 123, got %v", id)
	}
}

func TestGetTabIDEmpty(t *testing.T) {
	tabTarget = ""
	id, err := GetTabID()
	if err != nil || id != nil {
		t.Fatalf("want nil,nil got %v,%v", id, err)
	}
}

func TestGetTabIDNonNumeric(t *testing.T) {
	tabTarget = "8E17C9D24A3B41F09E60C1D2A55F7B31"
	defer func() { tabTarget = "" }()
	if _, err := GetTabID(); err == nil {
		t.Fatal("want error for non-numeric tab on extension transport")
	}
}

func TestGetTabTarget(t *testing.T) {
	tabTarget = "8E17C9D24A3B41F09E60C1D2A55F7B31"
	defer func() { tabTarget = "" }()
	if got := GetTabTarget(); got != "8E17C9D24A3B41F09E60C1D2A55F7B31" {
		t.Fatalf("got %q", got)
	}
}
