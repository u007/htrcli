package host_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/host"
)

func noopConn(d *host.Daemon) *host.RelayConn {
	return d.AddConn(func(_ []byte) error { return nil })
}

func TestDaemonTabRegistry(t *testing.T) {
	d := host.NewDaemon()
	rc := noopConn(d)

	d.RegisterTab(rc, 1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
	d.RegisterTab(rc, 2, host.TabInfo{ID: 2, URL: "https://b.com", Title: "B", Active: false})

	tabs := d.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("want 2 tabs, got %d", len(tabs))
	}

	d.RemoveTab(rc, 1)
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
	rc := noopConn(d)
	d.RegisterTab(rc, 1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})

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

// Commands must reach the connection that owns the target tab, not some other
// connected browser — and one browser disconnecting must not drop the others'
// tabs.
func TestDaemonRoutesPerConnection(t *testing.T) {
	d := host.NewDaemon()

	chromeGot := make(chan int, 1)
	firefoxGot := make(chan int, 1)
	chrome := d.AddConn(func(msg []byte) error {
		var m host.NativeMessage
		json.Unmarshal(msg, &m)
		chromeGot <- m.TabID
		return nil
	})
	firefox := d.AddConn(func(msg []byte) error {
		var m host.NativeMessage
		json.Unmarshal(msg, &m)
		firefoxGot <- m.TabID
		return nil
	})

	d.RegisterTab(chrome, 1100638190, host.TabInfo{ID: 1100638190, URL: "https://maps.google.com"})
	d.RegisterTab(firefox, 3, host.TabInfo{ID: 3, URL: "https://yahoo.com"})

	if _, err := d.EnqueueCommand(3, host.Command{ID: "c1", Action: "reload"}); err != nil {
		t.Fatalf("EnqueueCommand(firefox tab): %v", err)
	}
	select {
	case got := <-firefoxGot:
		if got != 3 {
			t.Errorf("firefox got tab %d, want 3", got)
		}
	case got := <-chromeGot:
		t.Fatalf("command for firefox tab 3 was misdelivered to chrome (tab %d)", got)
	case <-time.After(time.Second):
		t.Fatal("firefox connection never received the command")
	}

	// Firefox disconnects — Chrome's tab must survive.
	d.RemoveConn(firefox)
	tabs := d.Tabs()
	if len(tabs) != 1 || tabs[0].ID != 1100638190 {
		t.Fatalf("after firefox disconnect, want only chrome tab; got %+v", tabs)
	}
	if _, err := d.EnqueueCommand(1100638190, host.Command{ID: "c2", Action: "reload"}); err != nil {
		t.Fatalf("chrome command after firefox disconnect: %v", err)
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

func TestSweepConnsReapsStaleAndPingsLive(t *testing.T) {
	d := host.NewDaemon()

	var pings [][]byte
	live := d.AddConn(func(msg []byte) error {
		pings = append(pings, msg)
		return nil
	})
	d.RegisterTab(live, 1, host.TabInfo{ID: 1, URL: "https://live.com"})

	// A generous staleAfter keeps the fresh connection and sends it a ping.
	if reaped := d.SweepConns(time.Hour); reaped != 0 {
		t.Fatalf("fresh conn reaped, want 0 reaped, got %d", reaped)
	}
	if len(pings) != 1 {
		t.Fatalf("want 1 ping to live conn, got %d", len(pings))
	}
	var msg host.NativeMessage
	if err := json.Unmarshal(pings[0], &msg); err != nil || msg.Type != "ping" {
		t.Fatalf("want ping message, got %s (err %v)", pings[0], err)
	}

	// staleAfter 0: any conn is stale once a moment has passed.
	closed := false
	stale := d.AddConn(func(_ []byte) error { return nil })
	d.SetConnCloser(stale, func() error {
		closed = true
		return nil
	})
	d.RegisterTab(stale, 3, host.TabInfo{ID: 3, URL: "https://stale.com"})
	time.Sleep(time.Millisecond)
	// Remove the live conn first so only the stale one is subject to sweep.
	d.RemoveConn(live)
	if reaped := d.SweepConns(0); reaped != 1 {
		t.Fatalf("want 1 reaped, got %d", reaped)
	}
	if !closed {
		t.Error("stale conn transport was not closed")
	}
	if len(d.Tabs()) != 0 {
		t.Errorf("stale conn tabs still listed: %+v", d.Tabs())
	}
}

func TestSweepConnsReapsWriteFailures(t *testing.T) {
	d := host.NewDaemon()
	closed := false
	rc := d.AddConn(func(_ []byte) error { return errBrokenPipe })
	d.SetConnCloser(rc, func() error {
		closed = true
		return nil
	})
	d.RegisterTab(rc, 7, host.TabInfo{ID: 7, URL: "https://dead.com"})

	if reaped := d.SweepConns(time.Hour); reaped != 1 {
		t.Fatalf("want 1 reaped on write failure, got %d", reaped)
	}
	if !closed {
		t.Error("conn transport was not closed after write failure")
	}
	if len(d.Tabs()) != 0 {
		t.Errorf("dead conn tabs still listed: %+v", d.Tabs())
	}
}

var errBrokenPipe = fmt.Errorf("write: broken pipe")
