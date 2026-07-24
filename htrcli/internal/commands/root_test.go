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

func TestGetCDPPortDefaultValue(t *testing.T) {
	contextName = ""
	contextCDPPort = 0
	port := GetCDPPort()
	if port != 9222 {
		t.Fatalf("expected default port 9222, got %d", port)
	}
}

func TestGetCDPPortContextCached(t *testing.T) {
	contextName = "testctx"
	contextCDPPort = 9555
	defer func() { contextName = ""; contextCDPPort = 0 }()
	port := GetCDPPort()
	if port != 9555 {
		t.Fatalf("expected cached context port 9555, got %d", port)
	}
}

func TestEnsureContextResolvedLazily(t *testing.T) {
	contextName = "testctx"
	contextCDPPort = 0
	called := false
	orig := resolveContextFn
	resolveContextFn = func() error {
		called = true
		contextCDPPort = 9666
		return nil
	}
	t.Cleanup(func() {
		resolveContextFn = orig
		contextName = ""
		contextCDPPort = 0
	})
	if err := ensureContextResolved(); err != nil {
		t.Fatalf("ensureContextResolved: %v", err)
	}
	if !called {
		t.Fatal("expected resolver to be called lazily")
	}
	if got := GetCDPPort(); got != 9666 {
		t.Fatalf("expected resolved port 9666, got %d", got)
	}
}

func TestPersistentPreRunDoesNotResolveContext(t *testing.T) {
	contextName = "testctx"
	contextCDPPort = 0
	orig := resolveContextFn
	resolveContextFn = func() error {
		t.Fatal("resolveContextFn should not be called by PersistentPreRunE")
		return nil
	}
	t.Cleanup(func() {
		resolveContextFn = orig
		contextName = ""
		contextCDPPort = 0
	})
	if err := rootCmd.PersistentPreRunE(rootCmd, nil); err != nil {
		t.Fatalf("PersistentPreRunE: %v", err)
	}
}
