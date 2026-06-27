package host_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/u007/htcli/internal/host"
)

func TestDaemonTabRegistry(t *testing.T) {
	d := host.NewDaemon()

	d.RegisterTab(1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
	d.RegisterTab(2, host.TabInfo{ID: 2, URL: "https://b.com", Title: "B", Active: false})

	tabs := d.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("want 2 tabs, got %d", len(tabs))
	}

	d.RemoveTab(1)
	if len(d.Tabs()) != 1 {
		t.Fatalf("want 1 tab after removal, got %d", len(d.Tabs()))
	}

	id, ok := d.FirstTabID()
	if !ok || id != 2 {
		t.Errorf("FirstTabID = %d, %v; want 2, true", id, ok)
	}
}

func TestDaemonEnqueueAndResolve(t *testing.T) {
	d := host.NewDaemon()
	d.RegisterTab(1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})

	ch, err := d.EnqueueCommand(1, host.Command{ID: "cmd-1", Action: "navigate", Value: "https://x.com"})
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	result := host.CommandResult{ID: "cmd-1", Success: true, Duration: 42}
	go func() {
		time.Sleep(10 * time.Millisecond)
		d.ResolveCommand("cmd-1", result)
	}()

	select {
	case got := <-ch:
		if got.ID != "cmd-1" || !got.Success {
			t.Errorf("unexpected result: %+v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for command result")
	}
}

func TestDaemonEnqueueTabNotFound(t *testing.T) {
	d := host.NewDaemon()
	_, err := d.EnqueueCommand(99, host.Command{ID: "cmd-x", Action: "navigate"})
	if err == nil {
		t.Error("expected error for missing tab, got nil")
	}
}

func TestCommandJSON(t *testing.T) {
	cmd := host.Command{ID: "c1", Action: "click"}
	b, _ := json.Marshal(cmd)
	var got host.Command
	json.Unmarshal(b, &got)
	if got.ID != "c1" || got.Action != "click" {
		t.Errorf("round-trip failed: %+v", got)
	}
}
