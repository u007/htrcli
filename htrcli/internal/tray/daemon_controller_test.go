package tray

import (
	"reflect"
	"runtime"
	"testing"
)

type fakeCommander struct {
	calls []fakeCmd
}

type fakeCmd struct {
	Name string
	Args []string
}

func (f *fakeCommander) Run(name string, args ...string) error {
	f.calls = append(f.calls, fakeCmd{name, args})
	return nil
}

func (f *fakeCommander) Output(name string, args ...string) ([]byte, error) {
	return nil, nil
}

func TestReinstallHost(t *testing.T) {
	d := &daemonController{
		selfPath: "/usr/bin/htrcli",
		getExtID: func(b string) string { return "my-ext-id" },
		cmd:      &fakeCommander{},
	}
	if err := d.ReinstallHost("chrome"); err != nil {
		t.Fatal(err)
	}
	fc := d.cmd.(*fakeCommander)
	if len(fc.calls) != 1 {
		t.Fatalf("calls: %v", fc.calls)
	}
	want := []string{"install", "--browser", "chrome", "--extension-id", "my-ext-id"}
	if !reflect.DeepEqual(fc.calls[0].Args, want) {
		t.Fatalf("got %v, want %v", fc.calls[0].Args, want)
	}
}

func TestReinstallHostMissingExtID(t *testing.T) {
	d := &daemonController{
		getExtID: func(b string) string { return "" },
		cmd:      &fakeCommander{},
	}
	if err := d.ReinstallHost("chrome"); err == nil {
		t.Fatal("want error for missing ext ID")
	}
}

func TestStripSecrets(t *testing.T) {
	in := []string{"serve", "--token", "supersecret", "--port", "3845"}
	out := stripSecrets(in)
	want := []string{"serve", "--port", "3845"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("got %v, want %v", out, want)
	}

	// =value form
	in2 := []string{"serve", "--token=supersecret", "--port", "3845"}
	out2 := stripSecrets(in2)
	if !reflect.DeepEqual(out2, want) {
		t.Fatalf("=form got %v, want %v", out2, want)
	}
}

func TestCopyTokenToClipboardNoToken(t *testing.T) {
	d := &daemonController{getToken: func() string { return "" }}
	if _, err := d.CopyTokenToClipboard(); err == nil {
		t.Fatal("want error when no token set")
	}
}

func TestCopyTokenToClipboardDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only: exercises pbcopy path")
	}
	d := &daemonController{
		getToken: func() string { return "abcd1234efgh5678" },
	}
	if _, err := d.CopyTokenToClipboard(); err != nil {
		t.Fatalf("copy: %v", err)
	}
}
