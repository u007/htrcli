package main

import (
	"os"
	"testing"
)

func TestIsNativeHostLaunch(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() {
		os.Args = origArgs
	})

	t.Run("chrome native host", func(t *testing.T) {
		os.Args = []string{"htrcli", "chrome-extension://abc123/"}
		if !isNativeHostLaunch() {
			t.Fatal("expected chrome native host launch to be detected")
		}
	})

	t.Run("firefox native host", func(t *testing.T) {
		os.Args = []string{"htrcli", "/tmp/" + hostManifestName, firefoxExtensionID}
		if !isNativeHostLaunch() {
			t.Fatal("expected firefox native host launch to be detected")
		}
	})

	t.Run("normal cli invocation", func(t *testing.T) {
		os.Args = []string{
			"htrcli",
			"install",
			"--browser",
			"firefox",
			"--extension-id",
			firefoxExtensionID,
		}
		if isNativeHostLaunch() {
			t.Fatal("did not expect cli invocation to be detected as native host")
		}
	})
}
